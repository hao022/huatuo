// Copyright 2025, 2026 The HuaTuo Authors
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
	"bytes"
	"fmt"
	"os"
	"strconv"
	"syscall"
)

var defaultDirPath = "/var/run"

func path(name string) string {
	return fmt.Sprintf("%s/%s.pid", defaultDirPath, name)
}

// Lock pid with file
func Lock(name string) error {
	name = path(name)

	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
			pid, err := os.ReadFile(name)
			if err != nil {
				return fmt.Errorf("running path: %s", name)
			}

			return fmt.Errorf("running path: %s, pid %s", name, pid)
		}

		return err
	}

	_, err = f.WriteString(strconv.Itoa(os.Getpid()))
	return err
}

// UnLock the pidfile
// If a return value is needed in the future, we will support it.
// The current implementation is simpler.
func UnLock(name string) {
	name = path(name)

	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o666)
	if err != nil {
		return
	}
	defer f.Close()

	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = os.Remove(name)
}

// Read reads the "PID file" at path, and returns the PID if it contains a
// valid PID of a running process, or 0 otherwise. It returns an error when
// failing to read the file, or if the file doesn't exist, but malformed content
// is ignored. Consumers should therefore check if the returned PID is a non-zero
// value before use.
func Read(path string) (int, error) {
	pidByte, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(string(bytes.TrimSpace(pidByte)))
}
