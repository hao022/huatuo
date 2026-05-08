---
title: DaemonSet 云原生集群部署
type: docs
description: 
author: HUATUO Team, hao022
date: 2026-01-11
weight: 2
---

本文介绍如何通过 Kubernetes DaemonSet 将华佗采集器部署到云原生集群。

### 1. 获取配置文件

```bash
$ curl -L -o huatuo-bamai.conf https://github.com/ccfos/huatuo/raw/main/huatuo-bamai.conf
```

### 2. 修改配置文件

根据实际部署环境修改配置文件，例如调整存储后端、Pod 信息获取方式等配置项，详见《配置指南》。

### 3. 创建 ConfigMap
```bash
$ kubectl delete configmap huatuo-bamai-config
$ kubectl create configmap huatuo-bamai-config --from-file=./huatuo-bamai.conf
```


### 4. 部署采集器
```bash
$ kubectl apply -f https://github.com/ccfos/huatuo/blob/main/build/huatuo-daemonset.minimal.yaml
```

注意事项：
- huatuo-daemonset.minimal.yaml 中容器镜像默认使用 huatuo-bamai:latest 标签。若需用于生产环境，请将其替换为指定的发行版本镜像。
- 若使用 huatuo-bamai:latest 进行测试，请确保该标签指向最新镜像（可通过 docker image rm huatuo/huatuo-bamai:latest 删除旧镜像后重新拉取）。
