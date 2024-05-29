// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ptr

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

// Overwrite copies src to dst as long as src is non-nil.
func Overwrite[T any](dst **T, src *T) bool {
	if dst == nil {
		return false
	}
	if src != nil {
		*dst = src
		return true
	}
	return false
}
