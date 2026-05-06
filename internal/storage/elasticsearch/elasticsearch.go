// Copyright 2025 The HuaTuo Authors
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
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"huatuo-bamai/internal/storage/types"

	elasticsearchgo "github.com/elastic/go-elasticsearch/v7"
	elasticsearchapi "github.com/elastic/go-elasticsearch/v7/esapi"
)

const (
	DefaultIndex = "huatuo_bamai"
)

var DefaultTransport http.RoundTripper = &http.Transport{
	MaxIdleConnsPerHost:   10,
	ResponseHeaderTimeout: 10 * time.Second,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	TLSClientConfig: &tls.Config{
		// #nosec G402
		InsecureSkipVerify: true,
	},
}

type StorageClient struct {
	client *elasticsearchgo.Client
	index  string
}

func NewStorageClient(addr, username, password, index string) (*StorageClient, error) {
	cfg := elasticsearchgo.Config{
		Addresses: []string{addr},
		Username:  username,
		Password:  password,
		Transport: DefaultTransport,
	}

	client, err := elasticsearchgo.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	// ping/check es server ...
	res, err := client.Info()
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch return statuscode: %d", res.StatusCode)
	}

	if index == "" {
		index = DefaultIndex
	}
	return &StorageClient{client: client, index: index}, nil
}

// IndexRequest is a function that performs the actual index request.
type IndexRequest func(ctx context.Context, body io.Reader) (*http.Response, error)

// WriteDocument, the common write logic for Elasticsearch and OpenSearch backends.
// 1. serializes the document
// 2. calls the request provider
// 3. checks the HTTP status, and validates the response body.
func WriteDocument(index string, doc *types.Document, do IndexRequest) error {
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("json marshal: %w", err)
	}

	res, err := do(context.Background(), strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("error executing index request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("index document failed with status: %d, %s", res.StatusCode, string(body))
	}

	var r map[string]any
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return fmt.Errorf("parse response body: %w", err)
	}

	return nil
}

// Write the document into ES
func (e *StorageClient) Write(doc *types.Document) error {
	return WriteDocument(e.index, doc, func(ctx context.Context, body io.Reader) (*http.Response, error) {
		req := elasticsearchapi.IndexRequest{
			Index: e.index,
			Body:  body,
		}
		res, err := req.Do(ctx, e.client)
		if err != nil {
			return nil, err
		}

		return &http.Response{
			StatusCode: res.StatusCode,
			Header:     res.Header,
			Body:       res.Body,
		}, nil
	})
}
