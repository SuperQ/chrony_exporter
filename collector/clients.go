// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"fmt"
	"log/slog"
	"math"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	clientsSubsystem = "clients"

	// intervalInvalid matches chrony's clientlog: an interval rate >= 127
	// means chronyd has no average interval for the client yet, which
	// chronyc renders as "-".
	intervalInvalid int8 = 127

	// lastHitNever is the (uint32)-1 sentinel chronyd uses for a "last hit
	// ago" that never happened, rendered as "-" by chronyc.
	lastHitNever uint32 = 0xFFFFFFFF
)

// clientsTimeBuckets are the upper bounds, in seconds, for the last-seen and
// NTP-interval histograms. They cover the 1 second to 1 day range requested in
// SuperQ/chrony_exporter#136.
var clientsTimeBuckets = []float64{1, 5, 30, 60, 300, 1800, 3600, 21600, 86400}

// clientsDropBuckets are the upper bounds (counts) for the per-client NTP drop
// histogram.
var clientsDropBuckets = []float64{0, 1, 2, 5, 10, 50, 100}

var (
	clientsConnected = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "connected"),
			"The number of clients in chronyd's log. The protocol label is \"nts\" for clients that completed an NTS-KE handshake and \"ntp\" for the rest. Saturates at the client log size (clientloglimit).",
			[]string{"protocol"},
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsLastNTPHitAgo = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, clientsSubsystem, "last_ntp_hit_ago_seconds"),
		"Histogram of the time since the last NTP request from each client, in seconds. Clients that have not sent an NTP request are not included.",
		nil,
		nil,
	)

	clientsNTPInterval = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, clientsSubsystem, "ntp_interval_seconds"),
		"Histogram of the mean interval between NTP requests from each client, in seconds. Clients that chronyd has not measured an interval for, such as those seen only once, are not included.",
		nil,
		nil,
	)

	clientsNTPDrops = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, clientsSubsystem, "ntp_drops"),
		"Histogram of the number of NTP requests dropped (rate limited) per client.",
		nil,
		nil,
	)
)

// clientsAggregate accumulates per-client statistics across all pages of a
// 'clients' reply into the aggregate metrics exported by the collector.
type clientsAggregate struct {
	ntpClients uint64
	ntsClients uint64

	lastHitCount uint64
	lastHitSum   float64
	lastHitBkts  map[float64]uint64

	intervalCount uint64
	intervalSum   float64
	intervalBkts  map[float64]uint64

	dropsCount uint64
	dropsSum   float64
	dropsBkts  map[float64]uint64
}

func newClientsAggregate() *clientsAggregate {
	return &clientsAggregate{
		lastHitBkts:  newBuckets(clientsTimeBuckets),
		intervalBkts: newBuckets(clientsTimeBuckets),
		dropsBkts:    newBuckets(clientsDropBuckets),
	}
}

// add folds a single client's statistics into the aggregate.
func (a *clientsAggregate) add(c chrony.ClientAccess) {
	// A client that has completed an NTS-KE handshake counts as nts.
	if c.NKEHits > 0 {
		a.ntsClients++
	} else {
		a.ntpClients++
	}

	// The drops histogram covers clients that sent or attempted NTP, so a
	// fully rate-limited client (NTPHits == 0, NTPDrops > 0) still counts.
	if c.NTPHits > 0 || c.NTPDrops > 0 {
		drops := float64(c.NTPDrops)
		a.dropsCount++
		a.dropsSum += drops
		observeBucket(a.dropsBkts, clientsDropBuckets, drops)
	}

	// The time histograms cover only clients that have served NTP.
	if c.NTPHits == 0 {
		return
	}

	if c.LastNTPHitAgo != lastHitNever {
		lastSeen := float64(c.LastNTPHitAgo)
		a.lastHitCount++
		a.lastHitSum += lastSeen
		observeBucket(a.lastHitBkts, clientsTimeBuckets, lastSeen)
	}

	// NTPInterval is a log2 seconds value; skip the no-interval sentinel.
	if c.NTPInterval < intervalInvalid {
		interval := math.Exp2(float64(c.NTPInterval))
		a.intervalCount++
		a.intervalSum += interval
		observeBucket(a.intervalBkts, clientsTimeBuckets, interval)
	}
}

// collect emits the accumulated aggregate as prometheus metrics.
func (a *clientsAggregate) collect(ch chan<- prometheus.Metric) {
	ch <- clientsConnected.mustNewConstMetric(float64(a.ntpClients), "ntp")
	ch <- clientsConnected.mustNewConstMetric(float64(a.ntsClients), "nts")

	ch <- prometheus.MustNewConstHistogram(clientsLastNTPHitAgo, a.lastHitCount, a.lastHitSum, a.lastHitBkts)
	ch <- prometheus.MustNewConstHistogram(clientsNTPInterval, a.intervalCount, a.intervalSum, a.intervalBkts)
	ch <- prometheus.MustNewConstHistogram(clientsNTPDrops, a.dropsCount, a.dropsSum, a.dropsBkts)
}

func (e Exporter) getClientsMetrics(logger *slog.Logger, ch chan<- prometheus.Metric, client *chrony.Client) error {
	agg := newClientsAggregate()

	firstIndex := uint32(0)
	for {
		packet, err := client.Communicate(chrony.NewClientAccessesByIndexPacket(firstIndex, chrony.MaxClientAccessesByIndex, 0))
		if err != nil {
			return err
		}
		logger.Debug("Got 'clients' response", "clients_packet", packet.GetStatus())

		clients, ok := packet.(*chrony.ReplyClientAccessesByIndex)
		if !ok {
			return fmt.Errorf("got wrong 'clients' response: %q", packet)
		}

		for _, c := range clients.Clients {
			agg.add(c)
		}

		// A page can hold fewer than MaxClientAccessesByIndex clients while the
		// table still has slots to scan, so page until NextIndex reaches
		// NIndices (the table size), like chronyc does. The NextIndex <=
		// firstIndex check stops a non-advancing reply from looping forever.
		if clients.NextIndex >= clients.NIndices || clients.NextIndex <= firstIndex {
			break
		}
		firstIndex = clients.NextIndex
	}

	agg.collect(ch)

	return nil
}

// newBuckets returns a zero-initialized bucket map for the given upper bounds.
// Seeding every bound makes MustNewConstHistogram emit all "le" series even
// with no observations, keeping the series set stable across scrapes.
func newBuckets(bounds []float64) map[float64]uint64 {
	buckets := make(map[float64]uint64, len(bounds))
	for _, ub := range bounds {
		buckets[ub] = 0
	}
	return buckets
}

// observeBucket records value into its cumulative histogram buckets,
// incrementing every bound that covers it.
func observeBucket(buckets map[float64]uint64, bounds []float64, value float64) {
	for _, ub := range bounds {
		if value <= ub {
			buckets[ub]++
		}
	}
}
