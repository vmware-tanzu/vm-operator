// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
)

type clientContextKey uint8

const (
	vimClientContextKey clientContextKey = iota
	restClientContextKey
	finderClientContextKey
)

func WithVimClient(
	parent context.Context,
	c *vim25.Client) context.Context {

	return context.WithValue(parent, vimClientContextKey, c)
}

func GetVimClient(ctx context.Context) *vim25.Client {
	obj := ctx.Value(vimClientContextKey)
	if obj == nil {
		return nil
	}

	val, ok := obj.(*vim25.Client)
	if !ok {
		return nil
	}

	return val
}

func WithRestClient(
	parent context.Context,
	c *rest.Client) context.Context {

	return context.WithValue(parent, restClientContextKey, c)
}

func GetRestClient(ctx context.Context) *rest.Client {
	obj := ctx.Value(restClientContextKey)
	if obj == nil {
		return nil
	}

	val, ok := obj.(*rest.Client)
	if !ok {
		return nil
	}

	return val
}

// WithFinder returns a new context with the finder inside of it.
func WithFinder(ctx context.Context, finder *find.Finder) context.Context {
	return context.WithValue(ctx, finderClientContextKey, finder)
}

// GetFinder returns the finder from a context.
// Nil is returned if the finder is not present in the context.
func GetFinder(ctx context.Context) *find.Finder {
	obj := ctx.Value(finderClientContextKey)
	if obj == nil {
		return nil
	}
	c, ok := obj.(*find.Finder)
	if !ok {
		return nil
	}
	return c
}
