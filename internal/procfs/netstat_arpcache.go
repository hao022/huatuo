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

package procfs

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// arpCacheStats contains statistics for all the counters from `/proc/net/stat/arp_cache`
type arpCacheStats struct {
	Stats map[string]uint64
}

// ARPCacheStats retrieves stats from `/proc/net/stat/arp_cache`,
// Not available in upstream procfs.
func ARPCacheStats() (arpCacheStats, error) {
	netStat := arpCacheStats{
		Stats: make(map[string]uint64),
	}

	path, err := DefaultPath("net/stat/arp_cache")
	if err != nil {
		return netStat, err
	}

	file, err := os.Open(path)
	if err != nil {
		return netStat, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()

	// First string is always a header for stats
	var headers []string
	headers = append(headers, strings.Fields(scanner.Text())...)

	// Fast path ...
	scanner.Scan()
	for num, counter := range strings.Fields(scanner.Text()) {
		value, err := strconv.ParseUint(counter, 16, 64)
		if err != nil {
			return arpCacheStats{}, err
		}
		netStat.Stats[headers[num]] = value
	}

	return netStat, nil
}
