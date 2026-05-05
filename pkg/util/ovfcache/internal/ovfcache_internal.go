// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vmware/govmomi/ovf"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

type contextKeyType uint8

type ContextValueType struct {
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

var (
	ovfCacheItemsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "vmservice",
		Name:      "ovf_cache_items",
		Help:      "Total number of items currently stored in the OVF cache.",
	})
)

func init() {
	ctrlmetrics.Registry.MustRegister(ovfCacheItemsGauge)
}

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
		func() ContextValueType {
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
					ovfCacheItemsGauge.Set(float64(cache.Size()))
				}
			}()

			return ContextValueType{
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
		func(curVal ContextValueType) pkgutil.CachePutResult {
			logger := pkglog.FromContextOrDefault(ctx).
				WithValues(
					"itemID", itemID,
				).V(4)
			putResult := curVal.cache.Put(itemID, env)
			ovfCacheItemsGauge.Set(float64(curVal.cache.Size()))
			logger.Info("Cache item put",
				"itemID", itemID,
				"putResult", putResult,
				"cacheItemsCount", curVal.cache.Size(),
			)
			return putResult
		})
}

func Cache(ctx context.Context) ContextValueType {
	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(curVal ContextValueType) ContextValueType {
			return curVal
		})
}

func GetLock(
	ctx context.Context,
	itemID string) sync.Locker {

	return ctxgen.FromContext(
		ctx,
		contextKeyValue,
		func(curVal ContextValueType) sync.Locker {
			return curVal.locks.Get(itemID)
		})
}

func SetGetter(parent context.Context, getter GetterFn) {
	ctxgen.SetContext(
		parent,
		contextKeyValue,
		func(curVal ContextValueType) ContextValueType {
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
		func(val ContextValueType) {
			logger := pkglog.FromContextOrDefault(ctx).
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
			ovfCacheItemsGauge.Set(float64(val.cache.Size()))
			logger.Info("Cache item put",
				"itemID", itemID,
				"putResult", putResult,
				"cacheItemsCount", val.cache.Size(),
			)
		})
	return env, err

}

func JoinContext(left, right context.Context) context.Context {
	return ctxgen.JoinContext(
		left,
		right,
		contextKeyValue,
		func(dst, src ContextValueType) ContextValueType {
			return src
		})
}
