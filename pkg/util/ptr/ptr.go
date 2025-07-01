// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package ptr

import (
	"reflect"
)

// To returns a pointer to t.
func To[T any](t T) *T {
	return &t
}

// Deref returns the value referenced by t if not nil, otherwise the empty value
// for T is returned.
func Deref[T any](t *T) T {
	var empT T
	return DerefWithDefault(t, empT)
}

// DerefWithDefault returns the value referenced by t if not nil, otherwise
// defaulT is returned.
func DerefWithDefault[T any](t *T, defaulT T) T {
	if t != nil {
		return *t
	}
	return defaulT
}

// Equal returns true if both arguments are nil or both arguments
// dereference to the same value.
func Equal[T comparable](a, b *T) bool {
	if (a == nil) != (b == nil) {
		// One of a or b is nil and the other is not.
		return false
	}

	if a == nil {
		// If a is nil, then we know b is nil as well.
		return true
	}

	return *a == *b
}

// Overwrite copies src to dst if:
// - src is a non-nil channel, function, interface, map, pointer, or slice
// - src is any other type of value
// Please note, this function panics if dst is nil.
func Overwrite[T any](dst *T, src T) {
	if dst == nil {
		panic("dst is nil")
	}

	valueOfSrc := reflect.ValueOf(src)
	switch valueOfSrc.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice:

		if !valueOfSrc.IsNil() {
			*dst = src
		}
	default:
		*dst = src
	}
}

// OverwriteWithUser overwrites the *dst with an optional user value depending on
// if the value needs to be updated based on the current value.
func OverwriteWithUser[T comparable](dst **T, user, current *T) {
	if dst == nil {
		panic("dst is nil")
	}

	// Determine what the ultimate desired value is. If set the user
	// value takes precedence.
	var desired *T
	switch {
	case user != nil:
		desired = user
	case *dst != nil:
		desired = *dst
	default:
		// Leave *dst as-is.
		return
	}

	if current == nil || *current != *desired {
		// An update is required to the desired value.
		*dst = desired
	} else if *current == *desired {
		// Already at the desired value so no update is required.
		*dst = nil
	}
}
