// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package ovfcache

import (
	"context"
	"time"

	"github.com/vmware/govmomi/ovf"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ovfcache/internal"
)

// GetterFn returns an OVF envelope for a provided library item ID.
type GetterFn = internal.GetterFn

const (
	maxItems            = 100
	expireAfter         = 30 * time.Minute
	expireCheckInterval = 5 * time.Minute
)

// ErrNoGetter is returned from GetOVFEnvelope if there is no getter function.
var ErrNoGetter = internal.ErrNoGetter

// WithContext returns a new context with an OVF cache.
func WithContext(parent context.Context) context.Context {
	return internal.WithContext(
		parent,
		maxItems,
		expireAfter,
		expireCheckInterval)
}

// JoinContext returns a context with the OVF cache from the right or left, in
// that order.
func JoinContext(left, right context.Context) context.Context {
	return internal.JoinContext(left, right)
}

// SetGetter assigns to the context the function used to retrieve an OVF
// envelope when it is not in the cache.
func SetGetter(parent context.Context, getter GetterFn) {
	internal.SetGetter(parent, getter)
}

// GetOVFEnvelope returns the OVF envelope for the provided item ID, either from
// the cache or from vSphere. If the item is not in the cache, it will be cached
// prior to being returned.
func GetOVFEnvelope(
	ctx context.Context,
	itemID, contentVersion string) (env *ovf.Envelope, err error) {

	return internal.GetOVFEnvelope(ctx, itemID, contentVersion)
}
