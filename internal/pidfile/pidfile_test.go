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

package pidfile

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPath(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"app", "/var/run/app.pid"},
		{"nginx", "/var/run/nginx.pid"},
		{"", "/var/run/.pid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, path(tt.name))
		})
	}
}

func TestLock_Success(t *testing.T) {
	tmpDir := t.TempDir()
	oldDefault := defaultDirPath
	defaultDirPath = tmpDir
	t.Cleanup(func() { defaultDirPath = oldDefault })

	name := "testapp"
	pidPath := path(name)

	err := Lock(name)
	require.NoError(t, err)
	t.Cleanup(func() { UnLock(name) })

	_, err = os.Stat(pidPath)
	require.NoError(t, err)

	data, err := os.ReadFile(pidPath)
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	assert.Equal(t, os.Getpid(), pid)
}

func TestLock_AlreadyLocked(t *testing.T) {
	tmpDir := t.TempDir()
	oldDefault := defaultDirPath
	defaultDirPath = tmpDir
	t.Cleanup(func() { defaultDirPath = oldDefault })

	name := "test-locked"

	err := Lock(name)
	require.NoError(t, err)
	t.Cleanup(func() { UnLock(name) })

	err = Lock(name)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "running")
}

func TestUnLock(t *testing.T) {
	tmpDir := t.TempDir()
	oldDefault := defaultDirPath
	defaultDirPath = tmpDir
	t.Cleanup(func() { defaultDirPath = oldDefault })

	name := "toremove"
	pidPath := path(name)

	err := os.WriteFile(pidPath, []byte("12345"), 0o600)
	require.NoError(t, err)

	UnLock(name)

	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err))
}

func TestRead(t *testing.T) {
	tmp := t.TempDir()
	pidPath := filepath.Join(tmp, "test.pid")

	tests := []struct {
		name        string
		content     string
		wantPID     int
		wantErrKind bool // 是否期望有错误
	}{
		{"normal", "12345", 12345, false},
		{"with space", "  67890  \n", 67890, false},
		{"negative", "-1", -1, false},
		{"empty", "", 0, true},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(pidPath, []byte(tt.content), 0o600)
			require.NoError(t, err)

			got, err := Read(pidPath)
			if tt.wantErrKind {
				assert.Error(t, err)
				assert.Zero(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPID, got)
			}
		})
	}
}

func TestRead_NotExist(t *testing.T) {
	_, err := Read("/this/file/does/not/exist.pid")
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}
