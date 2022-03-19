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
	"github.com/facebook/time/ntp/chrony"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	trackingSubsystem = "tracking"
)

var (
	trackingLastOffset = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, trackingSubsystem, "last_offset_seconds"),
			"Chrony tracking last offset in seconds",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}
)

func (e Exporter) getTrackingMetrics(ch chan<- prometheus.Metric, client chrony.Client) {
	packet, err := client.Communicate(chrony.NewTrackingPacket())
	if err != nil {
		level.Error(e.logger).Log("msg", "Couldn't get tracking", "err", err)
		return
	}
	level.Debug(e.logger).Log("msg", "Got 'tracking' response", "tracking_packet", packet.GetStatus())

	tracking, ok := packet.(*chrony.ReplyTracking)
	if !ok {
		level.Error(e.logger).Log("msg", "Got wrong 'tracking' response", "packet", packet)
		return
	}

	ch <- trackingLastOffset.mustNewConstMetric(tracking.LastOffset)
	level.Debug(e.logger).Log("msg", "Last Offset", "offset", tracking.LastOffset)
}
