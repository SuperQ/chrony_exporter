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
	"bytes"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func newTestExporter(t *testing.T, dnsHost, dnsPort string, systemLookup func(string) (string, error)) Exporter {
	t.Helper()
	e := Exporter{
		dnsLookups:      true,
		systemLookup:    systemLookup,
		dnsCache:        newDNSCache(0),
		pathologyWarner: newPathologyWarner(),
		logger:          slog.New(slog.DiscardHandler),
	}
	if dnsHost != "" {
		e.resolver = newTestResolver(dnsHost, dnsPort)
	}
	return e
}

func startTestPTRServer(t *testing.T, names []string, ttl uint32) (host, port string) {
	t.Helper()
	return startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		ttls := make([]uint32, len(names))
		for i := range ttls {
			ttls[i] = ttl
		}
		_ = w.WriteMsg(ptrAnswer(t, r, names, ttls))
	})
}

func TestDNSLookupSystemAnswerWinsOnDisagreement(t *testing.T) {
	host, port := startTestPTRServer(t, []string{"from-dns.example"}, 60)
	e := newTestExporter(t, host, port, func(string) (string, error) {
		return "from-system.example", nil
	})

	if got := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1")); got != "from-system.example" {
		t.Fatalf("got %q, want the system resolver's answer to win over a disagreeing direct DNS answer", got)
	}
}

func TestDNSLookupCachesForTheDNSRecordTTLWhenSystemAnswerHasAnExtraAlias(t *testing.T) {
	// /etc/hosts "IP fqdn alias" convention: net.LookupAddr reports both
	// names, PTR only the FQDN. Not a real disagreement.
	var dnsCalls atomic.Int32
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		dnsCalls.Add(1)
		_ = w.WriteMsg(ptrAnswer(t, r, []string{"data.pve1.internal.lan"}, []uint32{600}))
	})
	var systemCalls atomic.Int32
	e := newTestExporter(t, host, port, func(string) (string, error) {
		systemCalls.Add(1)
		return "data.pve1.internal.lan,data1", nil
	})

	got1 := e.dnsLookup(e.logger, net.ParseIP("172.16.12.124"))
	got2 := e.dnsLookup(e.logger, net.ParseIP("172.16.12.124"))

	if got1 != "data.pve1.internal.lan,data1" || got2 != "data.pve1.internal.lan,data1" {
		t.Fatalf("got %q, %q, want the system resolver's fuller answer both times", got1, got2)
	}
	if dnsCalls.Load() != 1 || systemCalls.Load() != 1 {
		t.Fatalf("dns calls = %d, system calls = %d, want 1 each (second lookup should hit the cache using the DNS record's TTL)", dnsCalls.Load(), systemCalls.Load())
	}
}

func TestDNSLookupDisagreementDoesNotBorrowTheDNSRecordTTL(t *testing.T) {
	// A disagreement must not reuse the losing DNS answer's TTL - same as
	// no TTL at all (floor 0 here, so every lookup re-resolves).
	var dnsCalls atomic.Int32
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		dnsCalls.Add(1)
		_ = w.WriteMsg(ptrAnswer(t, r, []string{"from-dns.example"}, []uint32{3600}))
	})
	var systemCalls atomic.Int32
	e := newTestExporter(t, host, port, func(string) (string, error) {
		systemCalls.Add(1)
		return "from-system.example", nil
	})

	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if dnsCalls.Load() != 2 || systemCalls.Load() != 2 {
		t.Fatalf("dns calls = %d, system calls = %d, want 2 each - a disagreement shouldn't cache using the losing answer's TTL", dnsCalls.Load(), systemCalls.Load())
	}
}

func TestDNSLookupCachesForTheDirectDNSRecordTTL(t *testing.T) {
	// systemLookup agrees here - only agreement trusts the DNS TTL.
	var calls atomic.Int32
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		calls.Add(1)
		_ = w.WriteMsg(ptrAnswer(t, r, []string{"agreed.example"}, []uint32{3600}))
	})
	var systemCalls atomic.Int32
	e := newTestExporter(t, host, port, func(string) (string, error) {
		systemCalls.Add(1)
		return "agreed.example", nil
	})

	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if calls.Load() != 1 || systemCalls.Load() != 1 {
		t.Fatalf("direct DNS calls = %d, system calls = %d, want 1 each (second lookup should hit cache using the direct DNS record's TTL)", calls.Load(), systemCalls.Load())
	}
}

func TestDNSLookupNeverSurfacesDNSNameWhenSystemLookupFails(t *testing.T) {
	// e.resolver is TTL-only, never a fallback name source.
	host, port := startTestPTRServer(t, []string{"from-dns.example"}, 60)
	e := newTestExporter(t, host, port, func(string) (string, error) {
		return "", errors.New("no such host")
	})

	if got := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1")); got != "192.0.2.1" {
		t.Fatalf("got %q, want the raw address - the direct DNS answer must never be surfaced when the system lookup fails", got)
	}
}

func TestDNSLookupPacesRetriesByDNSRecordTTLEvenWhenNotSurfaced(t *testing.T) {
	// TTL still paces retries on system-lookup failure, without making
	// dnsName eligible to be returned.
	var dnsCalls atomic.Int32
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		dnsCalls.Add(1)
		_ = w.WriteMsg(ptrAnswer(t, r, []string{"from-dns.example"}, []uint32{3600}))
	})
	var systemCalls atomic.Int32
	e := newTestExporter(t, host, port, func(string) (string, error) {
		systemCalls.Add(1)
		return "", errors.New("no such host")
	})

	got1 := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	got2 := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if got1 != "192.0.2.1" || got2 != "192.0.2.1" {
		t.Fatalf("got %q, %q, want the raw address both times", got1, got2)
	}
	if dnsCalls.Load() != 1 || systemCalls.Load() != 1 {
		t.Fatalf("dns calls = %d, system calls = %d, want 1 each (second lookup should hit the cache using the DNS record's TTL)", dnsCalls.Load(), systemCalls.Load())
	}
}

func TestDNSLookupFallsBackToRawAddressWhenBothFail(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(resp)
	})
	e := newTestExporter(t, host, port, func(string) (string, error) {
		return "", errors.New("no such host")
	})

	if got := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1")); got != "192.0.2.1" {
		t.Fatalf("got %q, want the raw address when neither resolver has an answer", got)
	}
}

func TestDNSLookupCachesNXDOMAINForItsNegativeCacheTTL(t *testing.T) {
	// No PTR record shouldn't mean re-querying every scrape - honor the
	// NXDOMAIN's RFC 2308 negative-cache TTL like a positive record's.
	var dnsCalls atomic.Int32
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		dnsCalls.Add(1)
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		resp.Ns = []dns.RR{soaAuthority("in-addr.arpa.", 3600, 300)}
		_ = w.WriteMsg(resp)
	})
	var systemCalls atomic.Int32
	e := newTestExporter(t, host, port, func(string) (string, error) {
		systemCalls.Add(1)
		return "", errors.New("no such host")
	})

	got1 := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	got2 := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if got1 != "192.0.2.1" || got2 != "192.0.2.1" {
		t.Fatalf("got %q, %q, want the raw address both times", got1, got2)
	}
	if dnsCalls.Load() != 1 || systemCalls.Load() != 1 {
		t.Fatalf("dns calls = %d, system calls = %d, want 1 each (second lookup should hit the negative cache entry)", dnsCalls.Load(), systemCalls.Load())
	}
}

func TestDNSLookupNoResolverStillUsesSystemAnswer(t *testing.T) {
	// e.resolver nil at runtime (see TestNewExporterWarnsWhenResolvConfIsUnreadable
	// for how that happens and warns): system lookup still works, ttl falls
	// back to minTTL (0 by default).
	e := newTestExporter(t, "", "", func(string) (string, error) {
		return "from-system.example", nil
	})

	if got := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1")); got != "from-system.example" {
		t.Fatalf("got %q, want the system resolver's answer even with no direct DNS resolver configured", got)
	}
}

func TestDNSLookupNoResolverStillHonorsTheConfiguredFloor(t *testing.T) {
	// With e.resolver nil, ttl is always 0 - so without a floor, every
	// lookup would re-hit systemLookup. Confirms the floor still provides
	// real caching in this fully-degraded case.
	var systemCalls atomic.Int32
	e := Exporter{
		dnsLookups: true,
		systemLookup: func(string) (string, error) {
			systemCalls.Add(1)
			return "from-system.example", nil
		},
		dnsCache:        newDNSCache(time.Hour),
		pathologyWarner: newPathologyWarner(),
		logger:          slog.New(slog.DiscardHandler),
	}

	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if systemCalls.Load() != 1 {
		t.Fatalf("system calls = %d, want 1 (second lookup should hit the cache via the configured floor)", systemCalls.Load())
	}
}

func TestDNSLookupDisabledCallsNeitherResolver(t *testing.T) {
	systemCalled := false
	e := Exporter{
		dnsLookups: false,
		systemLookup: func(string) (string, error) {
			systemCalled = true
			return "should-not-be-used.example", nil
		},
		logger: slog.New(slog.DiscardHandler),
	}

	if got := e.dnsLookup(e.logger, net.ParseIP("192.0.2.1")); got != "192.0.2.1" {
		t.Fatalf("got %q, want the raw address (--no-collector.dns-lookups)", got)
	}
	if systemCalled {
		t.Fatal("system resolver was called even though dns-lookups is disabled")
	}
}

func newCapturingLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestDNSLookupWarnsOnceOnPersistentDisagreement(t *testing.T) {
	host, port := startTestPTRServer(t, []string{"from-dns.example"}, 60)
	logger, buf := newCapturingLogger()
	e := Exporter{
		dnsLookups:      true,
		resolver:        newTestResolver(host, port),
		dnsCache:        newDNSCache(0),
		pathologyWarner: newPathologyWarner(),
		systemLookup: func(string) (string, error) {
			return "from-system.example", nil
		},
		logger: logger,
	}

	for i := 0; i < 3; i++ {
		e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	}

	count := strings.Count(buf.String(), "pathological reverse DNS lookup")
	if count != 1 {
		t.Fatalf("warning logged %d times across 3 lookups of the same disagreeing address, want exactly 1\nlog:\n%s", count, buf.String())
	}
}

func TestDNSLookupWarnsOnceWhenBothResolversHaveNoUsableTTL(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeServerFailure) // no SOA, no negative-cache TTL
		_ = w.WriteMsg(resp)
	})
	logger, buf := newCapturingLogger()
	e := Exporter{
		dnsLookups:      true,
		resolver:        newTestResolver(host, port),
		dnsCache:        newDNSCache(0),
		pathologyWarner: newPathologyWarner(),
		systemLookup: func(string) (string, error) {
			return "", errors.New("no such host")
		},
		logger: logger,
	}

	for i := 0; i < 3; i++ {
		e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	}

	count := strings.Count(buf.String(), "pathological reverse DNS lookup")
	if count != 1 {
		t.Fatalf("warning logged %d times across 3 lookups with no usable TTL, want exactly 1\nlog:\n%s", count, buf.String())
	}
}

func TestDNSLookupDoesNotWarnWhenNegativeCachingSucceeds(t *testing.T) {
	// A clean NXDOMAIN+SOA is negative caching working, not pathological.
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		resp.Ns = []dns.RR{soaAuthority("in-addr.arpa.", 3600, 300)}
		_ = w.WriteMsg(resp)
	})
	logger, buf := newCapturingLogger()
	e := Exporter{
		dnsLookups:      true,
		resolver:        newTestResolver(host, port),
		dnsCache:        newDNSCache(0),
		pathologyWarner: newPathologyWarner(),
		systemLookup: func(string) (string, error) {
			return "", errors.New("no such host")
		},
		logger: logger,
	}

	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if strings.Contains(buf.String(), "pathological") {
		t.Fatalf("no warning expected when negative caching found a real SOA-backed TTL\nlog:\n%s", buf.String())
	}
}

func TestDNSLookupDoesNotWarnOnAgreement(t *testing.T) {
	// Clean agreement is the normal case - never pathological.
	host, port := startTestPTRServer(t, []string{"agreed.example"}, 600)
	logger, buf := newCapturingLogger()
	e := Exporter{
		dnsLookups:      true,
		resolver:        newTestResolver(host, port),
		dnsCache:        newDNSCache(0),
		pathologyWarner: newPathologyWarner(),
		systemLookup: func(string) (string, error) {
			return "agreed.example", nil
		},
		logger: logger,
	}

	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))

	if strings.Contains(buf.String(), "pathological") {
		t.Fatalf("no warning expected on clean agreement\nlog:\n%s", buf.String())
	}
}

func TestDNSLookupWarningsAreIndependentPerAddress(t *testing.T) {
	host, port := startTestPTRServer(t, []string{"from-dns.example"}, 60)
	logger, buf := newCapturingLogger()
	e := Exporter{
		dnsLookups:      true,
		resolver:        newTestResolver(host, port),
		dnsCache:        newDNSCache(0),
		pathologyWarner: newPathologyWarner(),
		systemLookup: func(string) (string, error) {
			return "from-system.example", nil
		},
		logger: logger,
	}

	e.dnsLookup(e.logger, net.ParseIP("192.0.2.1"))
	e.dnsLookup(e.logger, net.ParseIP("192.0.2.2"))

	count := strings.Count(buf.String(), "pathological reverse DNS lookup")
	if count != 2 {
		t.Fatalf("warning logged %d times for 2 distinct disagreeing addresses, want 2 (once each)\nlog:\n%s", count, buf.String())
	}
}
