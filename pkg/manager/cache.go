// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
)

type implHasCache struct {
	ctrlcache.Cache
}

// GetCache implements the ctrl-runtime manager hasCache interface so these caches
// end up on the managers Cache runnable list and get started along with the other
// caches.
func (c *implHasCache) GetCache() ctrlcache.Cache {
	return c
}

// NewLabelSelectorCacheForObject creates a new cache that watches the specified
// object type selected by the label selector in all namespaces . The cache is
// added to the manager and starts alongside the other leader-election runnables.
func NewLabelSelectorCacheForObject(
	mgr ctrlmgr.Manager,
	resync *time.Duration,
	object ctrlclient.Object,
	selector labels.Selector) (ctrlcache.Cache, error) {

	cache, err := ctrlcache.New(mgr.GetConfig(),
		ctrlcache.Options{
			Scheme:           mgr.GetScheme(),
			Mapper:           mgr.GetRESTMapper(),
			DefaultTransform: ctrlcache.TransformStripManagedFields(),
			SyncPeriod:       resync,
			ByObject: map[ctrlclient.Object]ctrlcache.ByObject{
				object: {
					Label: selector,
				},
			},
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create label selector cache for %T for namespaces: %w", object, err)
	}

	c := &implHasCache{Cache: cache}

	if err := mgr.Add(c); err != nil {
		return nil, fmt.Errorf("failed to add label selector cache for %T: %w", object, err)
	}

	return cache, nil
}

// NewNamespacedCacheForObject creates a new cache that watches the specified
// object type only for the specified namespaces. The cache is added to the
// manager and starts alongside the other leader-election runnables.
func NewNamespacedCacheForObject(
	mgr ctrlmgr.Manager,
	resync *time.Duration,
	object ctrlclient.Object,
	namespaces ...string) (ctrlcache.Cache, error) {

	cache, err := ctrlcache.New(mgr.GetConfig(),
		ctrlcache.Options{
			Scheme:           mgr.GetScheme(),
			Mapper:           mgr.GetRESTMapper(),
			DefaultTransform: ctrlcache.TransformStripManagedFields(),
			SyncPeriod:       resync,
			ByObject: map[ctrlclient.Object]ctrlcache.ByObject{
				object: {
					Namespaces: GetNamespaceCacheConfigs(namespaces...),
				},
			},
		},
	)

	if err != nil {
		return nil, fmt.Errorf(
			"failed to create cache for %T for namespaces %v: %w",
			object, namespaces, err)
	}

	c := &implHasCache{Cache: cache}

	if err := mgr.Add(c); err != nil {
		return nil, fmt.Errorf(
			"failed to add cache for %T for namespaces %v: %w",
			object, namespaces, err)
	}

	return cache, nil
}

// GetNamespaceCacheConfigs returns a map of cache configurations for the
// provided namespaces. A nil value is returned if the provided list is
// empty or has a single, empty element.
func GetNamespaceCacheConfigs(namespaces ...string) map[string]ctrlcache.Config {
	if len(namespaces) == 0 {
		return nil
	}
	if len(namespaces) == 1 && namespaces[0] == "" {
		return nil
	}
	nsc := make(map[string]ctrlcache.Config, len(namespaces))
	for i := range namespaces {
		if namespaces[i] != "" {
			nsc[namespaces[i]] = ctrlcache.Config{}
		}
	}
	return nsc
}
