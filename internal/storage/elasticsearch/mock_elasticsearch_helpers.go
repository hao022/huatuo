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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

type mockElasticsearchDocument struct {
	ID     string
	Source json.RawMessage
	Fields map[string]any
}

type mockElasticsearchServer struct {
	mu                sync.Mutex
	indexes           map[string]map[string]mockElasticsearchDocument
	createIndexBodies []map[string]any
	searchBodies      []map[string]any
	countBodies       []map[string]any
	server            *httptest.Server
}

func newMockElasticsearchServer() *mockElasticsearchServer {
	mockServer := &mockElasticsearchServer{
		indexes: make(map[string]map[string]mockElasticsearchDocument),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Elastic-Product", "Elasticsearch")

		path := strings.Trim(r.URL.Path, "/")
		if path == "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"mock-es","version":{"number":"7.17.0"}}`))
			return
		}

		parts := strings.Split(path, "/")
		switch {
		case len(parts) == 1 && r.Method == http.MethodHead:
			mockServer.handleIndexExists(w, parts[0])
		case len(parts) == 1 && r.Method == http.MethodPut:
			mockServer.handleCreateIndex(w, r, parts[0])
		case len(parts) == 3 && parts[1] == "_doc" && (r.Method == http.MethodPut || r.Method == http.MethodPost):
			mockServer.handleSaveDocument(w, r, parts[0], parts[2])
		case len(parts) == 3 && parts[1] == "_doc" && r.Method == http.MethodGet:
			mockServer.handleGetDocument(w, parts[0], parts[2])
		case len(parts) == 3 && parts[1] == "_doc" && r.Method == http.MethodDelete:
			mockServer.handleDeleteDocument(w, parts[0], parts[2])
		case len(parts) == 2 && parts[1] == "_search":
			mockServer.handleSearch(w, r, parts[0])
		case len(parts) == 2 && parts[1] == "_count":
			mockServer.handleCount(w, r, parts[0])
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"route not found"}`))
		}
	})

	mockServer.server = httptest.NewServer(handler)
	return mockServer
}

func (m *mockElasticsearchServer) Close() {
	if m == nil || m.server == nil {
		return
	}
	m.server.Close()
}

func (m *mockElasticsearchServer) URL() string {
	if m == nil || m.server == nil {
		return ""
	}
	return m.server.URL
}

func (m *mockElasticsearchServer) handleIndexExists(w http.ResponseWriter, index string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexes[index]; ok {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (m *mockElasticsearchServer) handleCreateIndex(w http.ResponseWriter, r *http.Request, index string) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexes[index]; !ok {
		m.indexes[index] = make(map[string]mockElasticsearchDocument)
	}
	m.createIndexBodies = append(m.createIndexBodies, body)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"acknowledged":true}`))
}

func (m *mockElasticsearchServer) handleSaveDocument(w http.ResponseWriter, r *http.Request, index, id string) {
	var raw json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&raw)

	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexes[index]; !ok {
		m.indexes[index] = make(map[string]mockElasticsearchDocument)
	}
	m.indexes[index][id] = mockElasticsearchDocument{
		ID:     id,
		Source: cloneRawMessage(raw),
		Fields: fields,
	}

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"result":"created"}`))
}

func (m *mockElasticsearchServer) handleGetDocument(w http.ResponseWriter, index, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	docs, ok := m.indexes[index]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"found":false}`))
		return
	}

	doc, ok := docs[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"found":false}`))
		return
	}

	resp := map[string]any{
		"_id":     id,
		"found":   true,
		"_source": doc.Source,
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *mockElasticsearchServer) handleDeleteDocument(w http.ResponseWriter, index, id string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if docs, ok := m.indexes[index]; ok {
		if _, found := docs[id]; found {
			delete(docs, id)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"result":"deleted"}`))
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"result":"not_found"}`))
}

func (m *mockElasticsearchServer) handleSearch(w http.ResponseWriter, r *http.Request, index string) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchBodies = append(m.searchBodies, body)
	if body["aggs"] != nil {
		docs := m.matchDocumentsLocked(index, body["query"])
		m.handleTermsSearch(w, body, docs)
		return
	}
	docs := m.queryDocumentsLocked(index, body)

	hits := make([]map[string]any, 0, len(docs))
	for _, doc := range docs {
		hits = append(hits, map[string]any{
			"_id":     doc.ID,
			"_source": doc.Source,
		})
	}

	resp := map[string]any{
		"hits": map[string]any{
			"total": map[string]any{
				"value":    len(docs),
				"relation": "eq",
			},
			"hits": hits,
		},
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *mockElasticsearchServer) handleTermsSearch(w http.ResponseWriter, body map[string]any, docs []mockElasticsearchDocument) {
	aggs, _ := body["aggs"].(map[string]any)
	termsAggregation, _ := aggs["terms"].(map[string]any)
	termsConfig, _ := termsAggregation["terms"].(map[string]any)
	fieldName := stringValue(termsConfig["field"])

	counts := make(map[string]int)
	for _, doc := range docs {
		key := stringValue(doc.Fields[fieldName])
		if key == "" {
			continue
		}
		counts[key]++
	}

	type bucket struct {
		Key   string
		Count int
	}

	buckets := make([]bucket, 0, len(counts))
	for key, count := range counts {
		buckets = append(buckets, bucket{Key: key, Count: count})
	}

	sort.SliceStable(buckets, func(i, j int) bool {
		if buckets[i].Count == buckets[j].Count {
			return buckets[i].Key < buckets[j].Key
		}
		return buckets[i].Count > buckets[j].Count
	})

	responseBuckets := make([]map[string]any, 0, len(buckets))
	for _, item := range buckets {
		responseBuckets = append(responseBuckets, map[string]any{
			"key":       item.Key,
			"doc_count": item.Count,
		})
	}

	resp := map[string]any{
		"hits": map[string]any{
			"total": map[string]any{
				"value":    len(docs),
				"relation": "eq",
			},
			"hits": []any{},
		},
		"aggregations": map[string]any{
			"terms": map[string]any{
				"buckets": responseBuckets,
			},
		},
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (m *mockElasticsearchServer) handleCount(w http.ResponseWriter, r *http.Request, index string) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.countBodies = append(m.countBodies, body)
	docs := m.queryDocumentsLocked(index, body)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"count": len(docs)})
}

func (m *mockElasticsearchServer) queryDocumentsLocked(index string, body map[string]any) []mockElasticsearchDocument {
	docs := m.matchDocumentsLocked(index, body["query"])

	applySorts(docs, body["sort"])

	from := intFromAny(body["from"])
	if from > len(docs) {
		return []mockElasticsearchDocument{}
	}

	size := len(docs)
	if rawSize, ok := body["size"]; ok {
		size = intFromAny(rawSize)
	}
	if size < 0 {
		size = 0
	}

	end := len(docs)
	if from+size < end {
		end = from + size
	}
	if from > end {
		return []mockElasticsearchDocument{}
	}

	return append([]mockElasticsearchDocument(nil), docs[from:end]...)
}

func (m *mockElasticsearchServer) matchDocumentsLocked(index string, rawQuery any) []mockElasticsearchDocument {
	docsByID := m.indexes[index]
	docs := make([]mockElasticsearchDocument, 0, len(docsByID))
	for _, doc := range docsByID {
		if matchesQuery(doc, rawQuery) {
			docs = append(docs, doc)
		}
	}
	return docs
}

func matchesQuery(doc mockElasticsearchDocument, rawQuery any) bool {
	queryMap, ok := rawQuery.(map[string]any)
	if !ok || len(queryMap) == 0 {
		return true
	}
	if _, ok := queryMap["match_all"]; ok {
		return true
	}

	boolQuery, ok := queryMap["bool"].(map[string]any)
	if !ok {
		return true
	}

	for _, clause := range toAnySlice(boolQuery["filter"]) {
		if !matchesClause(doc, clause) {
			return false
		}
	}
	for _, clause := range toAnySlice(boolQuery["must_not"]) {
		if matchesClause(doc, clause) {
			return false
		}
	}

	return true
}

func matchesClause(doc mockElasticsearchDocument, rawClause any) bool {
	clause, ok := rawClause.(map[string]any)
	if !ok {
		return false
	}

	if rawTerm, ok := clause["term"].(map[string]any); ok {
		for path, rawValue := range rawTerm {
			// Handle both short form {"field": "val"} and long form {"field": {"value": "val"}}.
			var value any
			if m, ok := rawValue.(map[string]any); ok {
				value = m["value"]
			} else {
				value = rawValue
			}
			return valuesEqual(fieldValue(doc, path), value)
		}
	}

	if rawTerms, ok := clause["terms"].(map[string]any); ok {
		for path, values := range rawTerms {
			current := fieldValue(doc, path)
			for _, value := range toAnySlice(values) {
				if valuesEqual(current, value) {
					return true
				}
			}
			return false
		}
	}

	if rawRange, ok := clause["range"].(map[string]any); ok {
		for path, conditions := range rawRange {
			conditionMap, ok := conditions.(map[string]any)
			if !ok {
				return false
			}
			current := fieldValue(doc, path)
			for operator, value := range conditionMap {
				compareResult, comparable := compareValues(current, value)
				if !comparable {
					return false
				}
				switch operator {
				case "gt":
					if compareResult <= 0 {
						return false
					}
				case "gte":
					if compareResult < 0 {
						return false
					}
				case "lt":
					if compareResult >= 0 {
						return false
					}
				case "lte":
					if compareResult > 0 {
						return false
					}
				default:
					return false
				}
			}
			return true
		}
	}

	return false
}

func applySorts(docs []mockElasticsearchDocument, rawSort any) {
	sortClauses := toAnySlice(rawSort)
	if len(sortClauses) == 0 {
		sort.SliceStable(docs, func(i, j int) bool {
			return docs[i].ID < docs[j].ID
		})
		return
	}

	sort.SliceStable(docs, func(i, j int) bool {
		left := docs[i]
		right := docs[j]

		for _, rawClause := range sortClauses {
			clause, ok := rawClause.(map[string]any)
			if !ok {
				continue
			}
			for path, rawOptions := range clause {
				options, _ := rawOptions.(map[string]any)
				desc := strings.EqualFold(stringValue(options["order"]), "desc")
				compareResult, comparable := compareValues(fieldValue(left, path), fieldValue(right, path))
				if !comparable || compareResult == 0 {
					continue
				}
				if desc {
					return compareResult > 0
				}
				return compareResult < 0
			}
		}

		return left.ID < right.ID
	})
}

func fieldValue(doc mockElasticsearchDocument, path string) any {
	return doc.Fields[path]
}

func valuesEqual(left, right any) bool {
	if leftFloat, ok := toFloat64(left); ok {
		rightFloat, ok := toFloat64(right)
		if !ok {
			return false
		}
		return leftFloat == rightFloat
	}
	return stringValue(left) == stringValue(right)
}

func compareValues(left, right any) (int, bool) {
	if leftFloat, ok := toFloat64(left); ok {
		rightFloat, ok := toFloat64(right)
		if !ok {
			return 0, false
		}
		switch {
		case leftFloat < rightFloat:
			return -1, true
		case leftFloat > rightFloat:
			return 1, true
		default:
			return 0, true
		}
	}

	leftString := stringValue(left)
	rightString := stringValue(right)
	switch {
	case leftString < rightString:
		return -1, true
	case leftString > rightString:
		return 1, true
	default:
		return 0, true
	}
}

func toFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func toAnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result
	default:
		return nil
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.RawMessage:
		return string(typed)
	case nil:
		return ""
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}

func cloneRawMessage(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), data...)
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	if len(raw) == 0 {
		return nil
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("json.Unmarshal() returned error: %v", err)
	}
	return result
}

func newBackendForTest(t *testing.T, server *mockElasticsearchServer) *Storage {
	t.Helper()

	backend, err := NewBackend(&Config{
		Addresses: []string{server.URL()},
		Index:     "huatuo_bamai",
	})
	if err != nil {
		t.Fatalf("NewBackend() returned error: %v", err)
	}
	return backend
}
