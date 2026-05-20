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

package pod

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	dockertypes "github.com/docker/docker/api/types"

	"huatuo-bamai/internal/pidfile"
)

func containerInitPid(containerID string) (int, error) {
	switch currContainerProvider {
	case containerProviderDocker:
		return containerInitPidInDocker(containerID)
	case containerProviderContainerd:
		return containerInitPidInContainerd(containerID)
	default:
		return -1, fmt.Errorf("container provider not initialized")
	}
}

func containerInitPidInDocker(containerID string) (int, error) {
	configPath := filepath.Join(dockerRootDir, "containers", containerID, "config.v2.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return -1, err
	}

	container := dockertypes.ContainerJSON{}
	if err := json.Unmarshal(content, &container); err != nil {
		return -1, err
	}

	if container.State.Pid == 0 {
		return -1, fmt.Errorf("invalid pid for container %s", containerID)
	}
	return container.State.Pid, nil
}

func containerInitPidInContainerd(containerID string) (int, error) {
	// pid: $state/io.containerd.runtime.v2.task/k8s.io/$container/init.pid
	// runtime runc v2?
	// kata ?
	filePath := filepath.Join(containerdStateDir, "io.containerd.runtime.v2.task", "k8s.io", containerID, "init.pid")

	return pidfile.Read(filePath)
}
