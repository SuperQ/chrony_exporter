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
	"net"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	trackingSubsystem = "tracking"
)

var (
	// The remote IP 127.127.1.1 means it is a "local" reference clock.
	trackingLocalIP = net.IPv4(127, 127, 1, 1)

	trackingInfo = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "info"),
			"Chrony tracking info",
			[]string{"tracking_address", "tracking_name", "tracking_refid"},
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingLastOffset = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "last_offset_seconds"),
			"Chrony tracking last offset in seconds",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingRefTime = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "reference_timestamp_seconds"),
			"Chrony tracking Reference timestamp",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingSystemTime = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "system_time_seconds"),
			"Chrony tracking System time",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingRemoteTracking = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "remote_reference"),
			"Chrony tracking is connected to a remote source",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingRMSOffset = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "rms_offset_seconds"),
			"Chrony tracking long-term average of the offset",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingRootDelay = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "root_delay_seconds"),
			"This is the total of the network path delays to the stratum-1 computer from which the computer is ultimately synchronised",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingRootDispersion = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "root_dispersion_seconds"),
			"Chrony tracking total of all measurement errors to the NTP root",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingFrequency = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "frequency_ppms"),
			"Rate by which the system's clock would be wrong if chronyd was not correcting it, in PPMs",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingResidualFrequency = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "residual_frequency_ppms"),
			"For the currently selected reference source, the difference between the frequency it suggests and the one currently in use, in PPMs",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingSkew = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "skew_ppms"),
			"The estimated error bound on the frequency, in PPMs",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingUpdateInterval = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "update_interval_seconds"),
			"The time elapsed since the last measurement from the reference source was processed, in seconds",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	trackingStratum = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "stratum"),
			"Chrony tracking client stratum",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}
)

func (e Exporter) trackingFormatName(logger *slog.Logger, tracking chrony.Tracking) string {
	if tracking.IPAddr.IsUnspecified() {
		return chrony.RefidToString(tracking.RefID)
	}
	return e.dnsLookup(logger, tracking.IPAddr)
}

func (e Exporter) getTrackingMetrics(logger *slog.Logger, ch chan<- prometheus.Metric, client chrony.Client) error {
	packet, err := client.Communicate(chrony.NewTrackingPacket())
	if err != nil {
		return err
	}
	logger.Debug("Got 'tracking' response", "tracking_packet", packet.GetStatus())

	tracking, ok := packet.(*chrony.ReplyTracking)
	if !ok {
		return fmt.Errorf("got wrong 'tracking' response: %q", packet)
	}

	ch <- trackingInfo.mustNewConstMetric(1.0, tracking.IPAddr.String(), e.trackingFormatName(logger, tracking.Tracking), chrony.RefidAsHEX(tracking.RefID))

	ch <- trackingLastOffset.mustNewConstMetric(tracking.LastOffset)
	logger.Debug("Tracking Last Offset", "offset", tracking.LastOffset)

	ch <- trackingRefTime.mustNewConstMetric(float64(tracking.RefTime.UnixNano()) / 1e9)
	logger.Debug("Tracking Ref Time", "ref_time", tracking.RefTime)

	ch <- trackingSystemTime.mustNewConstMetric(float64(tracking.CurrentCorrection))
	logger.Debug("Tracking System Time", "system_time", tracking.CurrentCorrection)

	remoteTracking := 1.0
	if tracking.IPAddr.Equal(trackingLocalIP) {
		remoteTracking = 0.0
	}
	ch <- trackingRemoteTracking.mustNewConstMetric(remoteTracking)
	logger.Debug("Tracking is remote", "bool_value", remoteTracking)

	ch <- trackingRMSOffset.mustNewConstMetric(tracking.RMSOffset)
	logger.Debug("Tracking RMS Offset", "rms_offset", tracking.RMSOffset)

	ch <- trackingRootDelay.mustNewConstMetric(tracking.RootDelay)
	logger.Debug("Tracking Root delay", "root_delay", tracking.RootDelay)

	ch <- trackingRootDispersion.mustNewConstMetric(tracking.RootDispersion)
	logger.Debug("Tracking Root dispersion", "root_dispersion", tracking.RootDispersion)

	ch <- trackingFrequency.mustNewConstMetric(tracking.FreqPPM)
	logger.Debug("Tracking Frequency", "frequency", tracking.FreqPPM)

	ch <- trackingResidualFrequency.mustNewConstMetric(tracking.ResidFreqPPM)
	logger.Debug("Tracking Residual Frequency", "residual_frequency", tracking.ResidFreqPPM)

	ch <- trackingSkew.mustNewConstMetric(tracking.SkewPPM)
	logger.Debug("Tracking Skew", "skew", tracking.SkewPPM)

	ch <- trackingUpdateInterval.mustNewConstMetric(tracking.LastUpdateInterval)
	logger.Debug("Tracking Last Update Interval", "update_interval", tracking.LastUpdateInterval)

	ch <- trackingStratum.mustNewConstMetric(float64(tracking.Stratum))
	logger.Debug("Tracking Stratum", "stratum", tracking.Stratum)

	return nil
}
