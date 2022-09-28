// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmpublish

import (
	"sync"
)

const (
	StateNotStart = "not_start"
	StateQueued   = "queued"
	StateRunning  = "running"
	StateSuccess  = "success"
	StateError    = "error"
)

type Result struct {
	CLItemUUID string
	State      string
}

type Catalog interface {
	UpdateVMPubRequestResult(vmPubUID, state, itemID string)
	GetPubRequestState(vmPubUID string) (bool, Result)
	RemoveFromPubRequestResult(vmPubUID string)
}

type catalog struct {
	sync.RWMutex
	// clItemToPubReqMap maintains a map, where key is the CL uuid,
	// value is another map between CL item name and pub request.
	// So that we can best effort avoid multiple requests to publish
	// to the content library with the same item name from VM Operator.
	// TODO: operations to this map need to be stored to a ConfigMap
	// to keep it consistent if VM Operator POD crashes.
	clItemToPubReqMap map[string]map[string]string
	// pubReqToResultMap maintains a map, where key is the vmpub uid,
	// value is the publish request result. This is a temp workaround
	// until CLS bug is resolved. 
	pubReqToResultMap map[string]*Result
}

func NewCatalog() Catalog {
	return &catalog{
		clItemToPubReqMap: make(map[string]map[string]string),
		pubReqToResultMap: make(map[string]*Result),
	}
}

func (c *catalog) UpdateVMPubRequestResult(vmPubUID, state, itemID string) {
	c.Lock()
	defer c.Unlock()

	if _, ok := c.pubReqToResultMap[vmPubUID]; !ok {
		c.pubReqToResultMap[vmPubUID] = &Result{}
	}

	c.pubReqToResultMap[vmPubUID].CLItemUUID = itemID
	c.pubReqToResultMap[vmPubUID].State = state
}

func (c *catalog) RemoveFromPubRequestResult(vmPubUID string) {
	c.Lock()
	defer c.Unlock()

	delete(c.pubReqToResultMap, vmPubUID)
}

func (c *catalog) GetPubRequestState(vmPubUID string) (bool, Result) {
	// temp workaround.
	c.Lock()
	defer c.Unlock()

	res, exist := c.pubReqToResultMap[vmPubUID]
	if res == nil {
		return exist, Result{}
	}
	return exist, *res
}
