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

package common

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// fsEntry stores the FS instance for one rootFS,
// ensure initialization of every fs runs only once.
type fsEntry struct {
	once sync.Once
	fs   FS
	err  error
}

// map[string]*fsEntry, caches FS by rootFS.
var fsCache sync.Map

// DefaultFS returns a cached FS for the given rootFS.
// NewFS is called at most once per rootFS.
func DefaultFS(rootFS string) (FS, error) {
	cached, _ := fsCache.LoadOrStore(rootFS, &fsEntry{})
	entry := cached.(*fsEntry)

	entry.once.Do(func() {
		entry.fs, entry.err = NewFS(rootFS)
	})

	return entry.fs, entry.err
}

// fork from "github.com/prometheus/procfs/internal" FS struct and its func
// interface to kernel data structures.
type FS string

// NewFS returns a new FS mounted under the given mountPoint. It will error
// if the mount point can't be read.
func NewFS(mountPoint string) (FS, error) {
	info, err := os.Stat(mountPoint)
	if err != nil {
		return "", fmt.Errorf("could not read %q: %w", mountPoint, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("mount point %q is not a directory", mountPoint)
	}

	return FS(mountPoint), nil
}

// Path appends the given path elements to the filesystem path, adding separators
// as necessary.
func (fs FS) Path(p ...string) string {
	return filepath.Join(append([]string{string(fs)}, p...)...)
}
