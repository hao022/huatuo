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

package opensearch

import (
	"fmt"
	"net/http"
	"testing"

	"huatuo-bamai/internal/storage/elasticsearch"
	"huatuo-bamai/internal/storage/types"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------- Local helpers using exported mock tools ----------

// newMockClientForWrite creates a StorageClient whose transport returns a
// successful (status code + body) response for any request.
func newMockClientForWrite(t *testing.T, statusCode int, responseBody string) *StorageClient {
	t.Helper()

	rt := new(elasticsearch.MockRoundTripper)
	rt.On("RoundTrip", mock.Anything).
		Return(func(req *http.Request) *http.Response {
			return elasticsearch.NewMockHTTPResponse(
				statusCode,
				responseBody,
				map[string]string{
					"Content-Type": "application/json",
				},
			)
		}, nil)

	cfg := opensearch.Config{
		Addresses: []string{"http://mock"},
		Transport: rt,
	}
	client, err := opensearch.NewClient(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		rt.AssertExpectations(t)
	})

	return &StorageClient{
		client: client,
		index:  elasticsearch.DefaultIndex,
	}
}

// newMockClientForWriteWithError creates a StorageClient whose transport
// always returns a network error.
func newMockClientForWriteWithError(t *testing.T) *StorageClient {
	t.Helper()

	rt := elasticsearch.NewErrorTransport(t)

	cfg := opensearch.Config{
		Addresses: []string{"http://mock"},
		Transport: rt,
	}
	client, err := opensearch.NewClient(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		rt.AssertExpectations(t)
	})

	return &StorageClient{
		client: client,
		index:  elasticsearch.DefaultIndex,
	}
}

// ---------- Tests ----------

func TestNewStorageClient_Success(t *testing.T) {
	origTransport := elasticsearch.DefaultTransport
	rt := new(elasticsearch.MockRoundTripper)
	elasticsearch.DefaultTransport = rt
	defer func() { elasticsearch.DefaultTransport = origTransport }()

	rt.On("RoundTrip", mock.Anything).
		Return(
			//nolint:bodyclose
			elasticsearch.NewMockHTTPResponse(
				http.StatusOK,
				`{
					"name":"mock-node",
					"cluster_name":"mock-cluster",
					"cluster_uuid":"abc123",
					"version":{"number":"1.3.0","distribution":"opensearch"}
				}`,
				map[string]string{"Content-Type": "application/json"},
			),
			nil,
		)

	client, err := NewStorageClient("http://mock-os:9200", "", "", "")
	require.NoError(t, err)
	require.Equal(t, elasticsearch.DefaultIndex, client.index)

	rt.AssertExpectations(t)
}

func TestNewStorageClient_InvalidURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"malformed URL", "http://[invalid]", true},
		{"unsupported scheme", "ftp://localhost:9200", true},
		{"empty URL", "", true},
		{"no scheme", "localhost:9200", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStorageClient(tt.url, "", "", "")
			if tt.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil but got err: %v", err)
			}
		})
	}
}

func TestNewStorageClient_FailOnInfo(t *testing.T) {
	origTransport := elasticsearch.DefaultTransport
	rt := new(elasticsearch.MockRoundTripper)
	elasticsearch.DefaultTransport = rt
	defer func() { elasticsearch.DefaultTransport = origTransport }()

	rt.On("RoundTrip", mock.Anything).
		Return(
			//nolint:bodyclose
			elasticsearch.NewMockHTTPResponse(
				http.StatusInternalServerError,
				"",
				map[string]string{"Content-Type": "application/json"},
			),
			nil,
		)

	_, err := NewStorageClient("http://mock-os:9200", "", "", "")
	require.Error(t, err)

	rt.AssertExpectations(t)
}

func TestWrite_Success(t *testing.T) {
	client := newMockClientForWrite(t, 201, `{"result":"created"}`)

	doc := &types.Document{
		TracerID:   "test-id",
		TracerData: "test-data",
	}

	err := client.Write(doc)
	require.NoError(t, err)
}

func TestWrite_FailStatus(t *testing.T) {
	client := newMockClientForWrite(t, 500, `{"error":"fail"}`)

	doc := &types.Document{
		TracerID:   "test-id",
		TracerData: "test-data",
	}

	err := client.Write(doc)
	require.Error(t, err)
}

func TestWrite_EmptyDocument(t *testing.T) {
	tests := []struct {
		name string
		doc  *types.Document
	}{
		{"nil document", nil},
		{"empty document", &types.Document{}},
		{"only tracer ID", &types.Document{TracerID: "empty-id"}},
		{"only tracer data", &types.Document{TracerData: "empty-data"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newMockClientForWrite(t, 201, `{"result":"created","_id":"test-id"}`)
			err := client.Write(tt.doc)
			require.NoError(t, err)
		})
	}
}

func TestWrite_Non200Success(t *testing.T) {
	client := newMockClientForWrite(t, 201, `{"result":"created"}`)

	err := client.Write(&types.Document{TracerID: "id"})
	require.NoError(t, err)
}

func TestWrite_JSONMarshalError(t *testing.T) {
	client := &StorageClient{
		client: nil, // client won't be used because marshal fails first
		index:  "test-index",
	}

	doc := &types.Document{
		TracerData: badMarshalType{},
	}

	err := client.Write(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "forced marshal error")
}

func TestWrite_IndexRequestDoError(t *testing.T) {
	client := newMockClientForWriteWithError(t)

	doc := &types.Document{
		TracerID: "test",
	}

	err := client.Write(doc)
	require.Error(t, err)
	require.Contains(t, err.Error(), "simulated network error")
}

func TestWrite_ReturnsErrorStatus(t *testing.T) {
	client := newMockClientForWrite(t, 300, `{"result":"created"}`)

	err := client.Write(&types.Document{TracerID: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "index document failed with status: 300")
}

func TestWrite_InvalidJSONResponse(t *testing.T) {
	client := newMockClientForWrite(t, 200, `this is not json`)

	err := client.Write(&types.Document{TracerID: "test"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse response body")
}

// badMarshalType helps simulate JSON marshaling errors.
type badMarshalType struct{}

func (b badMarshalType) MarshalJSON() ([]byte, error) {
	return nil, fmt.Errorf("forced marshal error")
}
