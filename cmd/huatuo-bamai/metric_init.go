// Copyright 2025 The HuaTuo Authors
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

package main

import (
	"huatuo-bamai/pkg/metric"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var promNamespace = "huatuo_bamai"

// InitMetricsCollector creates a new MetricsCollector instance.
func InitMetricsCollector(blackListed []string, region string) (*prometheus.Registry, error) {
	nc, err := metric.NewCollectorManager(blackListed, region)
	if err != nil {
		return nil, err
	}

	promRegistry := prometheus.NewRegistry()
	promRegistry.MustRegister(
		nc,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(
			collectors.ProcessCollectorOpts{Namespace: promNamespace}))

	return promRegistry, nil
}
