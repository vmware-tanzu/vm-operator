// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"math"

	"k8s.io/apimachinery/pkg/api/resource"
)

func MemoryQuantityToMb(q resource.Quantity) int64 {
	return int64(math.Ceil(float64(q.Value()) / float64(1024*1024)))
}

// MbToBytes converts a vSphere MB-denominated value (always binary mebibytes)
// into bytes, the inverse of MemoryQuantityToMb.
func MbToBytes(mb int64) int64 {
	return mb * 1024 * 1024
}

func CPUQuantityToMhz(q resource.Quantity, cpuFreqMhz uint64) int64 {
	return int64(math.Ceil(float64(q.MilliValue()) * float64(cpuFreqMhz) / float64(1000)))
}
