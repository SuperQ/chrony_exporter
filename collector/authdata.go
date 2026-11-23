// Copyright Ben Kochie
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
	"strconv"

	"github.com/facebook/time/ntp/chrony"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	authdataSubsystem = "authdata"
)

var (
	authdataInfo = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "info"),
			"Chrony source authentication info, mode is none, symmetric or nts",
			[]string{"source_address", "source_name", "mode", "key_id", "key_type"},
			nil,
		),
		prometheus.GaugeValue,
	}

	authdataKeyLength = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "key_length_bits"),
			"Chrony source authentication key length in bits",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	authdataKEAttempts = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "ke_attempts"),
			"Chrony source NTS-KE attempts since the last successful key exchange",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	authdataLastKEAge = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "last_ke_age_seconds"),
			"Chrony source time since the last successful NTS-KE in seconds",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	authdataCookies = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "cookies"),
			"Chrony source number of NTS cookies held",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	authdataCookieLength = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "cookie_length_bytes"),
			"Chrony source length of the next NTS cookie in bytes",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}

	authdataNAK = typedDesc{
		prometheus.NewDesc(
			prometheus.BuildFQName(namespace, authdataSubsystem, "nak"),
			"Chrony source whether the last NTS response was a NAK",
			[]string{"source_address", "source_name"},
			nil,
		),
		prometheus.GaugeValue,
	}
)

func (e Exporter) getAuthdataMetrics(logger *slog.Logger, ch chan<- prometheus.Metric, client *chrony.Client) error {
	packet, err := client.Communicate(chrony.NewSourcesPacket())
	if err != nil {
		return err
	}
	logger.Debug("Got 'sources' response", "sources_packet", packet.GetStatus())

	sources, ok := packet.(*chrony.ReplySources)
	if !ok {
		return fmt.Errorf("got wrong 'sources' response: %q", packet)
	}

	for i := 0; i < sources.NSources; i++ {
		logger.Debug("Fetching source", "source_index", i)
		packet, err = client.Communicate(chrony.NewSourceDataPacket(int32(i)))
		if err != nil {
			return fmt.Errorf("failed to get sourcedata response: %d", i)
		}
		sourceData, ok := packet.(*chrony.ReplySourceData)
		if !ok {
			return fmt.Errorf("got wrong 'sourcedata' response: %q", packet)
		}

		// Like chronyc authdata: reference clocks have no authentication
		// and unresolved sources can't be identified.
		if sourceData.Mode == chrony.SourceModeRef {
			continue
		}
		if sourceData.IPAddr.Family == chrony.IPAddrID {
			logger.Debug("Skipping unresolved IP address", "address", sourceData.IPAddr.String())
			continue
		}

		packet, err = client.Communicate(chrony.NewAuthDataPacket(sourceData.IPAddr))
		if err != nil {
			return fmt.Errorf("failed to get authdata response for: %s: %w", sourceData.IPAddr, err)
		}
		authData, ok := packet.(*chrony.ReplyAuthData)
		if !ok {
			return fmt.Errorf("got wrong 'authdata' response: %q", packet)
		}

		sourceAddress := sourceData.IPAddr.String()
		sourceName := e.dnsLookup(logger, sourceData.IPAddr.ToNetIP())
		emitAuthdataMetrics(ch, sourceAddress, sourceName, &authData.AuthData)
	}

	return nil
}

func emitAuthdataMetrics(ch chan<- prometheus.Metric, sourceAddress, sourceName string, a *chrony.AuthData) {
	ch <- authdataInfo.mustNewConstMetric(1.0, sourceAddress, sourceName,
		a.Mode.String(),
		strconv.FormatUint(uint64(a.KeyID), 10),
		strconv.FormatUint(uint64(a.KeyType), 10),
	)
	if a.Mode == chrony.AuthModeNone {
		return
	}
	ch <- authdataKeyLength.mustNewConstMetric(float64(a.KeyLength), sourceAddress, sourceName)
	if a.Mode != chrony.AuthModeNTS {
		return
	}
	ch <- authdataKEAttempts.mustNewConstMetric(float64(a.KEAttempts), sourceAddress, sourceName)
	if a.LastKEAgo != chrony.AuthDataNoKeyExchange {
		ch <- authdataLastKEAge.mustNewConstMetric(float64(a.LastKEAgo), sourceAddress, sourceName)
	}
	ch <- authdataCookies.mustNewConstMetric(float64(a.Cookies), sourceAddress, sourceName)
	ch <- authdataCookieLength.mustNewConstMetric(float64(a.CookieLength), sourceAddress, sourceName)
	nak := 0.0
	if a.NAK {
		nak = 1.0
	}
	ch <- authdataNAK.mustNewConstMetric(nak, sourceAddress, sourceName)
}
