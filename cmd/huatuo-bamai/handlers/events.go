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
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"huatuo-bamai/internal/matcher"
	"huatuo-bamai/internal/server"
	"huatuo-bamai/internal/server/response"
	"huatuo-bamai/internal/storage"
	"huatuo-bamai/internal/storage/types"
)

const (
	defaultMaxClients        = 100
	defaultKeepAliveInterval = 30 * time.Second
	maxKeepAliveFailures     = 3
)

// EventsHandler handles kernel event streaming over SSE.
type EventsHandler struct {
	Handlers          []server.Handle
	maxClients        int
	keepAliveInterval time.Duration
	activeClients     atomic.Int32
}

// NewEventsHandler constructs an EventsHandler.
// maxClients is the maximum number of concurrent /v1/events/watch connections;
// zero or negative values fall back to defaultMaxClients.
// keepAliveIntervalSecs is the SSE heartbeat interval in seconds;
// zero or negative values fall back to defaultKeepAliveInterval.
func NewEventsHandler(maxClients, keepAliveIntervalSecs int) *EventsHandler {
	if maxClients <= 0 {
		maxClients = defaultMaxClients
	}
	keepAlive := time.Duration(keepAliveIntervalSecs) * time.Second
	if keepAlive <= 0 {
		keepAlive = defaultKeepAliveInterval
	}

	h := &EventsHandler{
		maxClients:        maxClients,
		keepAliveInterval: keepAlive,
	}
	h.Handlers = []server.Handle{
		{Typ: server.HttpPost, Uri: "/watch", Handle: h.watch},
	}
	return h
}

// WatchRequest is the POST body sent by a client to register an event watch.
// All filter fields are optional regex patterns; omitting a field matches all values.
// Additional filter fields can be added to WatchFilters without breaking existing clients.
type WatchRequest struct {
	Filters WatchFilters `json:"filters"`
}

// WatchFilters holds optional regex patterns for the fields callers care about.
// Each non-empty pattern is compiled and matched against the corresponding
// Document field; all non-empty patterns must match for an event to be delivered.
type WatchFilters struct {
	TracerName             string `json:"tracer_name,omitempty"`
	Hostname               string `json:"hostname,omitempty"`
	ContainerHostname      string `json:"container_hostname,omitempty"`
	ContainerHostNamespace string `json:"container_host_namespace,omitempty"`
	Region                 string `json:"region,omitempty"`
}

// matcher builds a matcher.FieldMatcher[*types.Document] from the filter's regex patterns.
func (wf *WatchFilters) matcher() (*matcher.FieldMatcher[*types.Document], error) {
	return matcher.NewFieldMatcher([]matcher.FieldSpec[*types.Document]{
		{
			Name:    "tracer_name",
			Pattern: wf.TracerName,
			Extract: func(d *types.Document) string { return d.TracerName },
		},
		{
			Name:    "hostname",
			Pattern: wf.Hostname,
			Extract: func(d *types.Document) string { return d.Hostname },
		},
		{
			Name:    "container_hostname",
			Pattern: wf.ContainerHostname,
			Extract: func(d *types.Document) string { return d.ContainerHostname },
		},
		{
			Name:    "container_host_namespace",
			Pattern: wf.ContainerHostNamespace,
			Extract: func(d *types.Document) string { return d.ContainerHostNamespace },
		},
		{
			Name:    "region",
			Pattern: wf.Region,
			Extract: func(d *types.Document) string { return d.Region },
		},
	})
}

// watch is the POST /v1/events/watch handler. It registers a storage subscriber,
// applies the caller-supplied filters, and streams matching events as SSE until
// the client disconnects, the server shuts down, or keepalive probes fail
// maxKeepAliveFailures consecutive times.
func (h *EventsHandler) watch(ctx *server.Context) error {
	if int(h.activeClients.Load()) >= h.maxClients {
		return response.ErrTooManyRequests.WithMessage("max watch clients reached")
	}
	h.activeClients.Add(1)
	defer h.activeClients.Add(-1)

	var req WatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		handleBindError(ctx, err)
		return nil
	}

	matcher, err := req.Filters.matcher()
	if err != nil {
		return response.ErrInvalidRequest.WithMessage(err.Error())
	}

	flusher, ok := ctx.Writer().(http.Flusher)
	if !ok {
		return response.ErrInternal.WithMessage("response writer does not support streaming")
	}

	ctx.Header("Content-Type", "text/event-stream")
	ctx.Header("Cache-Control", "no-cache")
	ctx.Header("Connection", "keep-alive")
	ctx.Header("X-Accel-Buffering", "no")

	docCh, cancel := storage.Subscribe()
	defer cancel()

	ticker := time.NewTicker(h.keepAliveInterval)
	defer ticker.Stop()

	clientGone := ctx.Request().Context().Done()
	var pingFailures int

	for {
		select {
		case <-clientGone:
			return nil

		case <-ticker.C:
			// Send an SSE comment line (RFC 8895 §9.1). Comment lines start
			// with ':' and are silently discarded by SSE clients at the
			// application layer, so they never surface as events. Their sole
			// purpose is to push bytes through the TCP connection so that
			// intermediate proxies and load balancers do not treat the idle
			// connection as stale and close it prematurely.
			// A single '\n' is used (not '\n\n') to avoid triggering a
			// spurious empty-event dispatch in the client's SSE parser.
			if _, err := fmt.Fprint(ctx.Writer(), ": ping\n"); err != nil {
				pingFailures++
				if pingFailures >= maxKeepAliveFailures {
					return nil
				}
			} else {
				pingFailures = 0
				flusher.Flush()
			}

		case doc, ok := <-docCh:
			if !ok {
				return nil
			}
			if !matcher.Match(doc) {
				continue
			}
			event := DocumentToWatchEvent(doc)
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(ctx.Writer(), "data: %s\n\n", data); err != nil {
				pingFailures++
				if pingFailures >= maxKeepAliveFailures {
					return nil
				}
			} else {
				pingFailures = 0
				flusher.Flush()
			}
		}
	}
}
