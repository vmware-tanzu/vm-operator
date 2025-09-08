// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"

	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
)

type clientContextKey uint8

const (
	vimClientContextKey clientContextKey = iota
	restClientContextKey
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
