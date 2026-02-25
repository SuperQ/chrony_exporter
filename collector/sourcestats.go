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
	"fmt"
	"log/slog"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	sourcestatsSubsystem = "sourcestats"
)

var (
	sourcestatsNSamples = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "samples"),
			"The number of sample points currently being retained for the server. (NP)",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcestatsNRuns = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "runs"),
			"The number of runs of residuals having the same sign following the last regression. (NR)",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcestatsSpanSeconds = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "span_seconds"),
			"The interval between the oldest and newest samples in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcestatsStdDeviation = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "stddev"),
			"The estimated sample standard deviation.",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcestatsFrequency = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "frequency_ppms"),
			"The estimated residual frequency for the server, in parts per million.",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcestatsFrequencySkew = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "frequency_skew_ppms"),
			"The estimated error bounds on Freq (again in parts per million).",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	sourcestatsOffset = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, sourcestatsSubsystem, "offset"),
			"The estimated offset of the source.",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}
)

func (e Exporter) getSourcestatsMetrics(logger *slog.Logger, ch chan<- prometheus.Metric, client *chrony.Client) error {
	packet, err := client.Communicate(chrony.NewSourcesPacket())
	if err != nil {
		return err
	}
	logger.Debug("Got 'sources' response", "sources_packet", packet.GetStatus())

	sources, ok := packet.(*chrony.ReplySources)
	if !ok {
		return fmt.Errorf("got wrong 'sources' response: %q", packet)
	}

	results := make([]chrony.ReplySourceStats, sources.NSources)

	for i := 0; i < int(sources.NSources); i++ {
		logger.Debug("Fetching source", "source_index", i)
		packet, err = client.Communicate(chrony.NewSourceStatsPacket(int32(i)))
		if err != nil {
			return fmt.Errorf("failed to get sourcestats response: %d", i)
		}
		sourceStats, ok := packet.(*chrony.ReplySourceStats)
		if !ok {
			return fmt.Errorf("got wrong 'sourcestats' response: %q", packet)
		}
		results[i] = *sourceStats
	}

	for _, r := range results {
		if r.IPAddr == nil {
			logger.Debug("Skipping source with nil IP address")
			continue
		}
		sourceAddress := r.IPAddr.String()
		sourceName := e.dnsLookup(logger, r.IPAddr)

		ch <- sourcestatsNSamples.mustNewConstMetric(float64(r.NSamples), sourceAddress, sourceName)
		ch <- sourcestatsNRuns.mustNewConstMetric(float64(r.NRuns), sourceAddress, sourceName)
		ch <- sourcestatsSpanSeconds.mustNewConstMetric(float64(r.SpanSeconds), sourceAddress, sourceName)
		ch <- sourcestatsStdDeviation.mustNewConstMetric(r.StandardDeviation, sourceAddress, sourceName)
		ch <- sourcestatsFrequency.mustNewConstMetric(r.ResidFreqPPM, sourceAddress, sourceName)
		ch <- sourcestatsFrequencySkew.mustNewConstMetric(r.SkewPPM, sourceAddress, sourceName)
		ch <- sourcestatsOffset.mustNewConstMetric(r.EstimatedOffset, sourceAddress, sourceName)
	}

	return nil
}
