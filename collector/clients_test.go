// Copyright The Prometheus Authors
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

// Clients are split into ntp and nts by whether they completed an NTS-KE
// handshake (NKEHits > 0). Every record counts towards one of the two.
func TestClientsAggregateProtocolSplit(t *testing.T) {
	agg := newClientsAggregate()
	agg.add(chrony.ClientAccess{NTPHits: 5})             // ntp
	agg.add(chrony.ClientAccess{NTPHits: 5, NKEHits: 2}) // nts
	agg.add(chrony.ClientAccess{NTPHits: 1})             // ntp
	agg.add(chrony.ClientAccess{CmdHits: 9})             // ntp (command only, no NKE)

	if agg.ntpClients != 3 {
		t.Errorf("ntpClients = %d, want 3", agg.ntpClients)
	}
	if agg.ntsClients != 1 {
		t.Errorf("ntsClients = %d, want 1", agg.ntsClients)
	}
}

func TestClientsAggregateDropsHistogram(t *testing.T) {
	agg := newClientsAggregate()
	agg.add(chrony.ClientAccess{NTPHits: 5, NTPDrops: 0}) // counted, 0 drops
	agg.add(chrony.ClientAccess{NTPHits: 0, NTPDrops: 3}) // fully rate limited, still counted
	agg.add(chrony.ClientAccess{NTPHits: 10, NTPDrops: 60})
	agg.add(chrony.ClientAccess{CmdHits: 2}) // no NTP traffic, excluded from drops

	if agg.dropsCount != 3 {
		t.Fatalf("dropsCount = %d, want 3", agg.dropsCount)
	}
	if agg.dropsSum != 63 {
		t.Errorf("dropsSum = %v, want 63", agg.dropsSum)
	}
	want := map[float64]uint64{0: 1, 1: 1, 2: 1, 5: 2, 10: 2, 50: 2, 100: 3}
	for ub, w := range want {
		if got := agg.dropsBkts[ub]; got != w {
			t.Errorf("dropsBkts[%v] = %d, want %d", ub, got, w)
		}
	}
}

// Clients that have never served an NTP request are excluded from the time
// histograms, but are still counted elsewhere.
func TestClientsAggregateSkipsZeroNTPHits(t *testing.T) {
	agg := newClientsAggregate()
	agg.add(chrony.ClientAccess{NTPHits: 0, CmdHits: 9, LastNTPHitAgo: 5, NTPInterval: 3})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 5, NTPInterval: 3})

	if agg.lastHitCount != 1 {
		t.Errorf("lastHitCount = %d, want 1 (zero-NTPHits client must be skipped)", agg.lastHitCount)
	}
	if agg.intervalCount != 1 {
		t.Errorf("intervalCount = %d, want 1", agg.intervalCount)
	}
}

// chronyd reports a sentinel interval (>= 127) and a sentinel last-hit
// (0xFFFFFFFF) for clients it has no average for; these are excluded from the
// time histograms, matching chronyc's "-".
func TestClientsAggregateSkipsSentinels(t *testing.T) {
	agg := newClientsAggregate()
	agg.add(chrony.ClientAccess{NTPHits: 5, LastNTPHitAgo: 10, NTPInterval: 6})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 2, NTPInterval: 127})
	agg.add(chrony.ClientAccess{NTPHits: 3, LastNTPHitAgo: 0xFFFFFFFF, NTPInterval: 4})

	if agg.lastHitCount != 2 {
		t.Errorf("lastHitCount = %d, want 2", agg.lastHitCount)
	}
	if agg.lastHitSum != 12 {
		t.Errorf("lastHitSum = %v, want 12", agg.lastHitSum)
	}
	if agg.intervalCount != 2 {
		t.Errorf("intervalCount = %d, want 2", agg.intervalCount)
	}
	// 2^6 + 2^4 = 64 + 16 = 80.
	if agg.intervalSum != 80 {
		t.Errorf("intervalSum = %v, want 80", agg.intervalSum)
	}
}

func TestClientsAggregateTimeHistograms(t *testing.T) {
	agg := newClientsAggregate()
	// LastNTPHitAgo values 3, 50, 3000; NTPInterval 0, 6, 6 -> 1s, 64s, 64s.
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 3, NTPInterval: 0})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 50, NTPInterval: 6})
	agg.add(chrony.ClientAccess{NTPHits: 1, LastNTPHitAgo: 3000, NTPInterval: 6})

	if agg.lastHitSum != 3053 {
		t.Errorf("lastHitSum = %v, want 3053", agg.lastHitSum)
	}
	wantLast := map[float64]uint64{1: 0, 5: 1, 30: 1, 60: 2, 300: 2, 1800: 2, 3600: 3, 21600: 3, 86400: 3}
	for ub, w := range wantLast {
		if got := agg.lastHitBkts[ub]; got != w {
			t.Errorf("lastHitBkts[%v] = %d, want %d", ub, got, w)
		}
	}

	if agg.intervalSum != 129 {
		t.Errorf("intervalSum = %v, want 129", agg.intervalSum)
	}
	wantInterval := map[float64]uint64{1: 1, 5: 1, 30: 1, 60: 1, 300: 3, 1800: 3, 3600: 3, 21600: 3, 86400: 3}
	for ub, w := range wantInterval {
		if got := agg.intervalBkts[ub]; got != w {
			t.Errorf("intervalBkts[%v] = %d, want %d", ub, got, w)
		}
	}
}

func TestNewBucketsSeedsAllBounds(t *testing.T) {
	for _, bounds := range [][]float64{clientsTimeBuckets, clientsDropBuckets} {
		buckets := newBuckets(bounds)
		if len(buckets) != len(bounds) {
			t.Fatalf("newBuckets len = %d, want %d", len(buckets), len(bounds))
		}
		for _, ub := range bounds {
			if _, ok := buckets[ub]; !ok {
				t.Errorf("newBuckets missing bound %v", ub)
			}
		}
	}
}
