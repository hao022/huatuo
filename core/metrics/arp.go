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

package collector

import (
	"fmt"
	"strconv"

	"huatuo-bamai/internal/pod"
	"huatuo-bamai/internal/procfs"
	"huatuo-bamai/pkg/metric"
	"huatuo-bamai/pkg/tracing"
)

type arpCollector struct {
	metric []*metric.Data
}

func init() {
	tracing.RegisterEventTracing("arp", newArp)
}

func newArp() (*tracing.EventTracingAttr, error) {
	return &tracing.EventTracingAttr{
		TracingData: &arpCollector{
			metric: []*metric.Data{
				metric.NewGaugeData("entries", 0, "host init namespace", nil),
				metric.NewGaugeData("total", 0, "arp_cache entries", nil),
			},
		},
		Flag: tracing.FlagMetric,
	}, nil
}

func (c *arpCollector) updateHostArp() ([]*metric.Data, error) {
	count, err := fileLineCounter(procfs.Path("1/net/arp"))
	if err != nil {
		return nil, err
	}

	cache, err := procfs.NetArpCache()
	if err != nil {
		return nil, err
	}

	c.metric[0].Value = float64(count - 1)
	c.metric[1].Value = float64(cache.Stats["entries"])

	return c.metric, err
}

func (c *arpCollector) Update() ([]*metric.Data, error) {
	data := []*metric.Data{}

	containers, err := pod.NormalContainers()
	if err != nil {
		return nil, fmt.Errorf("GetNormalContainers: %w", err)
	}

	for _, container := range containers {
		count, err := fileLineCounter(procfs.Path(strconv.Itoa(container.InitPid), "net/arp"))
		if err != nil {
			return nil, err
		}

		data = append(data, metric.NewContainerGaugeData(container, "entries", float64(count-1), "arp for container and host", nil))
	}

	hostMetrics, err := c.updateHostArp()
	if err != nil {
		return nil, err
	}

	return append(data, hostMetrics...), nil
}
