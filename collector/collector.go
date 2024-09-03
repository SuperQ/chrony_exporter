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
	stdlog "log"
	"net"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/facebook/time/ntp/chrony"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "chrony"
)

var (
	upMetric = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Whether the chrony server is up.",
			nil,
			nil,
		),
		prometheus.GaugeValue,
	}

	// Globally track scrapes to provide better logging context.
	scrapeID atomic.Uint64
)

// Exporter collects chrony stats from the given server and exports
// them using the prometheus metrics package.
type Exporter struct {
	address string
	timeout time.Duration

	collectSources     bool
	collectTracking    bool
	collectServerstats bool
	chmodSocket        bool
	dnsLookups         bool

	logger log.Logger
}

type typedDesc struct {
	desc      *prometheus.Desc
	valueType prometheus.ValueType
}

func (d *typedDesc) mustNewConstMetric(value float64, labels ...string) prometheus.Metric {
	return prometheus.MustNewConstMetric(d.desc, d.valueType, value, labels...)
}

// ChronyCollectorConfig configures the exporter parameters.
type ChronyCollectorConfig struct {
	// Address is the Chrony server UDP command port.
	Address string
	// Timeout configures the socket timeout to the Chrony server.
	Timeout time.Duration

	// ChmodSocket will set the unix datagram socket to mode `0666` when true.
	ChmodSocket bool
	// DNSLookups will reverse resolve IP addresses to names when true.
	DNSLookups bool

	// CollectSources will configure the exporter to collect `chronyc sources`.
	CollectSources bool
	// CollectTracking will configure the exporter to collect `chronyc tracking`.
	CollectTracking bool
	// CollectServerstats will configure the exporter to collect `chronyc serverstats`.
	CollectServerstats bool
}

func NewExporter(conf ChronyCollectorConfig, logger log.Logger) Exporter {
	return Exporter{
		address: conf.Address,
		timeout: conf.Timeout,

		collectSources:     conf.CollectSources,
		collectTracking:    conf.CollectTracking,
		collectServerstats: conf.CollectServerstats,
		chmodSocket:        conf.ChmodSocket,
		dnsLookups:         conf.DNSLookups,

		logger: logger,
	}
}

// Describe implements prometheus.Collector.
func (e Exporter) Describe(ch chan<- *prometheus.Desc) {
}

func (e Exporter) dial() (net.Conn, error, func()) {
	if strings.HasPrefix(e.address, "unix://") {
		remote := strings.TrimPrefix(e.address, "unix://")
		base, _ := path.Split(remote)
		local := path.Join(base, fmt.Sprintf("chrony_exporter.%d.sock", os.Getpid()))
		conn, err := net.DialUnix("unixgram",
			&net.UnixAddr{Name: local, Net: "unixgram"},
			&net.UnixAddr{Name: remote, Net: "unixgram"},
		)
		if err != nil {
			return nil, err, func() { os.Remove(local) }
		}
		if e.chmodSocket {
			if err := os.Chmod(local, 0666); err != nil {
				return nil, err, func() { conn.Close(); os.Remove(local) }
			}
		}
		err = conn.SetReadDeadline(time.Now().Add(e.timeout))
		if err != nil {
			level.Debug(e.logger).Log("msg", "Couldn't set read-timeout for unix datagram socket", "err", err)
		}
		return conn, nil, func() { conn.Close(); os.Remove(local) }
	}

	conn, err := net.DialTimeout("udp", e.address, e.timeout)
	return conn, err, func() {}
}

// Collect implements prometheus.Collector.
func (e Exporter) Collect(ch chan<- prometheus.Metric) {
	logger := log.With(e.logger, "scrape_id", scrapeID.Add(1))
	start := time.Now()
	level.Debug(logger).Log("msg", "Scrape starting")
	var up float64
	defer func() {
		level.Debug(logger).Log("msg", "Scrape completed", "seconds", time.Since(start).Seconds())
		ch <- upMetric.mustNewConstMetric(up)
	}()
	conn, err, cleanup := e.dial()
	defer cleanup()
	if err != nil {
		level.Debug(logger).Log("msg", "Couldn't connect to chrony", "address", e.address, "err", err)
		return
	}

	up = 1

	client := chrony.Client{Sequence: 1, Connection: conn}

	chrony.Logger = stdlog.New(log.NewStdlibAdapter(level.Debug(logger)), "", 0)

	if e.collectSources {
		err = e.getSourcesMetrics(logger, ch, client)
		if err != nil {
			level.Debug(logger).Log("msg", "Couldn't get sources", "err", err)
			up = 0
		}
	}

	if e.collectTracking {
		err = e.getTrackingMetrics(logger, ch, client)
		if err != nil {
			level.Debug(logger).Log("msg", "Couldn't get tracking", "err", err)
			up = 0
		}
	}

	if e.collectServerstats {
		err = e.getServerstatsMetrics(logger, ch, client)
		if err != nil {
			level.Debug(logger).Log("msg", "Couldn't get serverstats", "err", err)
			up = 0
		}
	}
}

func (e Exporter) dnsLookup(logger log.Logger, address net.IP) string {
	start := time.Now()
	defer func() {
		level.Debug(logger).Log("msg", "DNS lookup took", "seconds", time.Since(start).Seconds())
	}()
	if !e.dnsLookups {
		return address.String()
	}
	names, err := net.LookupAddr(address.String())
	if err != nil || len(names) < 1 {
		return address.String()
	}
	for i, name := range names {
		names[i] = strings.TrimRight(name, ".")
	}
	sort.Strings(names)
	return strings.Join(slices.Compact(names), ",")
}
