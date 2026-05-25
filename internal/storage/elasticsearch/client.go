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
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

var defaultTransport http.RoundTripper = &http.Transport{
	MaxIdleConnsPerHost:   10,
	ResponseHeaderTimeout: 10 * time.Second,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, // #nosec G402
	},
}

// productHeaderTransport injects X-Elastic-Product: Elasticsearch into
// responses that lack it. Required for ES v7 < 7.14 which pre-dates the header.
type productHeaderTransport struct {
	inner http.RoundTripper
}

func (t *productHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.Header.Get("X-Elastic-Product") == "" {
		resp.Header.Set("X-Elastic-Product", "Elasticsearch")
	}
	return resp, nil
}

// newCompatTransport creates a transport that works with ES v7/v8 and OpenSearch.
func newCompatTransport(addresses []string, username, password string) (esapi.Transport, error) {
	urls := make([]*url.URL, 0, len(addresses))
	for _, addr := range addresses {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, fmt.Errorf("elasticsearch parse address %q: %w", addr, err)
		}
		urls = append(urls, u)
	}
	if len(urls) == 0 {
		u, _ := url.Parse("http://localhost:9200")
		urls = append(urls, u)
	}

	tp, err := elastictransport.New(elastictransport.Config{
		URLs:      urls,
		Username:  username,
		Password:  password,
		Transport: &productHeaderTransport{inner: defaultTransport},
	})
	if err != nil {
		return nil, fmt.Errorf("elasticsearch new transport: %w", err)
	}

	// Probe connectivity.
	req := esapi.InfoRequest{}
	res, err := req.Do(context.Background(), tp)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch info probe: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch info probe: status %d", res.StatusCode)
	}
	return tp, nil
}
