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

package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/superq/chrony_exporter/collector"

	kingpin "github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/client_golang/prometheus"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

var (
	conf   = collector.ChronyCollectorConfig{}
	logger *slog.Logger
)

func main() {
	kingpin.Flag(
		"chrony.address",
		"Address of the Chrony srever.",
	).Default("[::1]:323").StringVar(&conf.Address)

	kingpin.Flag(
		"chrony.timeout",
		"Timeout on requests to the Chrony srever.",
	).Default("5s").DurationVar(&conf.Timeout)

	kingpin.Flag(
		"collector.tracking",
		"Collect tracking metrics",
	).Default("true").BoolVar(&conf.CollectTracking)

	kingpin.Flag(
		"collector.sources",
		"Collect sources metrics",
	).Default("false").BoolVar(&conf.CollectSources)

	kingpin.Flag(
		"collector.sources.with-ntpdata",
		"Extend sources with ntpdata metrics (requires socket connection)",
	).Default("false").BoolVar(&conf.CollectNtpdata)

	kingpin.Flag(
		"collector.serverstats",
		"Collect serverstats metrics",
	).Default("false").BoolVar(&conf.CollectServerstats)

	kingpin.Flag(
		"collector.chmod-socket",
		"Chmod 0666 the receiving unix datagram socket",
	).Default("false").BoolVar(&conf.ChmodSocket)

	kingpin.Flag(
		"collector.dns-lookups", "do reverse DNS lookups",
	).Default("true").BoolVar(&conf.DNSLookups)

	metricsPath := kingpin.Flag(
		"web.telemetry-path",
		"Path under which to expose metrics.",
	).Default("/metrics").String()

	toolkitFlags := kingpinflag.AddFlags(kingpin.CommandLine, ":9123")

	promslogConfig := &promslog.Config{}
	flag.AddFlags(kingpin.CommandLine, promslogConfig)
	kingpin.CommandLine.UsageWriter(os.Stdout)
	kingpin.HelpFlag.Short('h')
	kingpin.Version(version.Print("chrony_exporter"))
	kingpin.Parse()

	logger = promslog.New(promslogConfig)
	logger.Info("Starting chrony_exporter", "version", version.Info())
	prometheus.MustRegister(versioncollector.NewCollector("chrony_exporter"))

	exporter := collector.NewExporter(conf, logger)
	prometheus.MustRegister(exporter)

	http.Handle(*metricsPath, promhttp.Handler())
	if *metricsPath != "/" && *metricsPath != "" {
		landingConfig := web.LandingConfig{
			Name:        "Chrony Exporter",
			Description: "Prometheus Exporter for Chrony NTP",
			Version:     version.Info(),
			Links: []web.LandingLinks{
				{
					Address: *metricsPath,
					Text:    "Metrics",
				},
				{
					Address: "https://chrony-project.org/",
					Text:    "Chrony NTP",
				},
			},
		}
		landingPage, err := web.NewLandingPage(landingConfig)
		if err != nil {
			logger.Error("error creating landing page", "err", err)
			os.Exit(1)
		}
		http.Handle("/", landingPage)
	}

	server := &http.Server{}
	if err := web.ListenAndServe(server, toolkitFlags, logger); err != nil {
		logger.Error("HTTP listener stopped", "error", err)
		os.Exit(1)
	}
}
