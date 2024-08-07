// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package cource

import (
	"context"
	"maps"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/event"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type sourceMap struct {
	sync.Mutex
	data map[any]chan event.GenericEvent
}

// FromContext creates or gets the channel for the specified key.
func FromContext(ctx context.Context, key any) chan event.GenericEvent {

	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(contextKeyValue)
	if obj == nil {
		panic("map is missing from context")
	}

	m := obj.(*sourceMap)
	m.Lock()
	defer m.Unlock()

	if c, ok := m.data[key]; ok {
		return c
	}

	c := make(chan event.GenericEvent)
	m.data[key] = c

	return c
}

// WithContext returns a new context with a new channels map, with the provided
// context as the parent.
func WithContext(parent context.Context) context.Context {

	if parent == nil {
		panic("parent context is nil")
	}
	return context.WithValue(
		parent,
		contextKeyValue,
		&sourceMap{
			data: map[any]chan event.GenericEvent{},
		},
	)
}

// NewContext returns a new context with a new channels map.
func NewContext() context.Context {
	return WithContext(context.Background())
}

// ValidateContext returns true if the provided context contains the channels
// map and key.
func ValidateContext(ctx context.Context) bool {
	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(contextKeyValue)
	if obj == nil {
		return false
	}
	_, ok := obj.(*sourceMap)
	return ok
}

// JoinContext returns a new context that contains a reference to the channels
// map from the specified context.
// This function panics if the provided context does not contain a channels map.
// This function is thread-safe.
func JoinContext(left, right context.Context) context.Context {

	if left == nil {
		panic("left context is nil")
	}
	if right == nil {
		panic("right context is nil")
	}

	leftObj := left.Value(contextKeyValue)
	rightObj := right.Value(contextKeyValue)

	if leftObj == nil && rightObj == nil {
		panic("map is missing from context")
	}

	if leftObj != nil && rightObj == nil {
		// The left context has the map and the right does not, so just return
		// the left context.
		return left
	}

	if leftObj == nil && rightObj != nil {
		// The right context has the map and the left does not, so return a new
		// context with the map in it.
		return context.WithValue(left, contextKeyValue, rightObj.(*sourceMap))
	}

	// Both contexts have the map, so the left context is returned after the map
	// from the right context is used to overwrite the map from the left.

	leftMap := leftObj.(*sourceMap)
	rightMap := rightObj.(*sourceMap)

	leftMap.Lock()
	rightMap.Lock()
	defer leftMap.Unlock()
	defer rightMap.Unlock()

	maps.Copy(leftMap.data, rightMap.data)

	return left
}
