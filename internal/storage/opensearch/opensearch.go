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
	"context"
	"fmt"
	"io"
	"net/http"

	"huatuo-bamai/internal/storage/elasticsearch"
	"huatuo-bamai/internal/storage/types"

	"github.com/opensearch-project/opensearch-go/v2"
	"github.com/opensearch-project/opensearch-go/v2/opensearchapi"
)

type StorageClient struct {
	client *opensearch.Client
	index  string
}

func NewStorageClient(addr, username, password, index string) (*StorageClient, error) {
	cfg := opensearch.Config{
		Addresses: []string{addr},
		Username:  username,
		Password:  password,
		Transport: elasticsearch.DefaultTransport,
	}

	client, err := opensearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new opensearch client: %w", err)
	}

	// Health check via Info API
	res, err := client.Info()
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("opensearch returned status code: %d", res.StatusCode)
	}

	if index == "" {
		index = elasticsearch.DefaultIndex
	}
	return &StorageClient{client: client, index: index}, nil
}

// Write indexes a document into OpenSearch.
func (c *StorageClient) Write(doc *types.Document) error {
	return elasticsearch.WriteDocument(c.index, doc, func(ctx context.Context, body io.Reader) (*http.Response, error) {
		req := opensearchapi.IndexRequest{
			Index: c.index,
			Body:  body,
		}

		res, err := req.Do(ctx, c.client)
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
