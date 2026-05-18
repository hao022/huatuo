---
title: Events Watch
type: docs
description: Subscribe to kernel events in real time via CloudEvents over SSE
author: HUATUO Team
date: 2026-05-18
weight: 3
---

## Overview

`/v1/events/watch` is HUATUO's real-time kernel event subscription endpoint. A single HTTP POST long-poll connection delivers a continuous stream of kernel anomaly events from the node. Events are wrapped in a [CloudEvents 1.0](https://cloudevents.io/) envelope and pushed over [Server-Sent Events (SSE)](https://html.spec.whatwg.org/multipage/server-sent-events.html).

---

## Use Cases and Value

Subscribing to kernel events exposes low-level OS anomaly signals directly to upper-layer systems, eliminating the latency and overhead of traditional polling. The following are typical integration scenarios.

### Self-Healing Systems

Kernel events are first-hand signals for autonomous remediation decisions. With `events/watch`, a self-healing controller can act at the moment an event fires rather than waiting for an alerting pipeline to process it:

- **OOM self-healing**: On receiving an `oom` event, immediately scale out, restart, or shed traffic from the affected container — compressing service interruption from minutes to seconds.
- **Hung Task isolation**: On `hungtask`, automatically cordon the node and evict Pods before a cascading stall spreads across the cluster.
- **Network self-healing**: On `netdev_txqueue_timeout` or `netdev_bonding_lacp`, trigger NIC reset or traffic failover to restore network links within minutes.
- **I/O storm mitigation**: On `iotracing`, dynamically throttle the offending container's disk I/O via cgroup blkio to protect co-located workloads.

### Observability Platforms

Integrating HUATUO kernel events fills the "kernel perspective" gap left by application metrics and logs:

- **Event timeline correlation**: Overlay `softlockup`, `oom`, and similar events on Grafana time series alongside application error rates and latency curves to pinpoint root causes precisely.
- **Anomaly-driven alerting**: Replace fixed-threshold alerts with kernel event triggers to eliminate false positives and false negatives — for example, a `ras` hardware error fires a high-priority alert immediately rather than waiting for a CPU error rate threshold to breach.
- **Capacity and stability analysis**: Continuously subscribe to `memburst` and `dload` AutoTracing events to build node stability baselines and inform capacity planning with kernel-level evidence.
- **Multi-dimensional drill-down**: Events carry container ID, namespace, region, and other context so alert links can navigate directly to the relevant Pod, Node, or Region view.

### Security Auditing and Compliance

- **Anomaly behavior detection**: A cluster of `oom`, `hungtask`, or `softlockup` events outside peak hours may indicate resource abuse or malicious workloads and can trigger a security review workflow.
- **Event retention and traceability**: Writing the CloudEvents stream to a message queue (Kafka, Pulsar) or object storage satisfies compliance requirements for retaining system anomaly records.

### Chaos Engineering and Load Testing

- **Fault injection verification**: After injecting network latency or memory pressure with a chaos platform, subscribe to `net_rx_latency` or `memburst` events in real time to confirm the fault took effect without manual observation.
- **Load test baseline calibration**: Continuously subscribe to all events during a load test; recording the timestamp of the first kernel anomaly event precisely marks the system's stress threshold.

### AIOps and Intelligent Operations

- **Event-driven root cause analysis**: Feed kernel events as features into AI/ML models alongside application metrics to perform multi-dimensional root cause inference and reduce manual investigation time.
- **Predictive maintenance**: Model `ras` hardware errors and `netdev_bonding_lacp` events to predict hardware failures before they become complete outages and trigger proactive workload migration.
- **Intelligent suppression and aggregation**: Automatically deduplicate and aggregate bursts of the same event type within a time window to prevent alert storms and present on-call engineers with a concise root cause summary.

### Why events/watch

| Dimension         | Traditional approach                          | HUATUO events/watch                                    |
|-----------------|-----------------------------------------------|--------------------------------------------------------|
| Timeliness      | Alert delay of 1–5 minutes                    | Real-time kernel push, latency < 1 second              |
| Signal accuracy | Threshold-based metrics, high false-alarm rate | Sourced directly from the kernel, zero false positives |
| Context richness | Limited metric dimensions                    | Full context: container, node, region, and more        |
| Integration cost | Build custom eBPF collectors or install agents | Single HTTP POST; standard CloudEvents format          |
| Protocol compatibility | Proprietary vendor formats            | CloudEvents 1.0 standard; works with any compatible platform |

---

## 1. CloudEvents Specification

### 1.1 CloudEvents 1.0 Envelope Fields

Every pushed event is a JSON object conforming to the CloudEvents 1.0 specification:

| Field              | Type   | Description                                                           |
|------------------|--------|-----------------------------------------------------------------------|
| `specversion`    | string | Always `"1.0"`                                                        |
| `id`             | string | Unique event identifier (UUID v4), generated independently per event  |
| `source`         | string | Event origin path: `/huatuo/{hostname}/{tracer_name}`                 |
| `type`           | string | Always `"tech.huatuo.kernel.event"`                                   |
| `datacontenttype` | string | Always `"application/json"`                                          |
| `time`           | string | Event capture time (RFC 3339 with nanosecond precision, UTC)          |
| `data`           | object | Event payload — the `WatchEventData` object                           |

### 1.2 HUATUO Event Payload (WatchEventData)

The `data` field contains HUATUO's standard event record:

```json
{
  "specversion": "1.0",
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "source": "/huatuo/node-1/oom",
  "type": "tech.huatuo.kernel.event",
  "datacontenttype": "application/json",
  "time": "2026-05-18T10:23:45.123456789Z",
  "data": {
    "hostname": "node-1",
    "region": "cn-beijing",
    "observed_timestamp": "2026-05-18T10:23:45Z",
    "tracer_name": "oom",
    "tracer_id": "abc123",
    "tracer_run_type": "auto",
    "container_id": "d3f1a2b4c5e6",
    "container_hostname": "app-pod",
    "container_host_namespace": "prod",
    "container_type": "docker",
    "container_qos": "Guaranteed"
  }
}
```

**WatchEventData field reference:**

| Field                      | Type   | Description                                               |
|--------------------------|--------|-----------------------------------------------------------|
| `hostname`               | string | Node hostname                                             |
| `region`                 | string | Node region                                               |
| `observed_timestamp`     | string | Time the kernel event was captured by the tracer          |
| `tracer_name`            | string | Name of the collector that triggered the event (see list below) |
| `tracer_id`              | string | Unique ID for this event instance                         |
| `tracer_run_type`        | string | Collection mode: `auto` (auto-triggered) or `manual`      |
| `container_id`           | string | Container ID (present for container-level events)         |
| `container_hostname`     | string | Container hostname                                        |
| `container_host_namespace` | string | Kubernetes namespace of the container                   |
| `container_type`         | string | Container runtime type (docker, containerd, etc.)         |
| `container_qos`          | string | Container QoS class                                       |

---

## 2. Supported Kernel Events

| `tracer_name`              | Description                                                   |
|--------------------------|---------------------------------------------------------------|
| `oom`                    | Out-of-Memory Killer (OOM) trigger events                     |
| `hungtask`               | Kernel task stuck in D state (Hung Task) detection            |
| `softlockup`             | CPU soft lockup detection                                     |
| `ras`                    | Hardware reliability (RAS) errors, e.g. ECC memory errors     |
| `dropwatch`              | Kernel network packet drop events                             |
| `netdev_events`          | Network device state changes (Link Up/Down, etc.)             |
| `netdev_txqueue_timeout` | Network device transmit queue timeout events                  |
| `netdev_bonding_lacp`    | Bond device LACP protocol anomaly events                      |
| `net_rx_latency`         | Network receive latency anomaly events                        |
| `softirq_tracing`        | Soft interrupt latency anomaly tracing events                 |
| `memory_reclaim_events`  | Memory reclaim anomaly events                                 |
| `cpuidle`                | CPU idle rate anomaly (AutoTracing, auto-triggered)           |
| `cpusys`                 | CPU system time anomaly (AutoTracing, auto-triggered)         |
| `dload`                  | System load average anomaly (AutoTracing, auto-triggered)     |
| `iotracing`              | I/O latency anomaly (AutoTracing, auto-triggered)             |
| `memburst`               | Memory burst anomaly (AutoTracing, auto-triggered)            |

---

## 3. POST Request Reference

### 3.1 Endpoint

```
POST /v1/events/watch
```

### 3.2 Request Headers

```
Content-Type: application/json
```

### 3.3 Request Body

```json
{
  "filters": {
    "tracer_name": "<regex>",
    "hostname": "<regex>",
    "container_hostname": "<regex>",
    "container_host_namespace": "<regex>",
    "region": "<regex>"
  }
}
```

**Filter fields:**

| Field                      | Type   | Required | Description                                        |
|--------------------------|--------|----------|----------------------------------------------------|
| `tracer_name`            | string | No       | Filter by tracer name; supports regular expressions |
| `hostname`               | string | No       | Filter by node hostname; supports regular expressions |
| `container_hostname`     | string | No       | Filter by container hostname; supports regular expressions |
| `container_host_namespace` | string | No     | Filter by container namespace; supports regular expressions |
| `region`                 | string | No       | Filter by region; supports regular expressions     |

- All filter fields are optional; an omitted or empty field matches all values.
- When multiple fields are specified, all conditions must be satisfied simultaneously (**AND semantics**).
- Filtering is applied server-side — only matching events are delivered to the client.

### 3.4 Response Format (SSE Stream)

Once the connection is established the server pushes events continuously in SSE format:

```
data: {"specversion":"1.0","id":"...","source":"/huatuo/node-1/oom",...}\n\n
```

The server also sends periodic keepalive comment lines to maintain the connection:

```
: ping\n
```

---

## 4. EventsWatch Configuration

Configure the `[EventsWatch]` section in `huatuo-bamai.conf`:

```toml
[EventsWatch]
    # Maximum number of concurrent client connections.
    # Requests beyond this limit are rejected with HTTP 429.
    # Default: 100
    MaxClients = 100

    # SSE keepalive ping interval in seconds.
    # Prevents proxies and load balancers from closing idle connections.
    # Three consecutive write failures cause the server to close the connection.
    # Default: 30
    KeepAliveInterval = 30
```

| Option              | Default | Description                                                                    |
|--------------------|---------|--------------------------------------------------------------------------------|
| `MaxClients`       | 100     | Max concurrent `/v1/events/watch` connections; excess requests return HTTP 429 |
| `KeepAliveInterval` | 30     | Keepalive interval in seconds; recommend 15–60 s, not exceeding upstream proxy idle timeout |

---

## 5. curl Examples

### 5.1 Subscribe to all kernel events

```bash
curl -s -N -X POST http://<node-ip>:19704/v1/events/watch \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "Cache-Control: no-cache" \
  -H "Connection: keep-alive" \
  -d '{}'
```

### 5.2 Subscribe to OOM events only

```bash
curl -s -N -X POST http://<node-ip>:19704/v1/events/watch \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "Cache-Control: no-cache" \
  -H "Connection: keep-alive" \
  -d '{"filters": {"tracer_name": "^oom$"}}'
```

### 5.3 Subscribe to network events on a specific node

```bash
curl -s -N -X POST http://<node-ip>:19704/v1/events/watch \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "Cache-Control: no-cache" \
  -H "Connection: keep-alive" \
  -d '{
    "filters": {
      "hostname": "^node-1$",
      "tracer_name": "netdev|dropwatch|net_rx_latency"
    }
  }'
```

### 5.4 Subscribe to container events in the prod namespace

```bash
curl -s -N -X POST http://<node-ip>:19704/v1/events/watch \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -H "Cache-Control: no-cache" \
  -H "Connection: keep-alive" \
  -d '{
    "filters": {
      "container_host_namespace": "^prod$"
    }
  }'
```

> **Note:** The `-N` flag disables curl's output buffering so SSE events appear in the terminal immediately.

---

## 6. Go Client Example

The following example shows how to subscribe to `/v1/events/watch` and consume CloudEvents in a Go program.

```go
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// WatchRequest is the body sent to /v1/events/watch.
type WatchRequest struct {
	Filters WatchFilters `json:"filters"`
}

type WatchFilters struct {
	TracerName             string `json:"tracer_name,omitempty"`
	Hostname               string `json:"hostname,omitempty"`
	ContainerHostname      string `json:"container_hostname,omitempty"`
	ContainerHostNamespace string `json:"container_host_namespace,omitempty"`
	Region                 string `json:"region,omitempty"`
}

// WatchEvent is the CloudEvents 1.0 envelope pushed by HUATUO.
// Mirrors huatuo-bamai/pkg/types.WatchEvent.
type WatchEvent struct {
	SpecVersion     string          `json:"specversion"`
	ID              string          `json:"id"`
	Source          string          `json:"source"`
	Type            string          `json:"type"`
	DataContentType string          `json:"datacontenttype"`
	Time            string          `json:"time"`
	Data            json.RawMessage `json:"data"`
}

func watchEvents(ctx context.Context, endpoint string, filters WatchFilters) error {
	reqBody, err := json.Marshal(WatchRequest{Filters: filters})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 0} // no timeout for SSE long-poll
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip keepalive comment lines and blank lines.
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// SSE data lines have the form: `data: <json>`
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}

		var event WatchEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			log.Printf("parse event: %v", err)
			continue
		}

		fmt.Printf("[%s] source=%s id=%s\n", event.Time, event.Source, event.ID)
		fmt.Printf("  data: %s\n", event.Data)
	}

	return scanner.Err()
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := watchEvents(ctx, "http://192.168.1.10:19704/v1/events/watch", WatchFilters{
		TracerName: "oom|hungtask|softlockup",
	})
	if err != nil {
		log.Fatalf("watch events: %v", err)
	}
}
```

### 6.1 Using the official pkg/types package (recommended)

If your project shares the same Go module as HUATUO, use the official types directly:

```go
import pkgtypes "huatuo-bamai/pkg/types"

var event pkgtypes.WatchEvent
if err := json.Unmarshal([]byte(data), &event); err != nil { ... }

// Unmarshal the data field into WatchEventData for type-safe field access.
dataBytes, _ := json.Marshal(event.Data)
var payload pkgtypes.WatchEventData
if err := json.Unmarshal(dataBytes, &payload); err == nil {
    fmt.Println("tracer:", payload.TracerName)
    fmt.Println("observed_timestamp:", payload.ObservedTimestamp)
}
```

### 6.2 Reconnection with exponential back-off

In production, network disruptions or server restarts can break the connection. Add exponential back-off reconnection:

```go
func watchWithRetry(ctx context.Context, endpoint string, filters WatchFilters) {
	backoff := time.Second
	for {
		if err := watchEvents(ctx, endpoint, filters); err != nil {
			if ctx.Err() != nil {
				return // context cancelled — clean exit
			}
			log.Printf("disconnected: %v, retry in %s", err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}
```
