// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgmgr "github.com/vmware-tanzu/vm-operator/pkg/manager"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

// DeferredInstanceStoragePVCCache manages a deferred PVC cache and watch for
// instance storage volumes.
//
// It lazily initializes the cache and watch only when first needed, avoiding
// unnecessary resource usage when instance storage is not in use. The cache
// is filtered to only watch PVCs with the instance storage label, and the
// watch is configured to enqueue the owning VirtualMachine for reconciliation
// when PVCs change.
//
// This is designed to be used by volume controllers that need to watch
// instance storage PVCs but want to defer the cost of the watch until
// actually needed.
type DeferredInstanceStoragePVCCache struct {
	cache        ctrlcache.Cache
	watchStarted bool
	mutex        sync.Mutex
	logger       logr.Logger

	// Dependencies for lazy initialization.
	controller controller.Controller
	mgr        manager.Manager
	syncPeriod *time.Duration
}

// NewDeferredInstanceStoragePVCCache creates a new deferred instance storage
// PVC cache.
//
// The cache and watch are created lazily on the first call to GetClient(),
// which helps avoid unnecessary resource usage when instance storage is not
// in use.
//
// Parameters:
//   - c: The controller that will own the watch
//   - mgr: The manager that provides the scheme and REST mapper
//   - syncPeriod: The sync period for the cache
//   - logger: The logger for logging initialization events
func NewDeferredInstanceStoragePVCCache(
	c controller.Controller,
	mgr manager.Manager,
	syncPeriod *time.Duration,
	logger logr.Logger,
) *DeferredInstanceStoragePVCCache {
	return &DeferredInstanceStoragePVCCache{
		controller: c,
		mgr:        mgr,
		syncPeriod: syncPeriod,
		logger:     logger,
	}
}

// GetClient returns a client.Reader for reading instance storage PVCs.
//
// On first call, it initializes the cache and sets up the watch. Subsequent
// calls return the cached reader without re-initialization. If initialization
// fails, the cache is cleaned up and the error is returned.
//
// This method is thread-safe and can be called concurrently.
func (c *DeferredInstanceStoragePVCCache) GetClient() (client.Reader, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// If already started, return the existing cache
	if c.watchStarted {
		return c.cache, nil
	}

	// Create the cache and set up the watch
	cache, err := c.createPVCCache()
	if err != nil {
		return nil, err
	}

	// Set cache temporarily so setupWatch can access it.
	c.cache = cache

	if err := c.setupWatch(); err != nil {
		// Clean up cache on failure to avoid inconsistent state.
		c.cache = nil
		return nil, err
	}

	c.logger.Info("Started deferred PVC cache and watch for instance storage")
	c.watchStarted = true
	return c.cache, nil
}

// createPVCCache creates a cache that only watches instance storage PVCs.
//
// The cache uses a label selector to filter for PVCs with the instance storage
// label, ensuring only relevant PVCs are cached and reducing memory overhead.
func (c *DeferredInstanceStoragePVCCache) createPVCCache() (ctrlcache.Cache, error) {
	// PVC label set on instance storage PVCs
	isPVCLabels := metav1.LabelSelector{
		MatchLabels: map[string]string{constants.InstanceStorageLabelKey: "true"},
	}
	labelSelector, err := metav1.LabelSelectorAsSelector(&isPVCLabels)
	if err != nil {
		return nil, err
	}

	// This cache will only contain instance storage PVCs because of
	// the label selector
	pvcCache, err := pkgmgr.NewLabelSelectorCacheForObject(
		c.mgr,
		c.syncPeriod,
		&corev1.PersistentVolumeClaim{},
		labelSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to create PVC cache: %w", err)
	}

	return pvcCache, nil
}

// setupWatch configures the controller to watch for PVC changes.
//
// The watch is configured to enqueue the VirtualMachine that owns each PVC,
// ensuring the VM reconciler is triggered when its instance storage PVCs
// change state.
func (c *DeferredInstanceStoragePVCCache) setupWatch() error {
	// Watch for changes to PersistentVolumeClaim, and enqueue
	// VirtualMachine which is the owner of PersistentVolumeClaim
	if err := c.controller.Watch(source.Kind(
		c.cache,
		&corev1.PersistentVolumeClaim{},
		handler.TypedEnqueueRequestForOwner[*corev1.PersistentVolumeClaim](
			c.mgr.GetScheme(),
			c.mgr.GetRESTMapper(),
			&vmopv1.VirtualMachine{},
			handler.OnlyControllerOwner(),
		),
	)); err != nil {
		return fmt.Errorf(
			"failed to start VirtualMachine watch for PersistentVolumeClaim: %w",
			err)
	}

	return nil
}
