// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package cource

import (
	"context"
	"maps"

	"sigs.k8s.io/controller-runtime/pkg/event"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type contextValueType map[any]chan event.GenericEvent

// FromContext creates or gets the channel for the specified key.
func FromContext(ctx context.Context, key any) chan event.GenericEvent {
	return FromContextWithBuffer(ctx, key, 0)
}

// FromContextWithBuffer creates or gets the buffered channel for the specified
// key and buffer size.
func FromContextWithBuffer(
	ctx context.Context,
	key any,
	buffer int) chan event.GenericEvent {

	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(val contextValueType) chan event.GenericEvent {
			if c, ok := val[key]; ok {
				return c
			}
			c := make(chan event.GenericEvent, buffer)
			val[key] = c
			return c
		})
}

// WithContext returns a new context with a new channels map, with the provided
// context as the parent.
func WithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() contextValueType { return contextValueType{} })
}

// NewContext returns a new context with a new channels map.
func NewContext() context.Context {
	return WithContext(context.Background())
}

// ValidateContext returns true if the provided context contains the channels
// map and key.
func ValidateContext(ctx context.Context) bool {
	return ctxgen.ValidateContext[contextValueType](ctx, contextKeyValue)
}

// JoinContext returns a new context that contains a reference to the channels
// map from the specified context.
// This function panics if the provided context does not contain a channels map.
// This function is thread-safe.
func JoinContext(left, right context.Context) context.Context {
	return ctxgen.JoinContext(
		left,
		right,
		contextKeyValue,
		func(dst, src contextValueType) contextValueType {
			maps.Copy(dst, src)
			return dst
		})
}
