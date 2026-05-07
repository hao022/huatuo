---
title: Backend Storage
type: docs
description:
author: HUATUO Team
date: 2026-05-05
weight: 1
---

HUATUO supports writing collected Linux kernel events and AutoTracing data to multiple backend storage systems. This document describes how to configure Elasticsearch and OpenSearch as storage backends. The examples are based on Docker images. In a production environment, replace the address with the actual storage service address; the configuration method remains the same.

### OpenSearch Storage

#### 1. Deploy OpenSearch
```bash
$ docker pull opensearchproject/opensearch:2.6.0
$ docker run -d --name opensearch -p 9200:9200 -p 9600:9600 -e "discovery.type=single-node" opensearchproject/opensearch:2.6.0
```

#### 2. Verify the service status
```bash
$ curl -k -u admin:admin https://localhost:9200
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

If the verification fails, inspect the container logs with the following command:

```bash
$ docker logs opensearch
```

#### 3. Configure huatuo-bamai

```bash
[Storage.ES]
    Address = "https://127.0.0.1:9200"
    Index = "huatuo_bamai"
    Username = "admin"
    Password = "admin"
```

The default username and password for the OpenSearch container image are both `admin`. For detailed configuration information, refer to the Configuration Guide section.

#### 4. Start huatuo-bamai

Specify the configuration file directory using the `--config-dir` flag.

```bash
./_output/bin/huatuo-bamai --region dev --config-dir .
```

When files (for example, `net_rx_latency`) appear in the local storage directory `huatuo-local`, kernel events have been captured successfully. Use the following command to query data from OpenSearch:

```bash
$ curl -k -u admin:admin -X GET "https://localhost:9200/huatuo_bamai/_search?pretty" -H 'Content-Type: application/json' -d '{
    "query": {
        "match_all": {}
    }
}'

...
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
        "state" : "<nil>",
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

### ElasticSearch Storage

#### 1. Deploy Elasticsearch
```bash
$ docker pull docker.elastic.co/elasticsearch/elasticsearch:8.15.5
$ docker run -d --name elasticsearch -p 9200:9200 -p 9300:9300 \
        -e "discovery.type=single-node" \
        -e "ES_JAVA_OPTS=-Xms1g -Xmx1g" \
        -e "ELASTIC_PASSWORD=123456" \
        docker.elastic.co/elasticsearch/elasticsearch:8.15.5
```

#### 2. Verify the service status
```bash
$ curl -k -u elastic:123456 https://localhost:9200
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

```bash
[Storage.ES]
    Address = "https://127.0.0.1:9200"
    Index = "huatuo_bamai"
    Username = "elastic"
    Password = "123456"
```

The default username for the Elasticsearch container image is `elastic`. The password is set through the `ELASTIC_PASSWORD` environment variable. For detailed configuration information, refer to the Configuration Guide section.

#### 4. Start huatuo-bamai

Specify the configuration file directory using the `--config-dir` flag.

```bash
./_output/bin/huatuo-bamai --region dev --config-dir .
```

When files (for example, `net_rx_latency`) appear in the local storage directory `huatuo-local`, kernel events have been captured successfully. Use the following command to query data from Elasticsearch:

```bash
$ curl -k -u admin:admin -X GET "https://localhost:9200/huatuo_bamai/_search?pretty" -H 'Content-Type: application/json' -d '{
    "query": {
        "match_all": {}
    }
}'

...
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
        "state" : "<nil>",
        "saddr" : "127.0.0.1",
        "daddr" : "127.0.0.1",
        "sport" : 2379,
        "dport" : 36706,
        "seq" : 950542706,
        "ack_seq" : 1960972383,
        "pkt_len" : 91
      }
}
```
