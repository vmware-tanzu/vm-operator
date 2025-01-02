// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"sync"
	"time"
)

// CacheItem wraps T so it has a LastUpdated field and can be used with Cache.
type CacheItem[T any] struct {
	// Item is the cached item.
	Item T

	// LastUpdated is the last time the item was updated.
	LastUpdated time.Time
}

// Cache is a generic implementation of a cache that can be configured with a
// maximum number of items and can evict items after a certain amount of time.
type Cache[T any] struct {
	mu       sync.Mutex
	items    map[string]CacheItem[T]
	maxItems int
	done     chan struct{}
	donce    sync.Once

	// expiredChan returns a channel on which the IDs of items that are expired
	// are sent. This channel is closed when the cache is closed, but only after
	// all, pending eviction notices are received.
	expiredChan chan string
	expiringNum sync.WaitGroup
}

// NewCache initializes a new cache with the provided expiration options.
func NewCache[T any](
	expireAfter, checkExpireInterval time.Duration,
	maxItems int) *Cache[T] {

	c := &Cache[T]{
		expiredChan: make(chan string),
		done:        make(chan struct{}),
		items:       map[string]CacheItem[T]{},
		maxItems:    maxItems,
	}

	// Start a goroutine to expire items from the cache.
	go func() {
		ticker := time.NewTicker(checkExpireInterval)

		// evictFn iterates over the cached items and evicts expired items
		evictFn := func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			for k, v := range c.items {
				if time.Since(v.LastUpdated) > expireAfter {
					// Delete the item from the cache.
					delete(c.items, k)

					// Notify subscribers that the item with the ID of k has
					// been evicted if the cache is not closed. Otherwise we
					// add one to the number of items being expired. This
					// ensures the channel cannot be closed until all, pending
					// notifications are sent.
					select {
					case <-c.done:
					default:
						c.expiringNum.Add(1)
						go func(k string) {
							c.expiredChan <- k
							c.expiringNum.Done()
						}(k)
					}
				}
			}
		}
		for {
			select {
			case <-c.done:
				ticker.Stop()
				return
			case <-ticker.C:
				evictFn()
			}
		}
	}()

	return c
}

// ExpiredChan returns a channel on which the IDs of items that are expired are
// sent. This channel is closed when the cache is closed, but only after all,
// pending eviction notices are received.
func (c *Cache[T]) ExpiredChan() <-chan string {
	return c.expiredChan
}

// Close shuts down the cache. It is safe to call this function more than once.
func (c *Cache[T]) Close() {
	c.donce.Do(func() {
		close(c.done)

		// Wait until all pending, expiry notifications are received until the
		// expired channel is closed.
		go func() {
			c.expiringNum.Wait()
			close(c.expiredChan)
		}()
	})
}

// Delete removes the specified item from the cache.
func (c *Cache[T]) Delete(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, id)
}

// Get returns the cached item with the provided ID. The function isHit may be
// optionally provided to further determine whether a cached item should hit
// or miss.
func (c *Cache[T]) Get(id string, isHit func(t T) bool) (T, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var emptyT T

	v, ok := c.items[id]
	if !ok {
		return emptyT, false
	}

	if isHit != nil && !isHit(v.Item) {
		return emptyT, false
	}

	return v.Item, true
}

// CachePutResult describes the result of putting an item into the cache.
type CachePutResult uint

const (
	// CachePutResultCreate indicates the item was created in the cache.
	CachePutResultCreate CachePutResult = iota + 1

	// CachePutResultUpdate indicates an existing, cached item was updated.
	CachePutResultUpdate

	// CachePutResultMaxItemsExceeded indicates the item was not stored because
	// the cache's maximum number of items would have been exceeded.
	CachePutResultMaxItemsExceeded
)

// Put stores the provided item in the cache, updating existing items.
func (c *Cache[T]) Put(id string, t T) CachePutResult {
	c.mu.Lock()
	defer c.mu.Unlock()

	v, ok := c.items[id]

	if ok {
		// Update existing item.
		v.Item = t
		v.LastUpdated = time.Now()
		c.items[id] = v
		return CachePutResultUpdate
	}

	// Store the new item in the cache as long as the maximum number of items
	// was note exceeded.
	if len(c.items) < c.maxItems {
		// Store new item in the cache.
		c.items[id] = CacheItem[T]{
			Item:        t,
			LastUpdated: time.Now(),
		}
		return CachePutResultCreate
	}

	return CachePutResultMaxItemsExceeded
}
