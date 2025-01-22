// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"context"
	"errors"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

type contextValueType = *Watcher

// setContext assigns the add/remove functions to the context.
func setContext(
	parent context.Context,
	newVal contextValueType) {
	ctxgen.SetContext(
		parent,
		contextKeyValue,
		func(curVal contextValueType) contextValueType {
			return newVal
		})
}

// WithContext returns a new context with a new functions object.
func WithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() contextValueType {
			return nil
		})
}

// NewContext returns a new context with a new functions object.
func NewContext() context.Context {
	return WithContext(context.Background())
}

// ValidateContext returns true if the provided context contains the functions
// object.
func ValidateContext(ctx context.Context) bool {
	return ctxgen.ValidateContext[contextValueType](ctx, contextKeyValue)
}

// JoinContext returns a new context that contains a reference to the functions
// object from the specified context.
// This function panics if the provided context does not contain a functions
// object.
// This function is thread-safe.
func JoinContext(left, right context.Context) context.Context {
	return ctxgen.JoinContext(
		left,
		right,
		contextKeyValue,
		func(dst, src contextValueType) contextValueType {
			return src
		})
}

var (
	// ErrAsyncSignalDisabled is returned from the Add/Remove functions if they
	// are called while async signal is disabled.
	ErrAsyncSignalDisabled = errors.New("async signal disabled")

	// ErrNoWatcher is returned from the Add/Remove functions if they are called
	// without a watcher in the context.
	ErrNoWatcher = errors.New("no watcher")
)

// Add starts watching a container to which VirtualMachine resources may belong,
// such as a Folder, Cluster, ResourcePool, etc.
func Add(ctx context.Context, ref moRef, id string) (err error) {
	if !pkgcfg.FromContext(ctx).AsyncSignalEnabled {
		return ErrAsyncSignalDisabled
	}
	ctxgen.ExecWithContext(
		ctx,
		contextKeyValue,
		func(w contextValueType) {
			if w == nil {
				err = ErrNoWatcher
			} else {
				err = w.add(ctx, ref, id)
			}
		})
	return
}

// Remove stops watching a container to which VirtualMachine resources may
// belong, such as a Folder, Cluster, ResourcePool, etc.
func Remove(ctx context.Context, ref moRef, id string) (err error) {
	if !pkgcfg.FromContext(ctx).AsyncSignalEnabled {
		return ErrAsyncSignalDisabled
	}
	ctxgen.ExecWithContext(
		ctx,
		contextKeyValue,
		func(w contextValueType) {
			if w == nil {
				err = ErrNoWatcher
			} else {
				err = w.remove(ctx, ref, id)
			}
		})
	return
}
