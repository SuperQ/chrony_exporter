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
	"log/slog"

	"github.com/facebook/time/ntp/chrony"
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
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "nts_ke_connections_dropped_total"),
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
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "authenticated_ntp_packets_total"),
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
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_timestamp_span_seconds"),
			"The interval (in seconds) covered by the currently held NTP timestamps.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	serverstatsNTPDaemonRxTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_daemon_rx_timestamps_total"),
			"The number of NTP responses which included a receive timestamp captured by the daemon.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPDaemonTxTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_daemon_tx_timestamps_total"),
			"The number of NTP responses which included a transmit timestamp captured by the daemon.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPKernelRxTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_kernel_rx_timestamps_total"),
			"The number of NTP responses which included a receive timestamp captured by the kernel.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPKernelTxTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_kernel_tx_timestamps_total"),
			"The number of NTP responses (in the interleaved mode) which included a transmit timestamp captured by the kernel.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPHwRxTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_hw_rx_timestamps_total"),
			"The number of NTP responses which included a receive timestamp captured by the NIC.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}

	serverstatsNTPHwTxTimestamps = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, serverstatsSubsystem, "ntp_hw_tx_timestamps_total"),
			"The number of NTP responses (in the interleaved mode) which included a transmit timestamp captured by the NIC.",
			nil,
			nil,
		),
		prometheus.CounterValue,
	}
)

func parseServerStatsPacket(p chrony.ResponsePacket) (chrony.ReplyServerStats4, error) {
	var serverStats chrony.ReplyServerStats4
	switch stats := p.(type) {
	case *chrony.ReplyServerStats:
		serverStats.NTPHits = uint64(stats.NTPHits)
		serverStats.CMDHits = uint64(stats.CMDHits)
		serverStats.NTPDrops = uint64(stats.NTPDrops)
		serverStats.CMDDrops = uint64(stats.CMDDrops)
		serverStats.LogDrops = uint64(stats.LogDrops)
	case *chrony.ReplyServerStats2:
		serverStats.NTPHits = uint64(stats.NTPHits)
		serverStats.NKEHits = uint64(stats.NKEHits)
		serverStats.CMDHits = uint64(stats.CMDHits)
		serverStats.NTPDrops = uint64(stats.NTPDrops)
		serverStats.NKEDrops = uint64(stats.NKEDrops)
		serverStats.CMDDrops = uint64(stats.CMDDrops)
		serverStats.LogDrops = uint64(stats.LogDrops)
		serverStats.NTPAuthHits = uint64(stats.NTPAuthHits)
	case *chrony.ReplyServerStats3:
		serverStats.NTPHits = uint64(stats.NTPHits)
		serverStats.NKEHits = uint64(stats.NKEHits)
		serverStats.CMDHits = uint64(stats.CMDHits)
		serverStats.NTPDrops = uint64(stats.NTPDrops)
		serverStats.NKEDrops = uint64(stats.NKEDrops)
		serverStats.CMDDrops = uint64(stats.CMDDrops)
		serverStats.LogDrops = uint64(stats.LogDrops)
		serverStats.NTPAuthHits = uint64(stats.NTPAuthHits)
		serverStats.NTPInterleavedHits = uint64(stats.NTPInterleavedHits)
		serverStats.NTPTimestamps = uint64(stats.NTPTimestamps)
		serverStats.NTPSpanSeconds = uint64(stats.NTPSpanSeconds)
	case *chrony.ReplyServerStats4:
		serverStats = *stats
	default:
		return serverStats, fmt.Errorf("invalid 'serverstats' packet type: %q", p)
	}
	return serverStats, nil
}

func (e Exporter) getServerstatsMetrics(logger *slog.Logger, ch chan<- prometheus.Metric, client chrony.Client) error {
	packet, err := client.Communicate(chrony.NewServerStatsPacket())
	if err != nil {
		return err
	}
	logger.Debug("Got 'serverstats' response", "serverstats_packet", packet.GetStatus())

	serverstats, err := parseServerStatsPacket(packet)
	if err != nil {
		return fmt.Errorf("Unable to parse 'serverstats' packet: %w", err)
	}

	ch <- serverstatsNTPHits.mustNewConstMetric(float64(serverstats.NTPHits))
	logger.Debug("Serverstats NTP Hits", "ntp_hits", serverstats.NTPHits)

	ch <- serverstatsNKEHits.mustNewConstMetric(float64(serverstats.NKEHits))
	logger.Debug("Serverstats NKE Hits", "nke_hits", serverstats.NKEHits)

	ch <- serverstatsCMDHits.mustNewConstMetric(float64(serverstats.CMDHits))
	logger.Debug("Serverstats CMD Hits", "cmd_hits", serverstats.CMDHits)

	ch <- serverstatsNTPDrops.mustNewConstMetric(float64(serverstats.NTPDrops))
	logger.Debug("Serverstats NTP Drops", "ntp_drops", serverstats.NTPDrops)

	ch <- serverstatsNKEDrops.mustNewConstMetric(float64(serverstats.NKEDrops))
	logger.Debug("Serverstats NKE Drops", "nke_drops", serverstats.NKEDrops)

	ch <- serverstatsCMDDrops.mustNewConstMetric(float64(serverstats.CMDDrops))
	logger.Debug("Serverstats CMD Drops", "cmd_drops", serverstats.CMDDrops)

	ch <- serverstatsLogDrops.mustNewConstMetric(float64(serverstats.LogDrops))
	logger.Debug("Serverstats Log Drops", "log_drops", serverstats.LogDrops)

	ch <- serverstatsNTPAuthHits.mustNewConstMetric(float64(serverstats.NTPAuthHits))
	logger.Debug("Serverstats Authenticated Packets", "auth_hits", serverstats.NTPAuthHits)

	ch <- serverstatsNTPInterleavedHits.mustNewConstMetric(float64(serverstats.NTPInterleavedHits))
	logger.Debug("Serverstats Interleaved Packets", "interleaved_hits", serverstats.NTPInterleavedHits)

	ch <- serverstatsNTPTimestamps.mustNewConstMetric(float64(serverstats.NTPTimestamps))
	logger.Debug("Serverstats Timestamps Held", "ntp_timestamps_held", serverstats.NTPTimestamps)

	ch <- serverstatsNTPSpanSeconds.mustNewConstMetric(float64(serverstats.NTPSpanSeconds))
	logger.Debug("Serverstats Timestamps Span", "ntp_timestamps_span", serverstats.NTPSpanSeconds)

	ch <- serverstatsNTPDaemonRxTimestamps.mustNewConstMetric(float64(serverstats.NTPDaemonRxtimestamps))
	logger.Debug("Serverstats Daemon Rx Timestamps", "ntp_daemon_rx_timestamps", serverstats.NTPDaemonRxtimestamps)

	ch <- serverstatsNTPDaemonTxTimestamps.mustNewConstMetric(float64(serverstats.NTPDaemonTxtimestamps))
	logger.Debug("Serverstats Daemon Tx Timestamps", "ntp_daemon_tx_timestamps", serverstats.NTPDaemonTxtimestamps)

	ch <- serverstatsNTPKernelRxTimestamps.mustNewConstMetric(float64(serverstats.NTPKernelRxtimestamps))
	logger.Debug("Serverstats Kernel Rx Timestamps", "ntp_kernel_rx_timestamps", serverstats.NTPKernelRxtimestamps)

	ch <- serverstatsNTPKernelTxTimestamps.mustNewConstMetric(float64(serverstats.NTPKernelTxtimestamps))
	logger.Debug("Serverstats Kernel Tx Timestamps", "ntp_kernel_tx_timestamps", serverstats.NTPKernelTxtimestamps)

	ch <- serverstatsNTPHwRxTimestamps.mustNewConstMetric(float64(serverstats.NTPHwRxTimestamps))
	logger.Debug("Serverstats Hardware Rx Timestamps", "ntp_hw_rx_timestamps", serverstats.NTPHwRxTimestamps)

	ch <- serverstatsNTPHwTxTimestamps.mustNewConstMetric(float64(serverstats.NTPHwTxTimestamps))
	logger.Debug("Serverstats Hardware Tx Timestamps", "ntp_hw_tx_timestamps", serverstats.NTPHwTxTimestamps)

	return nil
}
