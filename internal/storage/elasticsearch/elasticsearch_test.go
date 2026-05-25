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

package elasticsearch

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"huatuo-bamai/internal/storage/driver"
)

// TestBuildSearchRequest covers query DSL construction: verifies that equality, not-equal, range, IN, sort, pagination, and invalid pagination are all translated to the correct ES request body.
func TestBuildSearchRequest(t *testing.T) {
	baseTime := time.Date(2026, 4, 9, 8, 0, 0, 123000000, time.UTC)

	cases := []struct {
		name     string
		query    driver.Query
		validate func(*testing.T, map[string]any, error)
	}{
		{
			name: "filters-sorts-and-pagination",
			query: driver.Query{
				Filters: []driver.Filter{
					{Field: "status", Op: driver.OpEq, Value: "running"},
					{Field: "priority", Op: driver.OpGt, Value: 5},
					{Field: "created_at", Op: driver.OpLte, Value: baseTime},
					{Field: "user_id", Op: driver.OpIn, Value: []string{"user-alpha", "user-beta"}},
				},
				Sorts: []driver.Sort{
					{Field: "priority", Desc: true},
				},
				Limit:  2,
				Offset: 1,
			},
			validate: func(t *testing.T, got map[string]any, err error) {
				if err != nil {
					t.Errorf("buildSearchBody() returned error: %v", err)
					return
				}

				if intFromAny(got["size"]) != 2 {
					t.Errorf("size = %v, want 2", got["size"])
				}
				if intFromAny(got["from"]) != 1 {
					t.Errorf("from = %v, want 1", got["from"])
				}

				queryMap, _ := got["query"].(map[string]any)
				boolQuery, _ := queryMap["bool"].(map[string]any)
				filterClauses := toAnySlice(boolQuery["filter"])
				if len(filterClauses) != 4 {
					t.Errorf("filter clause count = %d, want 4", len(filterClauses))
				}

				sortClauses := toAnySlice(got["sort"])
				if len(sortClauses) != 1 {
					t.Errorf("sort clause count = %d, want 1", len(sortClauses))
				}
			},
		},
		{
			name: "not-equal-uses-must-not",
			query: driver.Query{
				Filters: []driver.Filter{
					{Field: "status", Op: driver.OpNe, Value: "completed"},
				},
			},
			validate: func(t *testing.T, got map[string]any, err error) {
				if err != nil {
					t.Errorf("buildSearchBody() returned error: %v", err)
					return
				}

				queryMap, _ := got["query"].(map[string]any)
				boolQuery, _ := queryMap["bool"].(map[string]any)
				mustNotClauses := toAnySlice(boolQuery["must_not"])
				if len(mustNotClauses) != 1 {
					t.Errorf("must_not clause count = %d, want 1", len(mustNotClauses))
				}
			},
		},
		{
			name: "invalid-pagination",
			query: driver.Query{
				Limit: -1,
			},
			validate: func(t *testing.T, got map[string]any, err error) {
				if err == nil {
					t.Errorf("buildSearchBody() error = nil, want error")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rawBody, err := buildSearchRequest(tc.query)
			body := decodeJSONMap(t, rawBody)
			tc.validate(t, body, err)
		})
	}
}

// TestElasticsearchBackendCRUD covers ES backend initialization and basic CRUD: verifies index creation, document save, lookup by ID, delete of existing and missing documents match the unified storage layer contract.
func TestElasticsearchBackendCRUD(t *testing.T) {
	server := newMockElasticsearchServer()
	defer server.Close()

	backend := newBackendForTest(t, server)
	if backend == nil {
		return
	}

	err := backend.Init(t.Context(), "jobs", []driver.Index{
		{Field: "user_id"},
		{Field: "status"},
		{Field: "priority"},
		{Field: "created_at"},
	})
	if err != nil {
		t.Errorf("Init() returned error: %v", err)
		return
	}

	recordTime := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	record := driver.Record{
		ID:   "job-es-alpha",
		Data: []byte(`{"id":"job-es-alpha","status":"running"}`),
		Fields: map[string]any{
			"user_id":    "user-alpha",
			"status":     "running",
			"priority":   9,
			"created_at": recordTime,
		},
	}

	if err := backend.Save(t.Context(), record); err != nil {
		t.Errorf("Save() returned error: %v", err)
	}

	gotRecord, err := backend.Get(t.Context(), "job-es-alpha")
	if err != nil {
		t.Errorf("Get() returned error: %v", err)
	}
	if !bytes.Equal(gotRecord.Data, record.Data) {
		t.Errorf("Get() data = %s, want %s", string(gotRecord.Data), string(record.Data))
	}

	if err := backend.Delete(t.Context(), "job-es-alpha"); err != nil {
		t.Errorf("Delete() returned error: %v", err)
	}

	_, err = backend.Get(t.Context(), "job-es-alpha")
	if !errors.Is(err, driver.ErrNotFound) {
		t.Errorf("Get() after delete = %v, want ErrNotFound", err)
	}

	if err := backend.Delete(t.Context(), "job-es-missing"); err != nil {
		t.Errorf("Delete() for missing id returned error: %v", err)
	}
}

// TestElasticsearchBackendQuery covers ES backend querying and counting: verifies filter, range conditions, sort, pagination, and Count/Query consistency all work correctly.
func TestElasticsearchBackendQuery(t *testing.T) {
	server := newMockElasticsearchServer()
	defer server.Close()

	backend := newBackendForTest(t, server)
	if backend == nil {
		return
	}

	err := backend.Init(t.Context(), "jobs", []driver.Index{
		{Field: "user_id"},
		{Field: "status"},
		{Field: "priority"},
		{Field: "created_at"},
	})
	if err != nil {
		t.Errorf("Init() returned error: %v", err)
		return
	}

	records := []driver.Record{
		{
			ID:   "job-es-alpha",
			Data: []byte(`{"id":"job-es-alpha","user_id":"user-alpha","status":"running","priority":10}`),
		},
		{
			ID:   "job-es-beta",
			Data: []byte(`{"id":"job-es-beta","user_id":"user-beta","status":"running","priority":6}`),
		},
		{
			ID:   "job-es-gamma",
			Data: []byte(`{"id":"job-es-gamma","user_id":"user-gamma","status":"completed","priority":3}`),
		},
	}

	for _, record := range records {
		if err := backend.Save(t.Context(), record); err != nil {
			t.Errorf("Save(%q) returned error: %v", record.ID, err)
		}
	}

	result, err := backend.Query(t.Context(), driver.Query{
		Filters: []driver.Filter{
			{Field: "status", Op: driver.OpEq, Value: "running"},
			{Field: "priority", Op: driver.OpGt, Value: 5},
		},
		Sorts: []driver.Sort{
			{Field: "priority", Desc: true},
		},
		Limit:  1,
		Offset: 0,
	})
	if err != nil {
		t.Errorf("Query() returned error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("Query() result length = %d, want 1", len(result))
	}
	if len(result) == 1 && result[0].ID != "job-es-alpha" {
		t.Errorf("Query() first id = %q, want %q", result[0].ID, "job-es-alpha")
	}

	count, err := backend.Count(t.Context(), driver.Query{
		Filters: []driver.Filter{
			{Field: "status", Op: driver.OpEq, Value: "running"},
			{Field: "priority", Op: driver.OpGt, Value: 5},
		},
	})
	if err != nil {
		t.Errorf("Count() returned error: %v", err)
	}
	if count != 2 {
		t.Errorf("Count() = %d, want 2", count)
	}

	server.mu.Lock()
	searchBodies := append([]map[string]any(nil), server.searchBodies...)
	countBodies := append([]map[string]any(nil), server.countBodies...)
	server.mu.Unlock()

	if len(searchBodies) != 1 {
		t.Errorf("search body count = %d, want 1", len(searchBodies))
	}
	if len(countBodies) != 1 {
		t.Errorf("count body count = %d, want 1", len(countBodies))
	}
}

// TestElasticsearchBackendTerms covers ES backend Terms aggregation: verifies the unified filter is applied during field aggregation and returns a deduplicated list of field values.
func TestElasticsearchBackendTerms(t *testing.T) {
	server := newMockElasticsearchServer()
	defer server.Close()

	backend := newBackendForTest(t, server)
	if backend == nil {
		return
	}

	err := backend.Init(t.Context(), "profiles", []driver.Index{
		{Field: "hostname"},
		{Field: "profile_type"},
		{Field: "time"},
	})
	if err != nil {
		t.Errorf("Init() returned error: %v", err)
		return
	}

	baseTime := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	records := []driver.Record{
		{
			ID:   "profile-alpha",
			Data: []byte(`{"tracer_id":"profile-alpha","hostname":"huatuo-dev","profile_type":"process_cpu:cpu:nanoseconds:cpu:nanoseconds","time":"2026-04-09 12:00:00.000 +0000"}`),
		},
		{
			ID:   "profile-beta",
			Data: []byte(`{"tracer_id":"profile-beta","hostname":"huatuo-dev","profile_type":"process_mem:alloc_objects:count:space:bytes","time":"2026-04-09 12:02:00.000 +0000"}`),
		},
		{
			ID:   "profile-gamma",
			Data: []byte(`{"tracer_id":"profile-gamma","hostname":"huatuo-dev","profile_type":"process_cpu:cpu:nanoseconds:cpu:nanoseconds","time":"2026-04-09 12:03:00.000 +0000"}`),
		},
	}

	for _, record := range records {
		if err := backend.Save(t.Context(), record); err != nil {
			t.Errorf("Save(%q) returned error: %v", record.ID, err)
		}
	}

	terms, err := backend.Values(t.Context(), "profile_type", driver.Query{
		Filters: []driver.Filter{
			{Field: "hostname", Op: driver.OpEq, Value: "huatuo-dev"},
			{Field: "time", Op: driver.OpGte, Value: baseTime.Add(-time.Minute)},
		},
	}, 10)
	if err != nil {
		t.Errorf("Terms() returned error: %v", err)
	}

	sort.Strings(terms)
	expectedTerms := []string{
		"process_cpu:cpu:nanoseconds:cpu:nanoseconds",
		"process_mem:alloc_objects:count:space:bytes",
	}
	if len(terms) != len(expectedTerms) {
		t.Errorf("Terms() count=%d, want %d", len(terms), len(expectedTerms))
	}
	for index, expectedTerm := range expectedTerms {
		if index >= len(terms) {
			break
		}
		if terms[index] != expectedTerm {
			t.Errorf("Terms()[%d]=%q, want %q", index, terms[index], expectedTerm)
		}
	}
}

// newMockServerWithoutProductHeader returns an httptest.Server that responds
// without the X-Elastic-Product header; our productHeaderTransport injects it.
func newMockServerWithoutProductHeader() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"mock-opensearch","version":{"number":"2.11.0","distribution":"opensearch"}}`))
	}))
}

// TestNewBackend_WithoutProductHeader verifies that NewBackend succeeds against
// a server that omits X-Elastic-Product; productHeaderTransport injects the
// header so the v8 client's product check passes.
func TestNewBackend_WithoutProductHeader(t *testing.T) {
	server := newMockServerWithoutProductHeader()
	defer server.Close()

	backend, err := NewBackend(&Config{Addresses: []string{server.URL}})
	if err != nil {
		t.Fatalf("NewBackend() returned error: %v", err)
	}
	if backend == nil {
		t.Fatal("NewBackend() returned nil backend")
	}
}

// TestNewBackend_ServerError verifies that NewBackend returns an error when the
// server responds with a non-2xx status on the Info call.
func TestNewBackend_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"simulated failure"}`))
	}))
	defer server.Close()

	_, err := NewBackend(&Config{Addresses: []string{server.URL}})
	if err == nil {
		t.Fatal("NewBackend() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "elasticsearch info probe") {
		t.Errorf("error = %q, want to contain \"elasticsearch info probe\"", err.Error())
	}
}
