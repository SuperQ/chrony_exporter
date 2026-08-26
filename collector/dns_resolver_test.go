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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func writeResolvConf(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write temp resolv.conf: %v", err)
	}
	return path
}

// startTestDNSServerAt runs an in-process DNS server bound to addr (e.g.
// "127.0.0.1:0" for an OS-assigned port, or a specific host:port so two
// servers can share a port across different loopback addresses, as the
// multi-server fallback test needs).
func startTestDNSServerAt(t *testing.T, addr string, handler dns.HandlerFunc) (host, port string) {
	t.Helper()

	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatalf("listen on %s: %v", addr, err)
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", handler)
	srv := &dns.Server{PacketConn: pc, Handler: mux}

	started := make(chan error, 1)
	srv.NotifyStartedFunc = func() { started <- nil }
	go func() {
		if err := srv.ActivateAndServe(); err != nil {
			select {
			case started <- err:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown()
	})

	if err := <-started; err != nil {
		t.Fatalf("start test DNS server on %s: %v", addr, err)
	}

	host, port, err = net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	return host, port
}

// startTestDNSServer is startTestDNSServerAt with an OS-assigned port on
// 127.0.0.1, for tests that only need a single server.
func startTestDNSServer(t *testing.T, handler dns.HandlerFunc) (host, port string) {
	t.Helper()
	return startTestDNSServerAt(t, "127.0.0.1:0", handler)
}

// freeUDPPort returns a currently-unused UDP port by briefly binding to
// one and releasing it. There's a small window before the caller rebinds
// it, but that's an accepted, well-known tradeoff for tests that need two
// listeners sharing one port number across different loopback addresses.
func freeUDPPort(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	_, port, err := net.SplitHostPort(pc.LocalAddr().String())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	if err := pc.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return port
}

func newTestResolver(host, port string, servers ...string) *reverseResolver {
	if len(servers) == 0 {
		servers = []string{host}
	}
	return &reverseResolver{
		client: &dns.Client{Timeout: 2 * time.Second},
		config: &dns.ClientConfig{Servers: servers, Port: port},
	}
}

func ptrAnswer(t *testing.T, msg *dns.Msg, names []string, ttls []uint32) *dns.Msg {
	t.Helper()
	resp := new(dns.Msg)
	resp.SetReply(msg)
	for i, name := range names {
		resp.Answer = append(resp.Answer, &dns.PTR{
			Hdr: dns.RR_Header{Name: msg.Question[0].Name, Rrtype: dns.TypePTR, Class: dns.ClassINET, Ttl: ttls[i]},
			Ptr: dns.Fqdn(name),
		})
	}
	return resp
}

func TestReverseResolverReturnsMinTTLAcrossRecords(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := ptrAnswer(t, r, []string{"b.example", "a.example"}, []uint32{300, 60})
		_ = w.WriteMsg(resp)
	})
	resolver := newTestResolver(host, port)

	name, ttl, err := resolver.lookup("192.0.2.1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if name != "a.example,b.example" {
		t.Fatalf("name = %q, want sorted joined names", name)
	}
	if ttl != 60*time.Second {
		t.Fatalf("ttl = %v, want 60s (the smaller of the two record TTLs)", ttl)
	}
}

func TestReverseResolverNXDOMAINWithoutSOAReturnsZeroTTL(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		_ = w.WriteMsg(resp)
	})
	resolver := newTestResolver(host, port)

	_, ttl, err := resolver.lookup("192.0.2.1")
	if err == nil {
		t.Fatal("expected an error for NXDOMAIN, got nil")
	}
	if ttl != 0 {
		t.Fatalf("ttl = %v, want 0 - a response with no SOA in Authority has nothing to derive a negative cache TTL from", ttl)
	}
}

// soaAuthority builds the SOA record real authoritative servers attach to
// the Authority section of an NXDOMAIN/NODATA response, which is what RFC
// 2308 negative caching reads a TTL from.
func soaAuthority(zone string, ttl, minttl uint32) *dns.SOA {
	return &dns.SOA{
		Hdr:     dns.RR_Header{Name: dns.Fqdn(zone), Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: ttl},
		Ns:      dns.Fqdn("ns1." + zone),
		Mbox:    dns.Fqdn("hostmaster." + zone),
		Serial:  1,
		Refresh: 3600,
		Retry:   600,
		Expire:  86400,
		Minttl:  minttl,
	}
}

func TestReverseResolverNXDOMAINWithSOAReturnsNegativeCacheTTL(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeNameError)
		resp.Ns = []dns.RR{soaAuthority("in-addr.arpa.", 3600, 300)}
		_ = w.WriteMsg(resp)
	})
	resolver := newTestResolver(host, port)

	_, ttl, err := resolver.lookup("192.0.2.1")
	if err == nil {
		t.Fatal("expected an error for NXDOMAIN, got nil")
	}
	if ttl != 300*time.Second {
		t.Fatalf("ttl = %v, want 300s - the smaller of the SOA record's own TTL (3600s) and its MINIMUM field (300s), per RFC 2308", ttl)
	}
}

func TestReverseResolverEmptyAnswerReturnsError(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		_ = w.WriteMsg(resp)
	})
	resolver := newTestResolver(host, port)

	_, ttl, err := resolver.lookup("192.0.2.1")
	if err == nil {
		t.Fatal("expected an error for a NOERROR response with no PTR records, got nil")
	}
	if ttl != 0 {
		t.Fatalf("ttl = %v, want 0 with no SOA in Authority", ttl)
	}
}

func TestReverseResolverEmptyAnswerWithSOAReturnsNegativeCacheTTL(t *testing.T) {
	host, port := startTestDNSServer(t, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(r)
		resp.Ns = []dns.RR{soaAuthority("in-addr.arpa.", 120, 900)}
		_ = w.WriteMsg(resp)
	})
	resolver := newTestResolver(host, port)

	_, ttl, err := resolver.lookup("192.0.2.1")
	if err == nil {
		t.Fatal("expected an error for a NOERROR response with no PTR records, got nil")
	}
	if ttl != 120*time.Second {
		t.Fatalf("ttl = %v, want 120s - the smaller of the SOA record's own TTL (120s) and its MINIMUM field (900s)", ttl)
	}
}

func TestReverseResolverFallsBackToNextServer(t *testing.T) {
	// dns.ClientConfig shares one port across all servers, so both test
	// servers use it on 127.0.0.1 and ::1 (not 127.0.0.2 - sandboxes may
	// refuse to bind extra IPv4 aliases). First answers SERVFAIL, resolver
	// should move on to the second.
	port := freeUDPPort(t)
	badHost, _ := startTestDNSServerAt(t, "127.0.0.1:"+port, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetRcode(r, dns.RcodeServerFailure)
		_ = w.WriteMsg(resp)
	})
	goodHost, _ := startTestDNSServerAt(t, "[::1]:"+port, func(w dns.ResponseWriter, r *dns.Msg) {
		resp := ptrAnswer(t, r, []string{"good.example"}, []uint32{120})
		_ = w.WriteMsg(resp)
	})

	resolver := newTestResolver(badHost, port, badHost, goodHost)

	name, ttl, err := resolver.lookup("192.0.2.1")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if name != "good.example" {
		t.Fatalf("name = %q, want the second server's answer", name)
	}
	if ttl != 120*time.Second {
		t.Fatalf("ttl = %v, want 120s", ttl)
	}
}

func TestNamesAgree(t *testing.T) {
	cases := []struct {
		name    string
		sys     string
		dns     string
		agree   bool
		comment string
	}{
		{"exact match", "a.example", "a.example", true, "identical single names"},
		{"both empty", "", "", true, "no answer from either resolver isn't a disagreement"},
		{
			"dns name subset of a /etc/hosts alias list", "data.pve1.internal.lan,data1", "data.pve1.internal.lan", true,
			"the standard /etc/hosts \"IP fqdn alias\" convention: net.LookupAddr reports the alias too, DNS PTR has no concept of one",
		},
		{
			"dns name matches one of several /etc/hosts aliases", "fqdn.example,alias1,alias2,alias3", "alias2", true,
			"a match against any alias in a longer /etc/hosts line, not just the first or second one",
		},
		{"unrelated names", "from-system.example", "from-dns.example", false, "no overlap at all"},
		{"dns has a name sys doesn't", "a.example", "a.example,b.example", false, "dns claims more than the system resolver backs up"},
		{"dns empty, sys not", "a.example", "", false, "no real DNS answer to agree with, not an empty-vs-empty match"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := namesAgree(c.sys, c.dns); got != c.agree {
				t.Fatalf("namesAgree(%q, %q) = %v, want %v (%s)", c.sys, c.dns, got, c.agree, c.comment)
			}
		})
	}
}

func TestNewReverseResolverFailsWhenResolvConfMissing(t *testing.T) {
	_, err := newReverseResolverFrom(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a missing resolv.conf, got nil")
	}
}

func TestNewReverseResolverFailsWhenNoNameservers(t *testing.T) {
	path := writeResolvConf(t, "# no nameserver lines here\n")
	_, err := newReverseResolverFrom(path)
	if err == nil {
		t.Fatal("expected an error for a resolv.conf with no nameservers, got nil")
	}
}

func TestNewReverseResolverSucceedsWithAValidResolvConf(t *testing.T) {
	path := writeResolvConf(t, "nameserver 192.0.2.1\n")
	r, err := newReverseResolverFrom(path)
	if err != nil {
		t.Fatalf("newReverseResolverFrom: %v", err)
	}
	if r == nil {
		t.Fatal("resolver is nil despite no error")
	}
}

func TestNewExporterWarnsWhenResolvConfIsUnreadable(t *testing.T) {
	orig := resolvConfPath
	resolvConfPath = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(func() { resolvConfPath = orig })

	logger, buf := newCapturingLogger()
	e := NewExporter(ChronyCollectorConfig{DNSLookups: true}, logger)

	if e.resolver != nil {
		t.Fatal("resolver should be nil when resolv.conf can't be read")
	}
	if !strings.Contains(buf.String(), "direct DNS queries for record TTLs are disabled") {
		t.Fatalf("expected a startup warning about the unreadable resolv.conf, got:\n%s", buf.String())
	}
}

func TestNewExporterNoWarningWithAValidResolvConf(t *testing.T) {
	orig := resolvConfPath
	resolvConfPath = writeResolvConf(t, "nameserver 192.0.2.1\n")
	t.Cleanup(func() { resolvConfPath = orig })

	logger, buf := newCapturingLogger()
	e := NewExporter(ChronyCollectorConfig{DNSLookups: true}, logger)

	if e.resolver == nil {
		t.Fatal("resolver should be constructed with a valid resolv.conf")
	}
	if strings.Contains(buf.String(), "direct DNS queries for record TTLs are disabled") {
		t.Fatalf("unexpected startup warning with a valid resolv.conf:\n%s", buf.String())
	}
}
