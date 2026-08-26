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
	"testing"
	"time"
)

func TestDNSCacheHonorsRecordTTLByDefault(t *testing.T) {
	c := newDNSCache(0)
	calls := 0
	resolve := func() (string, time.Duration) {
		calls++
		return "host.example.", time.Hour
	}

	if got := c.lookup("192.0.2.1", resolve); got != "host.example." {
		t.Fatalf("got %q, want %q", got, "host.example.")
	}
	if got := c.lookup("192.0.2.1", resolve); got != "host.example." {
		t.Fatalf("got %q, want %q", got, "host.example.")
	}
	if calls != 1 {
		t.Fatalf("resolve called %d times, want 1 (second lookup should hit cache)", calls)
	}
}

func TestDNSCacheReResolvesAfterRecordTTLExpires(t *testing.T) {
	c := newDNSCache(0)
	calls := 0
	resolve := func() (string, time.Duration) {
		calls++
		return "host.example.", time.Millisecond
	}

	c.lookup("192.0.2.1", resolve)
	time.Sleep(20 * time.Millisecond)
	c.lookup("192.0.2.1", resolve)

	if calls != 2 {
		t.Fatalf("resolve called %d times, want 2 (entry should have expired)", calls)
	}
}

func TestDNSCacheNeverCachesFailedLookupWithNoFloor(t *testing.T) {
	c := newDNSCache(0)
	calls := 0
	resolve := func() (string, time.Duration) {
		calls++
		return "192.0.2.1", 0
	}

	c.lookup("192.0.2.1", resolve)
	time.Sleep(time.Millisecond)
	c.lookup("192.0.2.1", resolve)

	if calls != 2 {
		t.Fatalf("resolve called %d times, want 2 (a ttl of 0 should never be cached)", calls)
	}
}

func TestDNSCacheMinTTLFloorsShortRecordTTL(t *testing.T) {
	c := newDNSCache(time.Hour)
	calls := 0
	resolve := func() (string, time.Duration) {
		calls++
		return "host.example.", time.Millisecond
	}

	c.lookup("192.0.2.1", resolve)
	time.Sleep(20 * time.Millisecond)
	c.lookup("192.0.2.1", resolve)

	if calls != 1 {
		t.Fatalf("resolve called %d times, want 1 (min TTL floor should have kept the entry cached)", calls)
	}
}

func TestDNSCacheMinTTLFloorsFailedLookup(t *testing.T) {
	c := newDNSCache(time.Hour)
	calls := 0
	resolve := func() (string, time.Duration) {
		calls++
		return "192.0.2.1", 0
	}

	c.lookup("192.0.2.1", resolve)
	time.Sleep(time.Millisecond)
	c.lookup("192.0.2.1", resolve)

	if calls != 1 {
		t.Fatalf("resolve called %d times, want 1 (a failed lookup should be floored to min TTL, not left uncached)", calls)
	}
}

func TestDNSCacheMinTTLDoesNotLowerLongerRecordTTL(t *testing.T) {
	c := newDNSCache(time.Millisecond)
	calls := 0
	resolve := func() (string, time.Duration) {
		calls++
		return "host.example.", time.Hour
	}

	c.lookup("192.0.2.1", resolve)
	time.Sleep(20 * time.Millisecond)
	c.lookup("192.0.2.1", resolve)

	if calls != 1 {
		t.Fatalf("resolve called %d times, want 1 (min TTL is a floor, not a ceiling: a long record TTL must win)", calls)
	}
}

func TestDNSCacheKeysByAddress(t *testing.T) {
	c := newDNSCache(0)
	calls := map[string]int{}
	resolve := func(addr, name string) func() (string, time.Duration) {
		return func() (string, time.Duration) {
			calls[addr]++
			return name, time.Hour
		}
	}

	got1 := c.lookup("192.0.2.1", resolve("192.0.2.1", "one.example."))
	got2 := c.lookup("192.0.2.2", resolve("192.0.2.2", "two.example."))

	if got1 != "one.example." || got2 != "two.example." {
		t.Fatalf("got %q, %q, want distinct names per address", got1, got2)
	}
	if calls["192.0.2.1"] != 1 || calls["192.0.2.2"] != 1 {
		t.Fatalf("call counts %v, want exactly one resolve per address", calls)
	}
}
