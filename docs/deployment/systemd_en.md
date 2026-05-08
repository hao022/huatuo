---
title: Systemd Bare-Metal
type: docs
description: 
author: HUATUO Team, HAO022
date: 2026-01-11
weight: 3
---

The RPM release of HUATUO is available from the OpenCloudOS repository. Only version 2.1.0 is currently supported.

### 1. Download the RPM package

The OpenCloudOS mirror provides the HUATUO RPM package. Download the appropriate package for your architecture:

```bash
wget https://mirrors.opencloudos.tech/epol/9/Everything/x86_64/os/Packages/huatuo-bamai-2.1.0-2.oc9.x86_64.rpm  
wget https://mirrors.opencloudos.tech/epol/9/Everything/aarch64/os/Packages/huatuo-bamai-2.1.0-2.oc9.aarch64.rpm
```

### 2. Install the RPM package

```bash
sudo rpm -ivh huatuo-bamai*.rpm
```

### 3. Modify the configuration

Edit the configuration file `/etc/huatuo-bamai/huatuo-bamai.conf` to match your deployment environment. For detailed configuration options, refer to the *Configuration Guide*.

### 4. Start the HUATUO service

```bash
sudo systemctl start huatuo-bamai
sudo systemctl enable huatuo-bamai
```

> For complete installation instructions, see [https://mp.weixin.qq.com/s/Gmst4_FsbXUIhuJw1BXNnQ](https://mp.weixin.qq.com/s/Gmst4_FsbXUIhuJw1BXNnQ)
