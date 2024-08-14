// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"context"
	"sync"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type operationType uint8

const (
	none operationType = iota
	create
	update
)

type operation struct {
	sync.RWMutex
	typ3 operationType
}

// IsCreate returns whether or not this is a create operation.
func IsCreate(ctx context.Context) bool {
	return fromContext(ctx) == create
}

// IsUpdate returns whether or not this is an update operation.
func IsUpdate(ctx context.Context) bool {
	return fromContext(ctx) == update
}

// MarkCreate indicates to the call stack this is a create operation.
func MarkCreate(ctx context.Context) {
	setType(ctx, create)
}

// MarkUpdate indicates to the call stack this is an update operation.
// This will be a no-op if MarkCreate was already called.
func MarkUpdate(ctx context.Context) {
	setType(ctx, update)
}

func fromContext(ctx context.Context) operationType {
	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(contextKeyValue)
	if obj == nil {
		panic("operation is missing from context")
	}
	data := obj.(*operation)
	data.RLock()
	defer data.RUnlock()
	return data.typ3
}

func setType(ctx context.Context, r operationType) {
	if ctx == nil {
		panic("context is nil")
	}
	obj := ctx.Value(contextKeyValue)
	if obj == nil {
		panic("operation is missing from context")
	}
	data := obj.(*operation)
	data.Lock()
	defer data.Unlock()

	if data.typ3 != create {
		data.typ3 = r
	}
}

// WithContext returns a new operation context.
func WithContext(parent context.Context) context.Context {
	if parent == nil {
		panic("parent context is nil")
	}
	return context.WithValue(
		parent,
		contextKeyValue,
		&operation{})
}

// JoinContext returns a new context that contains the operation from the right
// or left side of the context, with the right taking precedence.
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
		panic("operation is missing from context")
	}

	if leftObj != nil && rightObj == nil {
		// The left context has the operation and the right does not, so just
		// return the left context.
		return left
	}

	// The right context has the operation, so return a new context with the
	// operation in it.
	return context.WithValue(left, contextKeyValue, rightObj.(*operation))
}
