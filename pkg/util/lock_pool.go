// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"reflect"
	"sync"
)

// LockPool is a synchronized set of maps that can be obtained with a provided
// ID.
type LockPool[K any, V sync.Locker] struct {
	ool sync.Map
}

// Get returns the lock for the provided ID.
func (p *LockPool[K, V]) Get(id K) sync.Locker {
	if obj, ok := p.ool.Load(id); ok {
		return obj.(V)
	}

	var (
		v          V
		objToStore any
	)

	tv := reflect.TypeOf(v)
	if tv.Kind() == reflect.Pointer {
		// Instantiate the type to which V points, ex. if V is a *sync.Mutex
		// then we want to instantiate a sync.Mutex and use its address.
		objToStore = reflect.New(tv.Elem()).Interface()
	}

	// If objToStore does not implement V then &objToStore will.
	if _, ok := objToStore.(V); !ok {
		objToStore = &objToStore
	}

	obj, _ := p.ool.LoadOrStore(id, objToStore)
	return obj.(V)
}

// Delete removes the specified lock from the pool.
func (p *LockPool[K, V]) Delete(id K) {
	p.ool.Delete(id)
}
