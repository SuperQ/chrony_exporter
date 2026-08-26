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
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/facebook/time/ntp/chrony"
	"github.com/google/uuid"

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
	collectSourcestats bool
	collectNtpdata     bool
	collectTracking    bool
	collectServerstats bool
	collectClients     bool
	chmodSocket        bool
	dnsLookups         bool
	resolver           *reverseResolver
	systemLookup       func(address string) (string, error)
	dnsCache           *dnsCache
	pathologyWarner    *pathologyWarner

	logger *slog.Logger
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
	// DNSPTRCacheMinTTL sets the floor for how long a reverse DNS lookup is
	// cached, raising short/absent record TTLs up to at least this long. A
	// time.Duration (ms, s, m, h) - not just seconds.
	DNSPTRCacheMinTTL time.Duration

	// CollectSources will configure the exporter to collect `chronyc sources`.
	CollectSources bool
	// CollectSourcestats will configure the exporter to collect `chronyc sourcestats`.
	CollectSourcestats bool
	// CollectNtpData will configure the exporter to extend sources info with `chronyc ntpdata`
	CollectNtpdata bool
	// CollectTracking will configure the exporter to collect `chronyc tracking`.
	CollectTracking bool
	// CollectServerstats will configure the exporter to collect `chronyc serverstats`.
	CollectServerstats bool
	// CollectClients will configure the exporter to collect `chronyc clients`.
	CollectClients bool
}

func NewExporter(conf ChronyCollectorConfig, logger *slog.Logger) Exporter {
	var resolver *reverseResolver
	var systemLookup func(string) (string, error)
	if conf.DNSLookups {
		var err error
		resolver, err = newReverseResolver()
		if err != nil {
			logger.Warn("failed to load resolver config: direct DNS queries for record TTLs are disabled; "+
				"reverse lookups will use --collector.dns-ptr-cache-min-ttl (default 0s) as a fixed cache duration instead of a real TTL",
				"err", err)
		}
		systemLookup = systemReverseLookup
	}

	return Exporter{
		address: conf.Address,
		timeout: conf.Timeout,

		collectSources:     conf.CollectSources,
		collectSourcestats: conf.CollectSourcestats,
		collectNtpdata:     conf.CollectNtpdata,
		collectTracking:    conf.CollectTracking,
		collectServerstats: conf.CollectServerstats,
		collectClients:     conf.CollectClients,
		chmodSocket:        conf.ChmodSocket,
		dnsLookups:         conf.DNSLookups,
		resolver:           resolver,
		systemLookup:       systemLookup,
		dnsCache:           newDNSCache(conf.DNSPTRCacheMinTTL),
		pathologyWarner:    newPathologyWarner(),

		logger: logger,
	}
}

// Describe implements prometheus.Collector.
func (e Exporter) Describe(ch chan<- *prometheus.Desc) {
}

func (e Exporter) dial() (net.Conn, func(), error) {
	if remote, ok := strings.CutPrefix(e.address, "unix://"); ok {
		base, _ := path.Split(remote)
		local := path.Join(base, fmt.Sprintf("chrony_exporter.%s.sock", uuid.New()))
		conn, err := net.DialUnix("unixgram",
			&net.UnixAddr{Name: local, Net: "unixgram"},
			&net.UnixAddr{Name: remote, Net: "unixgram"},
		)
		if err != nil {
			return nil, func() { os.Remove(local) }, err
		}
		if e.chmodSocket {
			if err := os.Chmod(local, 0666); err != nil {
				return nil, func() { conn.Close(); os.Remove(local) }, err
			}
		}
		err = conn.SetReadDeadline(time.Now().Add(e.timeout))
		if err != nil {
			e.logger.Debug("Couldn't set read-timeout for unix datagram socket", "err", err)
		}
		return conn, func() { conn.Close(); os.Remove(local) }, nil
	}

	conn, err := net.DialTimeout("udp", e.address, e.timeout)
	return conn, func() {}, err
}

// Collect implements prometheus.Collector.
func (e Exporter) Collect(ch chan<- prometheus.Metric) {
	logger := e.logger.With("scrape_id", scrapeID.Add(1))
	start := time.Now()
	logger.Debug("Scrape starting")
	var up float64
	defer func() {
		logger.Debug("Scrape completed", "seconds", time.Since(start).Seconds())
		ch <- upMetric.mustNewConstMetric(up)
	}()
	conn, cleanup, err := e.dial()
	defer cleanup()
	if err != nil {
		logger.Debug("Couldn't connect to chrony", "address", e.address, "err", err)
		return
	}

	up = 1

	client := chrony.Client{Sequence: 1, Connection: conn}

	if e.collectSources {
		err = e.getSourcesMetrics(logger, ch, &client, e.collectNtpdata)
		if err != nil {
			logger.Debug("Couldn't get sources", "err", err)
			up = 0
		}
	}

	if e.collectSourcestats {
		err = e.getSourcestatsMetrics(logger, ch, &client)
		if err != nil {
			logger.Debug("Couldn't get sourcestats", "err", err)
			up = 0
		}
	}

	if e.collectTracking {
		err = e.getTrackingMetrics(logger, ch, &client)
		if err != nil {
			logger.Debug("Couldn't get tracking", "err", err)
			up = 0
		}
	}

	if e.collectServerstats {
		err = e.getServerstatsMetrics(logger, ch, &client)
		if err != nil {
			logger.Debug("Couldn't get serverstats", "err", err)
			up = 0
		}
	}

	if e.collectClients {
		err = e.getClientsMetrics(logger, ch, &client)
		if err != nil {
			logger.Debug("Couldn't get clients", "err", err)
			up = 0
		}
	}
}

// dnsLookup resolves address via two resolvers behind a cache:
// systemLookup, the name authority, and e.resolver, which exists only to
// supply a TTL net.LookupAddr can't. e.resolver's TTL is trusted only on
// namesAgree with systemLookup's answer; otherwise the configured floor
// applies. Both run at most once per cache entry, not per scrape.
func (e Exporter) dnsLookup(logger *slog.Logger, address net.IP) string {
	if !e.dnsLookups {
		return address.String()
	}
	addr := address.String()

	return e.dnsCache.lookup(addr, func() (string, time.Duration) {
		start := time.Now()
		defer func() {
			logger.Debug("DNS lookup took", "seconds", time.Since(start).Seconds())
		}()

		var dnsName string
		var ttl time.Duration
		if e.resolver != nil {
			name, recordTTL, err := e.resolver.lookup(addr)
			// ttl may be a negative-cache TTL even on error; 0 means no
			// usable TTL at all (hard failure, not a real negative answer).
			ttl = recordTTL
			if err != nil {
				logger.Debug("direct DNS lookup failed", "address", addr, "err", err)
			} else {
				dnsName = name
			}
		}

		sysName, sysErr := e.systemLookup(addr)
		if sysErr != nil {
			logger.Debug("system reverse lookup failed", "address", addr, "err", sysErr)
		}

		switch {
		case sysErr == nil && namesAgree(sysName, dnsName):
			return sysName, ttl
		case sysErr == nil:
			// ttl describes dnsName's record, not sysName's - don't reuse it.
			if dnsName != "" {
				logger.Debug("system resolver disagrees with direct DNS lookup, using the system's answer without the DNS record's TTL",
					"address", addr, "dns_name", dnsName, "system_name", sysName)
				e.pathologyWarner.warnOnce("mismatch:"+addr, func() {
					logger.Warn("pathological reverse DNS lookup: system resolver and direct DNS query keep disagreeing, "+
						"falling back to the configured min-ttl floor for this address (once-per-process notice)",
						"address", addr, "dns_name", dnsName, "system_name", sysName, "min_ttl_floor", e.dnsCache.minTTL)
				})
			}
			return sysName, 0
		default:
			// systemLookup has nothing; dnsName is never surfaced here (see
			// doc comment above). ttl still paces retries when non-zero.
			if ttl == 0 {
				e.pathologyWarner.warnOnce("no-ttl:"+addr, func() {
					logger.Warn("pathological reverse DNS lookup: neither resolver could resolve this address and DNS gave no "+
						"usable TTL, falling back to the configured min-ttl floor (once-per-process notice)",
						"address", addr, "min_ttl_floor", e.dnsCache.minTTL)
				})
			}
			return addr, ttl
		}
	})
}
