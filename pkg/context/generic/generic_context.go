// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package generic

import (
	"context"
	"sync"
)

type contextData[T any] struct {
	sync.Mutex
	data T
}

// SetContext sets the data in the context.
func SetContext[T any, K comparable](
	ctx context.Context,
	key K,
	setT func(curT T) T) {

	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(key)
	if obj == nil {
		panic("value is missing from context")
	}

	m := obj.(*contextData[T])
	m.Lock()
	defer m.Unlock()

	m.data = setT(m.data)
}

// FromContext returns the data from the context.
func FromContext[T any, K comparable, V any](
	ctx context.Context,
	key K,
	getV func(val T) V) V {

	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(key)
	if obj == nil {
		panic("value is missing from context")
	}

	m := obj.(*contextData[T])
	m.Lock()
	defer m.Unlock()

	return getV(m.data)
}

// WithContext returns a new context with the data in the specified key.
func WithContext[T any, K comparable](
	parent context.Context,
	key K,
	newT func() T) context.Context {

	if parent == nil {
		panic("parent context is nil")
	}

	return context.WithValue(
		parent,
		key,
		&contextData[T]{
			data: newT(),
		},
	)
}

// NewContext returns a new context with the data in the specified key.
func NewContext[T any, K comparable](key K, newT func() T) context.Context {
	return WithContext(context.Background(), key, newT)
}

// ValidateContext returns true if the provided context contains the data at the
// specified key.
func ValidateContext[T any, K comparable](ctx context.Context, key K) bool {
	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(key)
	if obj == nil {
		return false
	}
	_, ok := obj.(*contextData[T])
	return ok
}

// JoinContext returns a new context that contains a reference the data at
// the specified key.
// This function panics if the provided context does not contain the data.
// This function is thread-safe.
func JoinContext[T any, K comparable](
	left, right context.Context,
	key K,
	copyT func(dstT, srcT T) T) context.Context {

	if left == nil {
		panic("left context is nil")
	}
	if right == nil {
		panic("right context is nil")
	}

	leftObj := left.Value(key)
	rightObj := right.Value(key)

	if leftObj == nil && rightObj == nil {
		panic("value is missing from context")
	}

	if leftObj != nil && rightObj == nil {
		// The left context has the value and the right does not, so just return
		// the left context.
		return left
	}

	if leftObj == nil && rightObj != nil {
		// The right context has the value and the left does not, so return a
		// new context with the value in it.
		return context.WithValue(left, key, rightObj.(*contextData[T]))
	}

	// Both contexts have the value, so the left context is returned after the
	// value from the right context is used to overwrite the value from the
	// left.

	leftVal := leftObj.(*contextData[T])
	rightVal := rightObj.(*contextData[T])

	leftVal.Lock()
	rightVal.Lock()
	defer leftVal.Unlock()
	defer rightVal.Unlock()

	leftVal.data = copyT(leftVal.data, rightVal.data)

	return left
}
