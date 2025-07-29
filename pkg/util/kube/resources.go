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
	return resource.NewQuantity(b, resource.BinarySI)
}
