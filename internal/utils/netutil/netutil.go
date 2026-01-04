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

package netutil

import (
	"encoding/binary"
	"net"

	"github.com/vishvananda/netlink/nl"
)

var NativeEndian = nl.NativeEndian()

// inet_ntop addr is big-endian
// convert IPv4 addresses (in network byte order) from binary to text
//
// https://man7.org/linux/man-pages/man3/inet_ntop.3.html
func Inetv4Ntop(addr uint32) net.IP {
	return net.IPv4(
		byte(addr>>24),
		byte(addr>>16),
		byte(addr>>8),
		byte(addr),
	).To4()
}

// InetNtohs is same as the ntohs
func InetNtohs(val uint16) uint16 {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, val)
	return NativeEndian.Uint16(buf)
}

// InetNtohl is same as the ntohl
func InetNtohl(val uint32) uint32 {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, val)
	return NativeEndian.Uint32(buf)
}

// InetHtons is same as the htons
func InetHtons(val uint16) uint16 {
	buf := make([]byte, 2)
	NativeEndian.PutUint16(buf, val)
	return binary.BigEndian.Uint16(buf)
}

// InetHtonl is same as the htonl
func InetHtonl(val uint32) uint32 {
	buf := make([]byte, 4)
	NativeEndian.PutUint32(buf, val)
	return binary.BigEndian.Uint32(buf)
}
