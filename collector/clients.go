// Copyright 2026 Ben Kochie
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

// clientsBuckets are the upper bounds, in seconds, for the last-seen and
// NTP-interval histograms. They cover the 1 second to 1 day range requested in
// SuperQ/chrony_exporter#136.
var clientsBuckets = []float64{1, 5, 30, 60, 300, 1800, 3600, 21600, 86400}

var (
	clientsConnected = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "connected"),
			"The number of clients in chronyd's log. Saturates at the client log size (clientloglimit), at which point chronyd evicts the oldest records.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsNTPHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "ntp_hits"),
			"The number of NTP requests received from the clients currently in chronyd's log.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsNKEHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "nke_hits"),
			"The number of NTS-KE connections received from the clients currently in chronyd's log.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsCmdHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "cmd_hits"),
			"The number of command requests received from the clients currently in chronyd's log.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsNTPDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "ntp_drops"),
			"The number of NTP requests dropped (rate limited) for the clients currently in chronyd's log.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsNKEDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "nke_drops"),
			"The number of NTS-KE connections dropped (rate limited) for the clients currently in chronyd's log.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	clientsCmdDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, clientsSubsystem, "cmd_drops"),
			"The number of command requests dropped (rate limited) for the clients currently in chronyd's log.",
			nil,
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
)

// clientsAggregate accumulates per-client statistics across all pages of a
// 'clients' reply into the aggregate metrics exported by the collector.
type clientsAggregate struct {
	connected uint64
	ntpHits   uint64
	nkeHits   uint64
	cmdHits   uint64
	ntpDrops  uint64
	nkeDrops  uint64
	cmdDrops  uint64

	lastHitCount uint64
	lastHitSum   float64
	lastHitBkts  map[float64]uint64

	intervalCount uint64
	intervalSum   float64
	intervalBkts  map[float64]uint64
}

func newClientsAggregate() *clientsAggregate {
	return &clientsAggregate{
		lastHitBkts:  newBuckets(),
		intervalBkts: newBuckets(),
	}
}

// add folds a single client's statistics into the aggregate. Every client
// record counts towards "connected", matching the row count of `chronyc
// clients`, regardless of whether it has served an NTP request.
func (a *clientsAggregate) add(c chrony.ClientAccess) {
	a.connected++
	a.ntpHits += uint64(c.NTPHits)
	a.nkeHits += uint64(c.NKEHits)
	a.cmdHits += uint64(c.CmdHits)
	a.ntpDrops += uint64(c.NTPDrops)
	a.nkeDrops += uint64(c.NKEDrops)
	a.cmdDrops += uint64(c.CmdDrops)

	// The histograms cover only NTP clients.
	if c.NTPHits == 0 {
		return
	}

	if c.LastNTPHitAgo != lastHitNever {
		lastSeen := float64(c.LastNTPHitAgo)
		a.lastHitCount++
		a.lastHitSum += lastSeen
		observeBucket(a.lastHitBkts, lastSeen)
	}

	// NTPInterval is a log2 seconds value; skip the no-interval sentinel.
	if c.NTPInterval < intervalInvalid {
		interval := math.Exp2(float64(c.NTPInterval))
		a.intervalCount++
		a.intervalSum += interval
		observeBucket(a.intervalBkts, interval)
	}
}

// collect emits the accumulated aggregate as prometheus metrics.
func (a *clientsAggregate) collect(ch chan<- prometheus.Metric) {
	ch <- clientsConnected.mustNewConstMetric(float64(a.connected))
	ch <- clientsNTPHits.mustNewConstMetric(float64(a.ntpHits))
	ch <- clientsNKEHits.mustNewConstMetric(float64(a.nkeHits))
	ch <- clientsCmdHits.mustNewConstMetric(float64(a.cmdHits))
	ch <- clientsNTPDrops.mustNewConstMetric(float64(a.ntpDrops))
	ch <- clientsNKEDrops.mustNewConstMetric(float64(a.nkeDrops))
	ch <- clientsCmdDrops.mustNewConstMetric(float64(a.cmdDrops))

	ch <- prometheus.MustNewConstHistogram(clientsLastNTPHitAgo, a.lastHitCount, a.lastHitSum, a.lastHitBkts)
	ch <- prometheus.MustNewConstHistogram(clientsNTPInterval, a.intervalCount, a.intervalSum, a.intervalBkts)
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

// newBuckets returns a zero-initialized bucket map. Seeding every upper bound
// makes MustNewConstHistogram emit all "le" series even with no observations,
// keeping the series set stable across scrapes.
func newBuckets() map[float64]uint64 {
	buckets := make(map[float64]uint64, len(clientsBuckets))
	for _, ub := range clientsBuckets {
		buckets[ub] = 0
	}
	return buckets
}

// observeBucket records value into its cumulative histogram buckets,
// incrementing every bucket whose upper bound covers it.
func observeBucket(buckets map[float64]uint64, value float64) {
	for _, ub := range clientsBuckets {
		if value <= ub {
			buckets[ub]++
		}
	}
}
