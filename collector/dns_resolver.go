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
	"net"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// resolvConfPath is a var, not a const, so tests can point it at a temp
// file to exercise newReverseResolver's failure path (and NewExporter's
// warning on it) without touching the real /etc/resolv.conf.
var resolvConfPath = "/etc/resolv.conf"

// reverseResolver issues PTR queries directly, to expose TTL
type reverseResolver struct {
	client *dns.Client
	config *dns.ClientConfig
}

func newReverseResolver() (*reverseResolver, error) {
	return newReverseResolverFrom(resolvConfPath)
}

func newReverseResolverFrom(path string) (*reverseResolver, error) {
	config, err := dns.ClientConfigFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(config.Servers) == 0 {
		return nil, fmt.Errorf("no nameservers configured in %s", path)
	}
	return &reverseResolver{
		client: &dns.Client{Timeout: 5 * time.Second},
		config: config,
	}, nil
}

// lookup returns the joined, sorted PTR names for an address and the smallest TTL.
// On NXDOMAIN/NODATA, ttl is the RFC 2308 negative-cache TTL from the SOA record instead, or 0 when missing
func (r *reverseResolver) lookup(address string) (name string, ttl time.Duration, err error) {
	question, err := dns.ReverseAddr(address)
	if err != nil {
		return "", 0, err
	}

	msg := new(dns.Msg)
	msg.SetQuestion(question, dns.TypePTR)
	msg.RecursionDesired = true

	var lastErr error
	for _, server := range r.config.Servers {
		resp, _, exchangeErr := r.client.Exchange(msg, net.JoinHostPort(server, r.config.Port))
		if exchangeErr != nil {
			lastErr = exchangeErr
			continue
		}
		if resp.Rcode == dns.RcodeNameError {
			// Authoritative and complete - don't retry other servers.
			return "", negativeCacheTTL(resp), fmt.Errorf("%s: NXDOMAIN", question)
		}
		if resp.Rcode != dns.RcodeSuccess {
			lastErr = fmt.Errorf("%s: %s", question, dns.RcodeToString[resp.Rcode])
			continue
		}

		var names []string
		var minTTL uint32
		for _, rr := range resp.Answer {
			ptr, ok := rr.(*dns.PTR)
			if !ok {
				continue
			}
			names = append(names, ptr.Ptr)
			if minTTL == 0 || rr.Header().Ttl < minTTL {
				minTTL = rr.Header().Ttl
			}
		}
		if len(names) == 0 {
			return "", negativeCacheTTL(resp), fmt.Errorf("%s: no PTR records", question)
		}

		return formatNames(names), time.Duration(minTTL) * time.Second, nil
	}

	return "", 0, lastErr
}

// negativeCacheTTL implements RFC 2308: an authoritative negative answer's
// SOA record caps how long it may be cached, at min(SOA TTL, SOA MINIMUM).
// Returns 0 if there's no SOA.
func negativeCacheTTL(resp *dns.Msg) time.Duration {
	for _, rr := range resp.Ns {
		soa, ok := rr.(*dns.SOA)
		if !ok {
			continue
		}
		ttl := soa.Hdr.Ttl
		if soa.Minttl < ttl {
			ttl = soa.Minttl
		}
		return time.Duration(ttl) * time.Second
	}
	return 0
}

// Normalizes reverseResolver and systemReverseLookup so their outputs are directly comparable.
func formatNames(names []string) string {
	trimmed := make([]string, len(names))
	for i, name := range names {
		trimmed[i] = strings.TrimRight(name, ".")
	}
	sort.Strings(trimmed)
	return strings.Join(slices.Compact(trimmed), ",")
}

// namesAgree reports if every name in dnsNames also appears in sysNames (both
// formatNames-style comma-joined lists). PTR has no alias concept, so an
// /etc/hosts "IP fqdn alias" makes sysNames carry an extra name that DNS
// doesn't know about.
func namesAgree(sysNames, dnsNames string) bool {
	if dnsNames == "" {
		return sysNames == ""
	}
	have := make(map[string]struct{})
	for _, n := range strings.Split(sysNames, ",") {
		have[n] = struct{}{}
	}
	for _, n := range strings.Split(dnsNames, ",") {
		if _, ok := have[n]; !ok {
			return false
		}
	}
	return true
}
