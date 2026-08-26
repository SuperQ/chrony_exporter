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

import "sync"

// pathologyWarner logs each distinct (address, kind) pathology once per
// process, so a pathological address produces one clear warning instead of
// spamming the log every scrape.
type pathologyWarner struct {
	mu     sync.Mutex
	warned map[string]struct{}
}

func newPathologyWarner() *pathologyWarner {
	return &pathologyWarner{warned: make(map[string]struct{})}
}

// warnOnce calls log the first time this exact key is seen
func (w *pathologyWarner) warnOnce(key string, log func()) {
	w.mu.Lock()
	_, seen := w.warned[key]
	if !seen {
		w.warned[key] = struct{}{}
	}
	w.mu.Unlock()

	if !seen {
		log()
	}
}
