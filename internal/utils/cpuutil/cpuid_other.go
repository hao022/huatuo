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

//go:build !amd64

package cpuutil

// Cpuid is a no-op stub on non-amd64 platforms.
func Cpuid(arg1, arg2 uint32) (eax, ebx, ecx, edx uint32) {
	return 0, 0, 0, 0
}

var CPUFn = Cpuid
