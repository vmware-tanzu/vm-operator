// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = Describe("LockPoolTest", func() {
	It("should obtain a lock", func() {
		var lp util.LockPool[string, *sync.RWMutex]
		l := lp.Get("hello")
		Expect(l).ToNot(BeNil())
	})
	It("should return the same lock", func() {
		var lp util.LockPool[string, *sync.RWMutex]
		l1 := lp.Get("hello")
		l2 := lp.Get("hello")
		Expect(l1).To(BeIdenticalTo(l2))
	})
	It("should return different locks", func() {
		var lp util.LockPool[string, *sync.RWMutex]
		l1 := lp.Get("hello")
		l2 := lp.Get("world")
		Expect(l1).ToNot(BeIdenticalTo(l2))
	})
	It("should delete a lock then return a different lock for the same ID", func() {
		var lp util.LockPool[string, *sync.RWMutex]
		l1 := lp.Get("hello")
		l2 := lp.Get("hello")
		Expect(l1).To(BeIdenticalTo(l2))
		lp.Delete("hello")
		l2 = lp.Get("hello")
		Expect(l1).ToNot(BeIdenticalTo(l2))
	})
})
