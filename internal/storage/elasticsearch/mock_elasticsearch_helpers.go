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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	elasticsearchgo "github.com/elastic/go-elasticsearch/v7"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var origTransport http.RoundTripper // used to save old value

// MockRoundTripper is a reusable mock for http.RoundTripper.
type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)

	resp := args.Get(0)
	if resp == nil {
		return nil, args.Error(1)
	}

	switch v := resp.(type) {
	case *http.Response:
		return v, args.Error(1)

	case func(*http.Request) *http.Response:
		return v(req), args.Error(1)

	default:
		panic(fmt.Sprintf("unexpected RoundTrip return type: %T", v))
	}
}

// NewMockHTTPResponse creates a *http.Response with the given status, body and headers.
func NewMockHTTPResponse(status int, body string, headers map[string]string) *http.Response {
	h := http.Header{}
	for k, v := range headers {
		h.Set(k, v)
	}

	return &http.Response{
		StatusCode: status,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// NewErrorTransport returns a MockRoundTripper that always returns an error.
func NewErrorTransport(t *testing.T) *MockRoundTripper {
	t.Helper()
	rt := new(MockRoundTripper)
	rt.On("RoundTrip", mock.Anything).Return(nil, errors.New("simulated network error"))
	return rt
}

// newMockClient create and replace DefaultTransport，and return MockRoundTripper
func newMockClient() http.RoundTripper {
	origTransport = DefaultTransport
	DefaultTransport = &MockRoundTripper{}
	return DefaultTransport
}

// closeMockClient restore DefaultTransport
func closeMockClient() {
	DefaultTransport = origTransport
}

// newMockClientForWrite creates a mocked StorageClient for testing Write.
func newMockClientForWrite(t *testing.T, statusCode int, responseBody string) *StorageClient {
	t.Helper()

	rt := new(MockRoundTripper)

	rt.On("RoundTrip", mock.Anything).
		Return(func(req *http.Request) *http.Response {
			return NewMockHTTPResponse(
				statusCode,
				responseBody,
				map[string]string{
					"X-Elastic-Product": "Elasticsearch",
					"Content-Type":      "application/json",
				},
			)
		}, nil)

	cfg := elasticsearchgo.Config{
		Addresses: []string{"http://mock"},
		Transport: rt,
	}

	client, err := elasticsearchgo.NewClient(cfg)
	if err != nil {
		t.Fatalf("failed to create es client: %v", err)
	}

	t.Cleanup(func() {
		rt.AssertExpectations(t)
	})

	return &StorageClient{
		client: client,
		index:  DefaultIndex,
	}
}

// newMockClientForWriteWithError creates a mocked StorageClient whose requests
// fail at the transport level (RoundTrip returns an error).
func newMockClientForWriteWithError(t *testing.T) *StorageClient {
	t.Helper()

	// Mock transport to simulate network/request errors.
	rt := new(MockRoundTripper)
	rt.On("RoundTrip", mock.Anything).
		Return(nil, errors.New("simulated network error"))

	cfg := elasticsearchgo.Config{
		Addresses: []string{"http://mock"},
		Transport: rt,
	}

	client, err := elasticsearchgo.NewClient(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		rt.AssertExpectations(t)
	})

	return &StorageClient{
		client: client,
		index:  DefaultIndex,
	}
}

// newMockClientForWriteWithoutProductCheck creates a mocked StorageClient
// that skips Elasticsearch product verification during client creation.
func newMockClientForWriteWithoutProductCheck(t *testing.T, statusCode int, responseBody string) *StorageClient {
	t.Helper()

	// Mock transport to return a non-error HTTP response.
	rt := new(MockRoundTripper)
	rt.On("RoundTrip", mock.Anything).
		Return(func(req *http.Request) *http.Response {
			return NewMockHTTPResponse(
				statusCode,
				responseBody,
				map[string]string{
					"Content-Type": "application/json",
				},
			)
		}, nil)

	cfg := elasticsearchgo.Config{
		Addresses: []string{"http://mock"},
		Transport: rt,
		// Disable product header check triggered by the initial GET / request.
		UseResponseCheckOnly: true,
	}

	client, err := elasticsearchgo.NewClient(cfg)
	require.NoError(t, err)

	t.Cleanup(func() {
		rt.AssertExpectations(t)
	})

	return &StorageClient{
		client: client,
		index:  DefaultIndex,
	}
}
