// Copyright 2024 Ben Kochie
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

	"github.com/facebook/time/ntp/chrony"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	serverstatsSubsystem = "serverstats"
)

var (
	serverstatsNTPHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_packets_received_total"),
			"The number of valid NTP requests received by the server.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNKEHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "nts_ke_connections_accepted_total"),
			"The number of NTS-KE connections accepted by the server.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsCMDHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "command_packets_received_total"),
			"The number of command requests received by the server.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_packets_dropped_total"),
			"The number of NTP requests dropped by the server due to rate limiting.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNKEDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "nts-ke_connections_dropped_total"),
			"The number of NTS-KE connections dropped by the server due to rate limiting.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsCMDDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "command_packets_dropped_total"),
			"The number of command requests dropped by the server due to rate limiting.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsLogDrops = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "client_log_records_dropped_total"),
			"The number of client log records dropped by the server to limit the memory use.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPAuthHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "auhtenticated_ntp_packets_total"),
			"The number of received NTP requests that were authenticated (with a symmetric key or NTS).",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPInterleavedHits = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "interleaved_ntp_packets_total"),
			"The number of received NTP requests that were detected to be in the interleaved mode.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_timestamps_held"),
			"The number of pairs of receive and transmit timestamps that the server is currently holding in memory for clients using the interleaved mode.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	serverstatsNTPSpanSeconds = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_timestamp_span"),
			"The interval (in seconds) covered by the currently held NTP timestamps.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}
)

func (e Exporter) getServerstatsMetrics(ch chan<- prometheus.Metric, client chrony.Client) error {
	packet, err := client.Communicate(chrony.NewServerStatsPacket())
	if err != nil {
		return err
	}
	level.Debug(e.logger).Log("msg", "Got 'serverstats' response", "serverstats_packet", packet.GetStatus())

	serverstats, ok := packet.(*chrony.ReplyServerStats3)
	if !ok {
		return fmt.Errorf("Got wrong 'serverstats' response: %q", packet)
	}

	ch <- serverstatsNTPHits.mustNewConstMetric(float64(serverstats.NTPHits))
	level.Debug(e.logger).Log("msg", "Serverstats NTP Hits", "ntp_hits", serverstats.NTPHits)

	ch <- serverstatsNKEHits.mustNewConstMetric(float64(serverstats.NKEHits))
	level.Debug(e.logger).Log("msg", "Serverstats NKE Hits", "nke_hits", serverstats.NKEHits)

	ch <- serverstatsCMDHits.mustNewConstMetric(float64(serverstats.CMDHits))
	level.Debug(e.logger).Log("msg", "Serverstats CMD Hits", "cmd_hits", serverstats.CMDHits)

	ch <- serverstatsNTPDrops.mustNewConstMetric(float64(serverstats.NTPDrops))
	level.Debug(e.logger).Log("msg", "Serverstats NTP Drops", "ntp_drops", serverstats.NTPDrops)

	ch <- serverstatsNKEDrops.mustNewConstMetric(float64(serverstats.NKEDrops))
	level.Debug(e.logger).Log("msg", "Serverstats NKE Drops", "nke_drops", serverstats.NKEDrops)

	ch <- serverstatsCMDDrops.mustNewConstMetric(float64(serverstats.CMDDrops))
	level.Debug(e.logger).Log("msg", "Serverstats CMD Drops", "cmd_drops", serverstats.CMDDrops)

	ch <- serverstatsLogDrops.mustNewConstMetric(float64(serverstats.LogDrops))
	level.Debug(e.logger).Log("msg", "Serverstats Log Drops", "log_drops", serverstats.LogDrops)

	ch <- serverstatsNTPAuthHits.mustNewConstMetric(float64(serverstats.NTPAuthHits))
	level.Debug(e.logger).Log("msg", "Serverstats Authenticated Packets", "auth_hits", serverstats.NTPAuthHits)

	ch <- serverstatsNTPInterleavedHits.mustNewConstMetric(float64(serverstats.NTPInterleavedHits))
	level.Debug(e.logger).Log("msg", "Serverstats Interleaved Packets", "interleaved_hits", serverstats.NTPInterleavedHits)

	ch <- serverstatsNTPTimestamps.mustNewConstMetric(float64(serverstats.NTPTimestamps))
	level.Debug(e.logger).Log("msg", "Serverstats Timestamps Held", "ntp_timestamps_held", serverstats.NTPTimestamps)

	ch <- serverstatsNTPSpanSeconds.mustNewConstMetric(float64(serverstats.NTPSpanSeconds))
	level.Debug(e.logger).Log("msg", "Serverstats Timestamps Span", "ntp_timestamps_span", serverstats.NTPSpanSeconds)

	return nil
}
