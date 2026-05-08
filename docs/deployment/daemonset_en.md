---
title: Kubernetes Daemonset
type: docs
description: 
author: HUATUO Team
date: 2026-01-11
weight: 2
---

This document describes how to deploy the Huatuo collector to a cloud-native cluster using a Kubernetes DaemonSet.

### 1. Download the configuration file

```bash
$ curl -L -o huatuo-bamai.conf https://github.com/ccfos/huatuo/raw/main/huatuo-bamai.conf
```

### 2. Modify the configuration file

Modify the configuration file according to your actual deployment environment. For example, adjust settings such as the storage backend and the method for obtaining Pod information. For details, see the *Configuration Guide*.

### 3. Create a ConfigMap

```bash
$ kubectl delete configmap huatuo-bamai-config
$ kubectl create configmap huatuo-bamai-config --from-file=./huatuo-bamai.conf
```

### 3. Deploy the Collector

```bash
$ kubectl apply -f https://github.com/ccfos/huatuo/blob/main/build/huatuo-daemonset.minimal.yaml
```

**Notes:**

- In `huatuo-daemonset.minimal.yaml`, the container image uses the `huatuo-bamai:latest` tag by default. For production deployments, replace it with a specific release version image.
- When using `huatuo-bamai:latest` for testing, verify that the tag points to the latest image. You can remove the old image and pull it again by running `docker image rm huatuo/huatuo-bamai:latest`.
