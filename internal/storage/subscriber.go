// Copyright 2026 The HuaTuo Authors
//
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

package storage

import (
	"sync"

	"huatuo-bamai/internal/storage/types"
)

// subscriberBufSize is the per-subscriber channel buffer.
// Slow consumers that fall behind will have events dropped rather than
// blocking the Save hot path.
const subscriberBufSize = 256

type subscriber struct {
	ch chan *types.Document
}

var (
	subsMu sync.RWMutex
	subs   = map[uint64]*subscriber{}
	nextID uint64
)

// Subscribe registers a new event subscriber and returns a read-only channel
// and a cancel function. Call cancel when the subscriber no longer needs events.
// Events that cannot be delivered (full buffer) are silently dropped.
func Subscribe() (<-chan *types.Document, func()) {
	subsMu.Lock()
	id := nextID
	nextID++
	sub := &subscriber{ch: make(chan *types.Document, subscriberBufSize)}
	subs[id] = sub
	subsMu.Unlock()

	cancel := func() {
		subsMu.Lock()
		delete(subs, id)
		subsMu.Unlock()
	}
	return sub.ch, cancel
}

// notifySubscribers fans out doc to every registered subscriber.
// It is called by Save after writing to the storage backends.
func notifySubscribers(doc *types.Document) {
	subsMu.RLock()
	defer subsMu.RUnlock()

	for _, sub := range subs {
		select {
		case sub.ch <- doc:
		default:
		}
	}
}
