// Copyright 2022 Ben Kochie
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
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"math/bits"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	sourcesSubsystem = "sources"
)

var (
	sourcesLastRx = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "last_sample_age_seconds"),
			"Chrony sources last good sample age in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesLastReachRatio = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "reachability_ratio"),
			"Chrony sources ratio of packet reachability",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesLastReachSuccess = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "reachability_success"),
			"Chrony sources last poll reachability success",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesLastSample = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "last_sample_offset_seconds"),
			"Chrony sources last sample offset in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesLastSampleErr = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "last_sample_error_margin_seconds"),
			"Chrony sources last sample margin of error in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesPollInterval = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "polling_interval_seconds"),
			"Chrony sources polling interval in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesStateInfo = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "state_info"),
			"Chrony sources state info",
			[]string{"source_address", "source_name", "source_state", "source_mode"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesStratum = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "stratum"),
			"Chrony sources stratum",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}
)

func (e Exporter) getSourcesMetrics(logger *slog.Logger, ch chan<- prometheus.Metric, client chrony.Client) error {
	packet, err := client.Communicate(chrony.NewSourcesPacket())
	if err != nil {
		return err
	}
	logger.Debug("Got 'sources' response", "sources_packet", packet.GetStatus())

	sources, ok := packet.(*chrony.ReplySources)
	if !ok {
		return fmt.Errorf("Got wrong 'sources' response: %q", packet)
	}

	results := make([]chrony.ReplySourceData, sources.NSources)

	for i := 0; i < int(sources.NSources); i++ {
		logger.Debug("Fetching source", "source_index", i)
		packet, err = client.Communicate(chrony.NewSourceDataPacket(int32(i)))
		if err != nil {
			return fmt.Errorf("Failed to get sourcedata response: %d", i)
		}
		sourceData, ok := packet.(*chrony.ReplySourceData)
		if !ok {
			return fmt.Errorf("Got wrong 'sourcedata' response: %q", packet)
		}
		results[i] = *sourceData
	}

	for _, r := range results {
		sourceAddress := r.IPAddr.String()
		sourceName := e.dnsLookup(logger, r.IPAddr)

		if r.Mode == chrony.SourceModeRef && r.IPAddr.To4() != nil {
			sourceName = chrony.RefidToString(binary.BigEndian.Uint32(r.IPAddr))
		}

		// Compute the reachability from the Reachability bits.
		lastReachRatio := float64(bits.OnesCount8(uint8(r.Reachability))) / 8.0
		lastReachSuccess := uint8(r.Reachability) & 1

		ch <- sourcesLastRx.mustNewConstMetric(float64(r.SinceSample), sourceAddress, sourceName)
		ch <- sourcesLastReachRatio.mustNewConstMetric(lastReachRatio, sourceAddress, sourceName)
		ch <- sourcesLastReachSuccess.mustNewConstMetric(float64(lastReachSuccess), sourceAddress, sourceName)
		ch <- sourcesLastSample.mustNewConstMetric(r.LatestMeas, sourceAddress, sourceName)
		ch <- sourcesLastSampleErr.mustNewConstMetric(r.LatestMeasErr, sourceAddress, sourceName)
		ch <- sourcesPollInterval.mustNewConstMetric(math.Pow(2, float64(r.Poll)), sourceAddress, sourceName)
		ch <- sourcesStateInfo.mustNewConstMetric(1.0, sourceAddress, sourceName, r.State.String(), r.Mode.String())
		ch <- sourcesStratum.mustNewConstMetric(float64(r.Stratum), sourceAddress, sourceName)
	}

	return nil
}
