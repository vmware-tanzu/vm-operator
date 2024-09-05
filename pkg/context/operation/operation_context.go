// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package operation

import (
	"context"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type contextValueType uint8

const (
	none contextValueType = iota
	create
	update
)

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

func fromContext(ctx context.Context) contextValueType {
	return ctxgen.FromContext[contextValueType](
		ctx,
		contextKeyValue,
		func(val contextValueType) contextValueType {
			return val
		})
}

func setType(ctx context.Context, newVal contextValueType) {
	ctxgen.SetContext(
		ctx,
		contextKeyValue,
		func(curVal contextValueType) contextValueType {
			if curVal == create {
				return curVal
			}
			return newVal
		})
}

// WithContext returns a new operation context.
func WithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() contextValueType { return none })
}

// JoinContext returns a new context that contains the operation from the right
// or left side of the context, with the right taking precedence.
// This function is thread-safe.
func JoinContext(left, right context.Context) context.Context {
	return ctxgen.JoinContext(
		left,
		right,
		contextKeyValue,
		func(dst, src contextValueType) contextValueType {
			if dst == create {
				return dst
			}
			return src
		})
}
