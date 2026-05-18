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
	"testing"
	"time"

	"huatuo-bamai/internal/storage/types"

	"github.com/stretchr/testify/require"
)

func resetSubscribers(t *testing.T) {
	t.Helper()
	subsMu.Lock()
	subs = map[uint64]*subscriber{}
	nextID = 0
	subsMu.Unlock()
}

func TestSubscribe_ReceivesNotifiedDocument(t *testing.T) {
	resetSubscribers(t)

	ch, cancel := Subscribe()
	defer cancel()

	doc := &types.Document{TracerName: "cpu", Hostname: "h1"}
	notifySubscribers(doc)

	select {
	case received := <-ch:
		require.Equal(t, doc, received)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for document")
	}
}

func TestSubscribe_MultipleSubscribersAllReceive(t *testing.T) {
	resetSubscribers(t)

	ch1, cancel1 := Subscribe()
	defer cancel1()
	ch2, cancel2 := Subscribe()
	defer cancel2()

	doc := &types.Document{TracerName: "mem"}
	notifySubscribers(doc)

	for _, ch := range []<-chan *types.Document{ch1, ch2} {
		select {
		case got := <-ch:
			require.Equal(t, doc, got)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for document on subscriber")
		}
	}
}

func TestSubscribe_CancelRemovesSubscriber(t *testing.T) {
	resetSubscribers(t)

	_, cancel := Subscribe()
	cancel()

	subsMu.RLock()
	count := len(subs)
	subsMu.RUnlock()

	require.Zero(t, count)
}

func TestSubscribe_SlowSubscriberDoesNotBlock(t *testing.T) {
	resetSubscribers(t)

	// Fill the channel completely before notify is called.
	ch, cancel := Subscribe()
	defer cancel()

	full := make([]*types.Document, subscriberBufSize)
	for i := range full {
		full[i] = &types.Document{TracerName: "fill"}
	}
	for _, d := range full {
		ch <- d //nolint:staticcheck // direct write to pre-fill buffer
	}

	// notifySubscribers must not block when buffer is full.
	done := make(chan struct{})
	go func() {
		notifySubscribers(&types.Document{TracerName: "dropped"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("notifySubscribers blocked on a full subscriber channel")
	}
}

func TestSubscribe_NoSubscribersNotifyIsNoop(t *testing.T) {
	resetSubscribers(t)

	require.NotPanics(t, func() {
		notifySubscribers(&types.Document{TracerName: "noop"})
	})
}
