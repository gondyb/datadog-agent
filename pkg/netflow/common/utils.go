// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package common

import "net"

// MinUint64 returns the min of the two passed number
func MinUint64(a uint64, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// MaxUint64 returns the max of the two passed number
func MaxUint64(a uint64, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// Uint32ToBytes converts uint32 to []byte
func Uint32ToBytes(val uint32) []byte {
	b := make([]byte, 4)
	for i := 0; i < 4; i++ {
		b[i] = byte(val >> (8 * i))
	}
	return b
}

// IPBytesToString convert IP in []byte to string
func IPBytesToString(ip []byte) string {
	if len(ip) == 0 {
		return ""
	}
	return net.IP(ip).String()
}
