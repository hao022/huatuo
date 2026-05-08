---
title: Systemd 物理机部署
type: docs
description: 
author: HUATUO Team, HAO022
date: 2026-01-11
weight: 3
---

HUATUO（华佗）的 RPM 发行版可通过 OpenCloudOS 镜像仓库获取，当前仅支持 v2.1.0 版本。

### 1. 下载 RPM 包

OpenCloudOS 镜像站提供了 HUATUO 的 RPM 安装包，可按需选择对应架构下载：

```bash
wget https://mirrors.opencloudos.tech/epol/9/Everything/x86_64/os/Packages/huatuo-bamai-2.1.0-2.oc9.x86_64.rpm  
wget https://mirrors.opencloudos.tech/epol/9/Everything/aarch64/os/Packages/huatuo-bamai-2.1.0-2.oc9.aarch64.rpm
```

### 2. 安装 RPM 包

```bash
sudo rpm -ivh huatuo-bamai*.rpm
```

### 3. 修改配置

根据实际部署环境编辑配置文件 /etc/huatuo-bamai/huatuo-bamai.conf，详细配置项说明请参见《配置指南》。

### 4. 启动 HUATUO 服务

```bash
sudo systemctl start huatuo-bamai
sudo systemctl enable huatuo-bamai
```

> 完整的安装指引请参阅 [https://mp.weixin.qq.com/s/Gmst4_FsbXUIhuJw1BXNnQ](https://mp.weixin.qq.com/s/Gmst4_FsbXUIhuJw1BXNnQ)
