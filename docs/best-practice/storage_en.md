---
title: Storage
type: docs
description: ""
author: HUATUO Team
date: 2026-05-05
weight: 1
---

{{% alert color="info" title="🎯 About HUATUO" %}}
<div style="text-align: center;">
HUATUO is an open-source OS-level deep observability project initiated by DiDi and incubated by the CCF (China Computer Federation). It provides kernel-level observability for cloud-native computing, AI computing, cloud services, and foundational infrastructure.
</div>
{{% /alert %}}

## 📖 Overview

HUATUO supports persisting Linux kernel events collected by the Tracer and AutoTracing data to external storage backends. Both Elasticsearch and OpenSearch are supported.

After serialization to JSON, collected events are written concurrently to the local node directory (`huatuo-local/`) and the configured remote storage backend. The local directory retains a local copy of events; the remote backend provides durable storage and structured query capabilities.

This document covers configuration and verification for both Elasticsearch and OpenSearch. Examples use Docker deployments. In production, replace the addresses with your actual service endpoints — the configuration format is the same.

---

## 🎯 Use Cases

### Kubernetes Cloud-Native Fault Tracing

In containerized environments, kernel events such as Pod OOM and node Hung Task are transient — logs are often purged shortly after the event occurs. By writing events to Elasticsearch or OpenSearch, operations teams can query the historical timeline of anomalies by time range and precisely identify the root cause of intermittent failures during post-incident reviews.

### AI Compute Cluster Stability Auditing

During long-running GPU training workloads, the historical distribution of events such as `ras` hardware errors and `iotracing` I/O latency is critical for capacity planning and hardware health assessment. Persisting collected data enables aggregate queries to establish node stability baselines and supports proactive maintenance decisions.

### Compliance and Event Retention

Security compliance standards require that system anomaly events be traceable. Writing HUATUO-captured kernel events to OpenSearch and configuring an index lifecycle policy satisfies compliance requirements for event retention periods and query capabilities.

### Observability Platform Integration

Both Elasticsearch and OpenSearch provide native data source integrations with Grafana. Once HUATUO events are written to storage, you can build kernel event trend dashboards in Grafana, overlaid with application-layer metrics for historical analysis and alert review.

---

## 💎 Value

| Dimension | Local Storage Only | With External Storage Backend |
|---|---|---|
| Data Durability | Limited by node disk capacity; may be lost on restart | Persisted to distributed storage; supports long-term retention |
| Query Capability | No structured queries; relies on file search | Full-text search, field filtering, time-range aggregation |
| Visualization | Not supported | Direct integration with Grafana, Kibana, and similar platforms |
| Multi-node Aggregation | Data scattered across individual nodes | Centralized storage; supports cross-node queries |
| Compliance Retention | Difficult to meet retention requirements | Configurable index lifecycle policies; meets compliance retention requirements |

---

## 🚀 Usage

### OpenSearch Storage

#### 1. Deploy OpenSearch

```bash
docker pull opensearchproject/opensearch:2.6.0
docker run -d --name opensearch -p 9200:9200 -p 9600:9600 \
  -e "discovery.type=single-node" \
  opensearchproject/opensearch:2.6.0
```

#### 2. Verify Service Status

```bash
curl -k -u admin:admin https://localhost:9200
```

Example response:

```json
{
  "name" : "22ca72df78c0",
  "cluster_name" : "docker-cluster",
  "cluster_uuid" : "yxb3foceQVKzXXO6bHpPHQ",
  "version" : {
    "distribution" : "opensearch",
    "number" : "2.6.0",
    "build_type" : "tar",
    "build_hash" : "7203a5af21a8a009aece1474446b437a3c674db6",
    "build_date" : "2023-02-24T18:57:04.388618985Z",
    "build_snapshot" : false,
    "lucene_version" : "9.5.0",
    "minimum_wire_compatibility_version" : "7.10.0",
    "minimum_index_compatibility_version" : "7.0.0"
  },
  "tagline" : "The OpenSearch Project: https://opensearch.org/"
}
```

If verification fails, check the container logs:

```bash
docker logs opensearch
```

#### 3. Configure huatuo-bamai

Add the following configuration to `huatuo-bamai.conf`. The default username and password for the OpenSearch container image are both `admin`. For a full description of storage configuration options, refer to the Configuration Guide.

```toml
[Storage.ES]
    Address = "https://127.0.0.1:9200"
    Index = "huatuo_bamai"
    Username = "admin"
    Password = "admin"
```

#### 4. Start huatuo-bamai

Use `--config-dir` to specify the directory containing the configuration file:

```bash
./_output/bin/huatuo-bamai --region dev --config-dir .
```

When files (e.g., `net_rx_latency`) appear in the local storage directory `huatuo-local/`, kernel events have been successfully captured. Query data from OpenSearch with:

```bash
curl -k -u admin:admin \
  -X GET "https://localhost:9200/huatuo_bamai/_search?pretty" \
  -H "Content-Type: application/json" \
  -d '{"query": {"match_all": {}}}'
```

Example response:

```json
{
    "_index" : "huatuo_bamai",
    "_id" : "yjPG_50Bu_OF-hukxKR7",
    "_score" : 1.0,
    "_source" : {
      "hostname" : "hostname",
      "region" : "dev",
      "uploaded_time" : "2026-05-07T00:11:49.753166222Z",
      "time" : "2026-05-07 00:11:49.753 +0000",
      "tracer_name" : "net_rx_latency",
      "tracer_time" : "2026-05-07 00:11:49.753 +0000",
      "tracer_type" : "auto",
      "tracer_data" : {
        "comm" : "<nil>",
        "pid" : 0,
        "where" : "TO_NETIF_RCV",
        "latency_ms" : 1776078133565,
        "saddr" : "127.0.0.1",
        "daddr" : "127.0.0.1",
        "sport" : 37736,
        "dport" : 9200,
        "seq" : 1080592402,
        "ack_seq" : 2465063876,
        "pkt_len" : 781
      }
    }
}
```

---

### Elasticsearch Storage

#### 1. Deploy Elasticsearch

```bash
docker pull docker.elastic.co/elasticsearch/elasticsearch:8.15.5
docker run -d --name elasticsearch -p 9200:9200 -p 9300:9300 \
  -e "discovery.type=single-node" \
  -e "ES_JAVA_OPTS=-Xms1g -Xmx1g" \
  -e "ELASTIC_PASSWORD=123456" \
  docker.elastic.co/elasticsearch/elasticsearch:8.15.5
```

#### 2. Verify Service Status

```bash
curl -k -u elastic:123456 https://localhost:9200
```

Example response:

```json
{
  "name" : "ab0b562f8dbd",
  "cluster_name" : "docker-cluster",
  "cluster_uuid" : "aVfOVgJTQXuhZ3HGotK3ww",
  "version" : {
    "number" : "8.15.5",
    "build_flavor" : "default",
    "build_type" : "docker",
    "build_hash" : "b10896bcfe167cce44a84ba2771d101fb596d40d",
    "build_date" : "2024-11-21T22:06:13.985834967Z",
    "build_snapshot" : false,
    "lucene_version" : "9.11.1",
    "minimum_wire_compatibility_version" : "7.17.0",
    "minimum_index_compatibility_version" : "7.0.0"
  },
  "tagline" : "You Know, for Search"
}
```

#### 3. Configure huatuo-bamai

Add the following configuration to `huatuo-bamai.conf`. The default username for the Elasticsearch container image is `elastic`; the password is set via the `ELASTIC_PASSWORD` environment variable. For a full description of storage configuration options, refer to the Configuration Guide.

```toml
[Storage.ES]
    Address = "https://127.0.0.1:9200"
    Index = "huatuo_bamai"
    Username = "elastic"
    Password = "123456"
```

#### 4. Start huatuo-bamai

Use `--config-dir` to specify the directory containing the configuration file:

```bash
./_output/bin/huatuo-bamai --region dev --config-dir .
```

When files (e.g., `net_rx_latency`) appear in the local storage directory `huatuo-local/`, kernel events have been successfully captured. Query data from Elasticsearch with:

```bash
curl -k -u elastic:123456 \
  -X GET "https://localhost:9200/huatuo_bamai/_search?pretty" \
  -H "Content-Type: application/json" \
  -d '{"query": {"match_all": {}}}'
```

Example response:

```json
{
    "_index" : "huatuo_bamai",
    "_id" : "WtNZAJ4BQ8x-thPHEY1i",
    "_score" : 1.0,
    "_source" : {
      "hostname" : "hostname",
      "region" : "dev",
      "uploaded_time" : "2026-05-07T02:51:37.696263325Z",
      "time" : "2026-05-07 02:51:37.696 +0000",
      "tracer_name" : "net_rx_latency",
      "tracer_time" : "2026-05-07 02:51:37.696 +0000",
      "tracer_type" : "auto",
      "tracer_data" : {
        "comm" : "<nil>",
        "pid" : 0,
        "where" : "TO_NETIF_RCV",
        "latency_ms" : 1776078133565,
        "saddr" : "127.0.0.1",
        "daddr" : "127.0.0.1",
        "sport" : 2379,
        "dport" : 36706,
        "seq" : 950542706,
        "ack_seq" : 1960972383,
        "pkt_len" : 91
      }
    }
}
```

---

## ⚙️ How It Works

### System Architecture

The HUATUO Storage module runs on each node. It writes kernel events captured by the Tracer concurrently to the local directory and to Elasticsearch or OpenSearch. Both storage backends share the same `[Storage.ES]` configuration interface and are differentiated by address.

```mermaid
graph TB
    subgraph kernel["Linux Kernel"]
        K1[Kernel Events]
        K2[AutoTracing]
    end

    subgraph huatuo["HUATUO Agent (node-level)"]
        T["Tracer Layer"]
        L["Local Directory\nhuatuo-local/"]
        S["Storage Module\n(concurrent write)"]
    end

    subgraph backends["Storage Backends"]
        ES[Elasticsearch]
        OS[OpenSearch]
    end

    kernel --> T
    T --> L
    T --> S
    S -->|Index API| ES
    S -->|Index API| OS
```

### Write Flow

After the Tracer captures a kernel event, the Storage module writes it concurrently to the local directory and the remote storage backend. The two write paths execute in parallel — the local directory retains a copy while the remote backend provides durable storage and query capabilities.

```mermaid
sequenceDiagram
    participant T as Tracer Layer
    participant L as Local Directory (huatuo-local/)
    participant S as Storage Module
    participant B as ES / OpenSearch

    T->>S: Kernel event captured, serialized to JSON
    par concurrent write
        S->>L: Write to local file
    and
        S->>B: Write to remote storage (Index API)
        B-->>S: Write acknowledged (200 OK)
    end
```

### Storage Pipeline

From kernel event to storage backend, the process involves three stages: capture, serialization, and concurrent write. The local directory and remote backend are written to in parallel without blocking each other.

```mermaid
flowchart LR
    A([Kernel Event]) --> B["Tracer Capture\nSerialize to JSON"]
    B --> C["Storage Module\n(concurrent write)"]
    C --> D["Write to Local Directory\nhuatuo-local/"]
    C --> E["Write to ES / OpenSearch\nIndex API"]
```

---

## 🌟 Stay Connected

{{% alert color="info" %}}
<div style="text-align: center;">
🌟 Star us on GitHub: <a href="https://github.com/ccfos/huatuo" target="_blank">https://github.com/ccfos/huatuo</a>
<br><br>
👀 Follow our official WeChat public account<br>
<img src="/img/contact-weixin.png" alt="WeChat QR code" style="max-width: 200px; margin-top: 10px;">
</div>
{{% /alert %}}
