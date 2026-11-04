// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package environmentbrowser provides inventory-keyed discovery caching.
package environmentbrowser

import (
	"sync"
	"time"
)

// RefreshInterval bounds detection latency for inventory changes in vCenter.
const RefreshInterval = 5 * time.Minute

// Cache retains successful queries by object identity and inventory revision.
// Its zero value is usable. Values are immutable after insertion. A cold cache
// (including after leader failover) queries again; errors are never cached.
type Cache[T any] struct {
	mu      sync.Mutex
	entries map[string]entry[T]
}

type entry[T any] struct {
	revision string
	value    T
}

// Get returns the query result for revision, querying only on a cache miss.
// Empty revisions disable caching, since an unknown inventory is not proof
// that the environment is unchanged.
func (c *Cache[T]) Get(key, revision string, query func() (T, error)) (T, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[key]; ok && revision != "" && e.revision == revision {
		return e.value, nil
	}
	// Invalidate before querying so a failure cannot resurrect stale results.
	delete(c.entries, key)
	v, err := query()
	if err == nil && revision != "" {
		if c.entries == nil {
			c.entries = map[string]entry[T]{}
		}
		c.entries[key] = entry[T]{revision: revision, value: v}
	}
	return v, err
}

// Delete releases the cached result when its Kubernetes object is deleted.
func (c *Cache[T]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
