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
	"net"
	"time"

	"github.com/facebook/time/ntp/chrony"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/alecthomas/kingpin.v2"
)

const (
	namespace = "chrony"
)

var (
	collectTracking = kingpin.Flag("collector.tracking", "Collect tracking metrics").Default("true").Bool()
	collectSources  = kingpin.Flag("collector.sources", "Collect sources metrics").Default("false").Bool()
)

// Exporter collects chrony stats from the given server and exports
// them using the prometheus metrics package.
type Exporter struct {
	address string
	timeout time.Duration

	collectSources  bool
	collectTracking bool

	logger log.Logger
}

type typedDesc struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

func (d *typedDesc) mustNewConstMetric(value float64, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(d.desc, d.valueType, value, labels...)
}

func NewExporter(address string, logger log.Logger) Exporter {

	return Exporter{
		address: address,
		timeout: 5 * time.Second,

		collectSources:  *collectSources,
		collectTracking: *collectTracking,

		logger: logger,
	}
}

// Describe implements prometheus.Collector.
func (e Exporter) Describe(ch chan<- *prometheus.Desc) {
}

// Collect implements prometheus.Collector.
func (e Exporter) Collect(ch chan<- prometheus.Metric) {
	conn, err := net.DialTimeout("udp", e.address, e.timeout)
	if err != nil {
		level.Info(e.logger).Log("msg", "Couldn't dial UDP", "address", e.address)
		return
	}

	client := chrony.Client{Sequence: 1, Connection: conn}

	if e.collectSources {
		e.getSourcesMetrics(ch, client)
	}

	if e.collectTracking {
		e.getTrackingMetrics(ch, client)
	}
}
