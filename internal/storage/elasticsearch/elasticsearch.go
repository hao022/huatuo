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

// Package elasticsearch implements a storage backend compatible with
// Elasticsearch v7/v8 and OpenSearch using the go-elasticsearch/v8 esapi.
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"

	"huatuo-bamai/internal/storage/driver"
)

const (
	defaultIndex     = "huatuo_bamai"
	defaultQuerySize = 10000
)

// Config contains Elasticsearch backend settings.
type Config struct {
	Addresses []string
	Username  string
	Password  string
	Index     string
}

// Storage stores records in Elasticsearch, OpenSearch, or any compatible backend.
type Storage struct {
	transport esapi.Transport
	index     string
}

var _ driver.Backend = (*Storage)(nil)

func init() {
	factory := func(cfg *driver.Config) (driver.Backend, error) {
		return NewBackend(&Config{
			Addresses: cfg.ESAddresses,
			Username:  cfg.ESUsername,
			Password:  cfg.ESPassword,
			Index:     cfg.ESIndex,
		})
	}
	driver.RegisterBackend("elasticsearch", factory)
	driver.RegisterBackend("opensearch", factory)
}

// NewBackend creates a backend that connects to Elasticsearch v7/v8 or OpenSearch.
func NewBackend(cfg *Config) (*Storage, error) {
	prefix := cfg.Index
	if prefix == "" {
		prefix = defaultIndex
	}
	transport, err := newCompatTransport(cfg.Addresses, cfg.Username, cfg.Password)
	if err != nil {
		return nil, err
	}
	return &Storage{transport: transport, index: prefix}, nil
}

func (s *Storage) Init(_ context.Context, _ string, indexes []driver.Index) error {
	for _, idx := range indexes {
		if err := validateFieldName(idx.Field); err != nil {
			return err
		}
	}
	return nil
}

func (s *Storage) Save(ctx context.Context, rec driver.Record) error {
	req := esapi.IndexRequest{
		Index:      s.index,
		DocumentID: rec.ID,
		Body:       bytes.NewReader(rec.Data),
	}
	res, err := req.Do(driver.WithContext(ctx), s.transport)
	if err != nil {
		return fmt.Errorf("elasticsearch backend save %s: %w", s.index, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return responseError("save document", s.index, res)
	}
	return nil
}

func (s *Storage) Get(ctx context.Context, id string) (rec driver.Record, err error) {
	req := esapi.GetRequest{Index: s.index, DocumentID: id}
	res, err := req.Do(driver.WithContext(ctx), s.transport)
	if err != nil {
		return rec, fmt.Errorf("elasticsearch backend get %s/%s: %w", s.index, id, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return rec, driver.ErrNotFound
	}
	if res.IsError() {
		return rec, responseError("get document", s.index, res)
	}

	var payload getResponse
	if err = json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return rec, fmt.Errorf("elasticsearch backend get %s/%s: decode: %w", s.index, id, err)
	}
	if !payload.Found {
		return rec, driver.ErrNotFound
	}
	recordID := payload.ID
	if recordID == "" {
		recordID = id
	}
	return driver.Record{ID: recordID, Data: driver.CloneBytes(payload.Source)}, nil
}

func (s *Storage) Delete(ctx context.Context, id string) error {
	req := esapi.DeleteRequest{Index: s.index, DocumentID: id, Refresh: "true"}
	res, err := req.Do(driver.WithContext(ctx), s.transport)
	if err != nil {
		return fmt.Errorf("elasticsearch backend delete %s/%s: %w", s.index, id, err)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.IsError() {
		return responseError("delete document", s.index, res)
	}
	return nil
}

func (s *Storage) Query(ctx context.Context, q driver.Query) ([]driver.Record, error) {
	body, err := buildSearchRequest(q)
	if err != nil {
		return nil, err
	}

	req := esapi.SearchRequest{Index: []string{s.index}, Body: bytes.NewReader(body)}
	res, err := req.Do(driver.WithContext(ctx), s.transport)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch backend query %s: %w", s.index, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, responseError("query documents", s.index, res)
	}

	var payload searchResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("elasticsearch backend query %s: decode: %w", s.index, err)
	}
	records := make([]driver.Record, 0, len(payload.Hits.Hits))
	for i := range payload.Hits.Hits {
		hit := &payload.Hits.Hits[i]
		records = append(records, driver.Record{ID: hit.ID, Data: driver.CloneBytes(hit.Source)})
	}
	return records, nil
}

func (s *Storage) Count(ctx context.Context, q driver.Query) (int64, error) {
	body, err := buildCountRequest(q)
	if err != nil {
		return 0, err
	}

	req := esapi.CountRequest{Index: []string{s.index}, Body: bytes.NewReader(body)}
	res, err := req.Do(driver.WithContext(ctx), s.transport)
	if err != nil {
		return 0, fmt.Errorf("elasticsearch backend count %s: %w", s.index, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, responseError("count documents", s.index, res)
	}

	var payload countResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("elasticsearch backend count %s: decode: %w", s.index, err)
	}
	return payload.Count, nil
}

func (s *Storage) Values(ctx context.Context, field string, q driver.Query, size int) ([]string, error) {
	body, err := buildValuesRequest(field, q, size)
	if err != nil {
		return nil, err
	}

	req := esapi.SearchRequest{Index: []string{s.index}, Body: bytes.NewReader(body)}
	res, err := req.Do(driver.WithContext(ctx), s.transport)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch backend terms %s/%s: %w", s.index, field, err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, responseError("terms aggregation", s.index, res)
	}

	var payload valuesResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("elasticsearch backend terms %s/%s: decode: %w", s.index, field, err)
	}
	result := make([]string, 0, len(payload.Aggregations.Terms.Buckets))
	for _, bucket := range payload.Aggregations.Terms.Buckets {
		result = append(result, driver.StringValue(bucket.Key))
	}
	return result, nil
}

func responseError(action, target string, res *esapi.Response) error {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("elasticsearch %s %s: status %d: read body: %w", action, target, res.StatusCode, err)
	}
	return fmt.Errorf("elasticsearch %s %s: status %d: %s", action, target, res.StatusCode, strings.TrimSpace(string(body)))
}
