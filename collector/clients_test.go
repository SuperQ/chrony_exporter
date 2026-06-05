// Copyright 2026 Ben Kochie
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

	"github.com/facebook/time/ntp/chrony"
)

func TestClientsAggregateCounters(t *testing.T) {
	agg := newClientsAggregate()
	for _, c := range []chrony.ClientAccess{
		{NTPHits: 10, NKEHits: 1, CmdHits: 2, NTPDrops: 3, NKEDrops: 4, CmdDrops: 5, NTPInterval: 4, LastNTPHitAgo: 7},
		{NTPHits: 20, NKEHits: 2, CmdHits: 0, NTPDrops: 0, NKEDrops: 1, CmdDrops: 0, NTPInterval: 6, LastNTPHitAgo: 70},
	} {
		agg.add(c)
	}

	tests := []struct {
		name string
		got  uint64
		want uint64
	}{
		{"connected", agg.connected, 2},
		{"ntpHits", agg.ntpHits, 30},
		{"nkeHits", agg.nkeHits, 3},
		{"cmdHits", agg.cmdHits, 2},
		{"ntpDrops", agg.ntpDrops, 3},
		{"nkeDrops", agg.nkeDrops, 5},
		{"cmdDrops", agg.cmdDrops, 5},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

// Clients that have never served an NTP request (NTPHits == 0) must not enter
// the "last seen" or interval histograms, but their other counters still count.
func TestClientsAggregateSkipsZeroNTPHits(t *testing.T) {
	agg := newClientsAggregate()
	agg.add(chrony.ClientAccess{NTPHits: 0, CmdHits: 9, LastNTPHitAgo: 5, NTPInterval: 3})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 5, NTPInterval: 3})

	if agg.connected != 2 {
		t.Errorf("connected = %d, want 2 (every record counts, including zero-NTPHits)", agg.connected)
	}
	if agg.cmdHits != 9 {
		t.Errorf("cmdHits = %d, want 9", agg.cmdHits)
	}
	if agg.lastHitCount != 1 {
		t.Errorf("lastHitCount = %d, want 1 (zero-NTPHits client must be skipped)", agg.lastHitCount)
	}
	if agg.intervalCount != 1 {
		t.Errorf("intervalCount = %d, want 1", agg.intervalCount)
	}
}

// chronyd reports a sentinel interval (>= 127) and a sentinel last-hit
// (0xFFFFFFFF) for clients it has no average for; these must be excluded from
// the histograms but still counted everywhere else, matching chronyc's "-".
func TestClientsAggregateSkipsSentinels(t *testing.T) {
	agg := newClientsAggregate()
	// Valid client.
	agg.add(chrony.ClientAccess{NTPHits: 5, LastNTPHitAgo: 10, NTPInterval: 6})
	// NTP client with no computed interval yet (single hit).
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 2, NTPInterval: 127})
	// NTP client whose last hit is the "never" sentinel.
	agg.add(chrony.ClientAccess{NTPHits: 3, LastNTPHitAgo: 0xFFFFFFFF, NTPInterval: 4})

	if agg.connected != 3 {
		t.Errorf("connected = %d, want 3", agg.connected)
	}
	if agg.ntpHits != 9 {
		t.Errorf("ntpHits = %d, want 9", agg.ntpHits)
	}
	// last-hit histogram: valid client + the interval-sentinel client (its
	// last hit is valid), but not the 0xFFFFFFFF one.
	if agg.lastHitCount != 2 {
		t.Errorf("lastHitCount = %d, want 2", agg.lastHitCount)
	}
	if agg.lastHitSum != 12 {
		t.Errorf("lastHitSum = %v, want 12", agg.lastHitSum)
	}
	// interval histogram: valid client + the last-hit-sentinel client (its
	// interval is valid), but not the NTPInterval==127 one.
	if agg.intervalCount != 2 {
		t.Errorf("intervalCount = %d, want 2", agg.intervalCount)
	}
	// 2^6 + 2^4 = 64 + 16 = 80.
	if agg.intervalSum != 80 {
		t.Errorf("intervalSum = %v, want 80", agg.intervalSum)
	}
}

func TestClientsAggregateHistograms(t *testing.T) {
	agg := newClientsAggregate()
	// LastNTPHitAgo values land in buckets le 5, le 60, le 3600.
	// NTPInterval 0 -> 2^0 = 1s, NTPInterval 6 -> 2^6 = 64s.
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 3, NTPInterval: 0})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 50, NTPInterval: 6})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 3000, NTPInterval: 6})

	if agg.lastHitCount != 3 {
		t.Fatalf("lastHitCount = %d, want 3", agg.lastHitCount)
	}
	if agg.lastHitSum != 3053 {
		t.Errorf("lastHitSum = %v, want 3053", agg.lastHitSum)
	}

	// last-seen cumulative buckets.
	wantLast := map[float64]uint64{1: 0, 5: 1, 30: 1, 60: 2, 300: 2, 1800: 2, 3600: 3, 21600: 3, 86400: 3}
	for ub, want := range wantLast {
		if got := agg.lastHitBkts[ub]; got != want {
			t.Errorf("lastHitBkts[%v] = %d, want %d", ub, got, want)
		}
	}

	// interval values: 1, 64, 64 -> sum 129.
	if agg.intervalSum != 129 {
		t.Errorf("intervalSum = %v, want 129", agg.intervalSum)
	}
	wantInterval := map[float64]uint64{1: 1, 5: 1, 30: 1, 60: 1, 300: 3, 1800: 3, 3600: 3, 21600: 3, 86400: 3}
	for ub, want := range wantInterval {
		if got := agg.intervalBkts[ub]; got != want {
			t.Errorf("intervalBkts[%v] = %d, want %d", ub, got, want)
		}
	}
}

// newBuckets must seed every configured upper bound so that
// prometheus.MustNewConstHistogram emits a complete set of le series.
func TestNewBucketsSeedsAllBounds(t *testing.T) {
	buckets := newBuckets()
	if len(buckets) != len(clientsBuckets) {
		t.Fatalf("newBuckets len = %d, want %d", len(buckets), len(clientsBuckets))
	}
	for _, ub := range clientsBuckets {
		if _, ok := buckets[ub]; !ok {
			t.Errorf("newBuckets missing bound %v", ub)
		}
	}
}
