// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmconfig

import (
	"context"
	"maps"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type contextValueType map[string]Reconciler

// Register registers the reconciler in the provided context.
// Please note, if two reconcilers have the same name, the last one registered
// takes precedence.
func Register(ctx context.Context, r Reconciler) context.Context {
	ctxgen.SetContext(
		ctx,
		contextKeyValue,
		func(curVal contextValueType) contextValueType {
			curVal[r.Name()] = r
			return curVal
		})
	if rwc, ok := r.(ReconcilerWithContext); ok {
		return rwc.WithContext(ctx)
	}
	return ctx
}

// FromContext returns the list of registered reconcilers.
func FromContext(ctx context.Context) []Reconciler {
	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(val contextValueType) []Reconciler {
			var list []Reconciler
			for _, r := range val {
				list = append(list, r)
			}
			return list
		})
}

// WithContext returns a new context with a new reconcilers map, with the
// provided context as the parent.
func WithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() contextValueType { return contextValueType{} })
}

// NewContext returns a new context with a new reconcilers map.
func NewContext() context.Context {
	return ctxgen.NewContext(
		contextKeyValue,
		func() contextValueType { return contextValueType{} })
}

// ValidateContext returns true if the provided context contains the reconcilers
// map.
func ValidateContext(ctx context.Context) bool {
	return ctxgen.ValidateContext[contextValueType](ctx, contextKeyValue)
}

// JoinContext returns a new context that contains a reference to the
// reconcilers map from the specified context.
// This function panics if the provided context does not contain a reconcilers
// map.
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
