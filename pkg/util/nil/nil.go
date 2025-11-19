// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package nil provides a utility function for determining if a value is nil.
//
//nolint:predeclared
package nil

import (
	"reflect"
)

// IsNil returns true if the provided argument is nil.
func IsNil(arg any) bool {
	v := reflect.ValueOf(arg)
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Ptr,
		reflect.Interface,
		reflect.Slice,
		reflect.Map,
		reflect.Chan,
		reflect.Func:
		return v.IsNil()
	}
	return false
}
