// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

// BytesToResource returns the resource.Quantity value for the specified number
// of bytes.
func BytesToResource(b int64) *resource.Quantity {
	if isBinarySI(b, b%1024) {
		return resource.NewQuantity(b, resource.BinarySI)
	}
	return resource.NewQuantity(b, resource.DecimalSI)
}

func isBinarySI(b int64, r int64) bool {
	switch {
	case r > 0:
		return false
	case b == 1024:
		return true
	case b < 1024:
		return r == 0
	default:
		return isBinarySI(b/1024, b%1024)
	}
}
