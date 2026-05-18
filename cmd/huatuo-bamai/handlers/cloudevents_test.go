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

package handlers

import (
	"strings"
	"testing"
	"time"

	storagetypes "huatuo-bamai/internal/storage/types"
	pkgtypes "huatuo-bamai/pkg/types"

	"github.com/stretchr/testify/require"
)

func newTestDocument() *storagetypes.Document {
	return &storagetypes.Document{
		Hostname:      "node-1",
		Region:        "cn",
		UploadedTime:  time.Unix(1_700_000_000, 0).UTC(),
		TracerName:    "cpu",
		TracerRunType: "auto",
	}
}

func TestDocumentToWatchEvent_CloudEventsFields(t *testing.T) {
	doc := newTestDocument()
	ev := DocumentToWatchEvent(doc)

	require.Equal(t, "1.0", ev.SpecVersion)
	require.Equal(t, "tech.huatuo.kernel.event", ev.Type)
	require.Equal(t, "application/json", ev.DataContentType)
	require.NotEmpty(t, ev.ID)
	require.True(t, strings.HasPrefix(ev.Source, "/huatuo/node-1/cpu"))
	require.Equal(t, doc.UploadedTime.UTC().Format(time.RFC3339Nano), ev.Time)
}

func TestDocumentToWatchEvent_UniqueIDs(t *testing.T) {
	doc := newTestDocument()
	require.NotEqual(t, DocumentToWatchEvent(doc).ID, DocumentToWatchEvent(doc).ID)
}

func TestDocumentToWatchEvent_Data(t *testing.T) {
	doc := newTestDocument()
	ev := DocumentToWatchEvent(doc)

	want := pkgtypes.WatchEventData{
		Hostname:          doc.Hostname,
		Region:            doc.Region,
		ObservedTimestamp: doc.TracerTime,
		TracerName:        doc.TracerName,
		TracerRunType:     doc.TracerRunType,
	}
	require.Equal(t, want, ev.Data)
}
