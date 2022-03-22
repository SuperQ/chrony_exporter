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
	"math"
	"net"
	"strings"

	"github.com/facebook/time/ntp/chrony"
	"github.com/go-kit/log/level"
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

	sourcesLastSample = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "last_sample_offset_seconds"),
			"Chrony sources last sample margin of error in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcesLastSampleErr = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcesSubsystem, "last_sample_error_margin_seconds"),
			"Chrony sources last sample offset in seconds",
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

func (e Exporter) getSourcesMetrics(ch chan<- prometheus.Metric, client chrony.Client) {
	packet, err := client.Communicate(chrony.NewSourcesPacket())
	if err != nil {
		level.Error(e.logger).Log("msg", "Couldn't get sources", "err", err)
		return
	}
	level.Debug(e.logger).Log("msg", "Got 'sources' response", "sources_packet", packet.GetStatus())

	sources, ok := packet.(*chrony.ReplySources)
	if !ok {
		level.Error(e.logger).Log("msg", "Got wrong 'sources' response", "packet", packet)
		return
	}

	results := make([]chrony.ReplySourceData, sources.NSources)

	for i := 0; i < int(sources.NSources); i++ {
		level.Debug(e.logger).Log("msg", "Fetching source", "source", i)
		packet, err = client.Communicate(chrony.NewSourceDataPacket(int32(i)))
		if err != nil {
			level.Error(e.logger).Log("msg", "Failed to get sourcedata response", "source", i)
			return
		}
		sourceData, ok := packet.(*chrony.ReplySourceData)
		if !ok {
			level.Error(e.logger).Log("msg", "Got wrong 'sourcedata' response", "packet", packet)
			return
		}
		results[i] = *sourceData
	}

	for _, r := range results {
		sourceAddress := r.IPAddr.String()
		// Ignore reverse lookup errors.
		sourceNames, _ := net.LookupAddr(sourceAddress)
		sourceName := strings.Join(sourceNames, ",")

		if r.Mode == chrony.SourceModeRef && r.IPAddr.To4() != nil {
			sourceName = chrony.RefidToString(binary.BigEndian.Uint32(r.IPAddr))
		}

		ch <- sourcesLastRx.mustNewConstMetric(float64(r.SinceSample), sourceAddress, sourceName)
		ch <- sourcesLastSample.mustNewConstMetric(r.LatestMeas, sourceAddress, sourceName)
		ch <- sourcesLastSampleErr.mustNewConstMetric(r.LatestMeasErr, sourceAddress, sourceName)
		ch <- sourcesPollInterval.mustNewConstMetric(math.Pow(2, float64(r.Poll)), sourceAddress, sourceName)
		ch <- sourcesStateInfo.mustNewConstMetric(1.0, sourceAddress, sourceName, r.State.String(), r.Mode.String())
		ch <- sourcesStratum.mustNewConstMetric(float64(r.Stratum), sourceAddress, sourceName)
	}
}
