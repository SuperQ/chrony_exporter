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
	"sync"
	"time"
)

// dnsCache memoizes reverse DNS lookups for their own record TTL by
// default (minTTL 0). minTTL, when set, floors short/absent TTLs, trading
// protocol correctness for a hard cap on repeat-query rate.
type dnsCache struct {
	minTTL time.Duration

	mu      sync.Mutex
	entries map[string]dnsCacheEntry
}

type dnsCacheEntry struct {
	name    string
	expires time.Time
}

func newDNSCache(minTTL time.Duration) *dnsCache {
	return &dnsCache{
		minTTL:  minTTL,
		entries: make(map[string]dnsCacheEntry),
	}
}

// lookup returns the cached name for an address if present and unexpired,
// otherwise calls resolve, caches the result, and returns it. resolve
// reports TTL or the cache's minTTL floor when shorter.
func (c *dnsCache) lookup(address string, resolve func() (name string, ttl time.Duration)) string {
	now := time.Now()

	c.mu.Lock()
	entry, ok := c.entries[address]
	c.mu.Unlock()

	if ok && now.Before(entry.expires) {
		return entry.name
	}

	name, ttl := resolve()
	if ttl < c.minTTL {
		ttl = c.minTTL
	}

	c.mu.Lock()
	c.entries[address] = dnsCacheEntry{name: name, expires: now.Add(ttl)}
	c.mu.Unlock()

	return name
}
