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
	"sync/atomic"
	"testing"
)

func TestPathologyWarnerFiresOnceForRepeatedKey(t *testing.T) {
	w := newPathologyWarner()
	var calls atomic.Int32

	for i := 0; i < 5; i++ {
		w.warnOnce("192.0.2.1", func() { calls.Add(1) })
	}

	if calls.Load() != 1 {
		t.Fatalf("log called %d times, want 1 (same key repeated)", calls.Load())
	}
}

func TestPathologyWarnerFiresIndependentlyPerKey(t *testing.T) {
	w := newPathologyWarner()
	var calls atomic.Int32

	w.warnOnce("192.0.2.1", func() { calls.Add(1) })
	w.warnOnce("192.0.2.2", func() { calls.Add(1) })
	w.warnOnce("192.0.2.1", func() { calls.Add(1) })

	if calls.Load() != 2 {
		t.Fatalf("log called %d times, want 2 (one per distinct key)", calls.Load())
	}
}

func TestPathologyWarnerConcurrentSameKeyFiresOnce(t *testing.T) {
	w := newPathologyWarner()
	var calls atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.warnOnce("192.0.2.1", func() { calls.Add(1) })
		}()
	}
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("log called %d times, want exactly 1 under concurrent access", calls.Load())
	}
}
