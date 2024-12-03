// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/ovf"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

type contextKeyType uint8

type contextValueType struct {
	cache *pkgutil.Cache[VersionedOVFEnvelope]
	locks *pkgutil.LockPool[string, *sync.RWMutex]
	getFn GetterFn
}

type VersionedOVFEnvelope struct {
	OvfEnvelope    *ovf.Envelope
	ContentVersion string
}

type GetterFn func(ctx context.Context, itemID string) (*ovf.Envelope, error)

const contextKeyValue contextKeyType = 0

// ErrNoGetter is returned from GetOVFEnvelope if there is no getter function.
var ErrNoGetter = errors.New("ovfcache getter fn is nil")

// WithContext returns a new context with an OVF cache.
func WithContext(
	parent context.Context,
	maxCachedItems int,
	expireAfter, expireCheckInterval time.Duration) context.Context {

	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() contextValueType {
			cache := pkgutil.NewCache[VersionedOVFEnvelope](
				expireAfter,
				expireCheckInterval,
				maxCachedItems)
			locks := &pkgutil.LockPool[string, *sync.RWMutex]{}

			// Clean up the lock pool when the ovf cache item expires.
			go func() {
				for k := range cache.ExpiredChan() {
					l := locks.Get(k)
					// This could still delete an in-use lock if it's retrieved
					// from the pool but not locked yet. If it's already locked,
					// this will wait until it's unlocked to delete it from the
					// pool.
					l.Lock()
					locks.Delete(k)
					l.Unlock()
				}
			}()

			return contextValueType{
				cache: cache,
				locks: locks,
			}
		})
}

func Put(
	ctx context.Context,
	itemID string,
	env VersionedOVFEnvelope) pkgutil.CachePutResult {

	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(curVal contextValueType) pkgutil.CachePutResult {
			return curVal.cache.Put(itemID, env)
		})
}

func GetLock(
	ctx context.Context,
	itemID string) sync.Locker {

	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(curVal contextValueType) sync.Locker {
			return curVal.locks.Get(itemID)
		})
}

func SetGetter(parent context.Context, getter GetterFn) {
	ctxgen.SetContext(
		parent,
		contextKeyValue,
		func(curVal contextValueType) contextValueType {
			curVal.getFn = getter
			return curVal
		})
}

func GetOVFEnvelope(
	ctx context.Context,
	itemID, contentVersion string) (env *ovf.Envelope, err error) {

	ctxgen.ExecWithContext(
		ctx,
		contextKeyValue,
		func(val contextValueType) {
			logger := logr.FromContextOrDiscard(ctx).
				WithValues(
					"itemID", itemID,
					"contentVersion", contentVersion,
				).V(4)

			// Lock the current item to prevent concurrent downloads of the same
			// OVF. This is done before the get from cache below to prevent
			// stale result.
			curItemLock := val.locks.Get(itemID)
			curItemLock.Lock()
			defer curItemLock.Unlock()

			isHitFn := func(e VersionedOVFEnvelope) bool {
				return contentVersion == e.ContentVersion
			}

			cacheItem, found := val.cache.Get(itemID, isHitFn)
			if found {
				logger.Info("Cache item hit, using cached OVF")
				env = cacheItem.OvfEnvelope
				return
			}

			if val.getFn == nil {
				err = ErrNoGetter
				return
			}

			logger.Info("Cache item miss, downloading OVF from vCenter")
			env, err = val.getFn(ctx, itemID)
			if err != nil || env == nil {
				env = nil
				return
			}

			cacheItem = VersionedOVFEnvelope{
				ContentVersion: contentVersion,
				OvfEnvelope:    env,
			}

			putResult := val.cache.Put(itemID, cacheItem)
			logger.Info("Cache item put",
				"itemID", itemID,
				"putResult", putResult)
		})
	return env, err

}
