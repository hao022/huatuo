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

package linkstatus

import "golang.org/x/sys/unix"

type Types uint8

const (
	Unknown Types = iota
	AdminUp
	AdminDown
	CarrierUp
	CarrierDown
	MaxTypeNums
)

func (link Types) String() string {
	return [...]string{"linkstatus_unknown", "linkstatus_adminup", "linkstatus_admindown", "linkstatus_carrierup", "linkstatus_carrierdown"}[link]
}

func Changed(flags, change uint32) []Types {
	var status []Types

	if change&unix.IFF_UP != 0 {
		if flags&unix.IFF_UP != 0 {
			status = append(status, AdminUp)
		} else {
			status = append(status, AdminDown)
		}
	}

	if change&unix.IFF_LOWER_UP != 0 {
		if flags&unix.IFF_LOWER_UP != 0 {
			status = append(status, CarrierUp)
		} else {
			status = append(status, CarrierDown)
		}
	}

	return status
}

func FlagsRaw(flags uint32) []string {
	var (
		status []Types
		strs   []string
	)

	// invalid value
	if flags == 0 {
		return []string{"unknown"}
	}

	// now we take care of IFF_UP, IFF_LOWER_UP
	if flags&unix.IFF_UP != 0 {
		status = append(status, AdminUp)
	} else {
		status = append(status, AdminDown)
	}

	if flags&unix.IFF_LOWER_UP != 0 {
		status = append(status, CarrierUp)
	} else {
		status = append(status, CarrierDown)
	}

	for _, s := range status {
		strs = append(strs, s.String())
	}

	return strs
}
