// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"time"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"
)

// NewNamespaceCache creates a cache.Cache that watches only the given namespace. Adds cache
// to the Manager so it starts along with the other leader-election runnables.
func NewNamespaceCache(mgr ctrlmgr.Manager, resync *time.Duration, namespace string) (cache.Cache, error) {
	nsCache, err := cache.New(mgr.GetConfig(),
		cache.Options{
			Scheme:    mgr.GetScheme(),
			Mapper:    mgr.GetRESTMapper(),
			Resync:    resync,
			Namespace: namespace,
		},
	)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create cache for namespace %s", namespace)
	}

	if err := mgr.Add(nsCache); err != nil {
		return nil, errors.Wrapf(err, "failed to add cache for namespace %s", namespace)
	}

	return nsCache, nil
}
