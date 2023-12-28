// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func cpuFreqTests() {

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim
		vmProvider vmprovider.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{WithV1A2: true}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vmProvider = nil
	})

	Context("ComputeCPUMinFrequency", func() {
		It("returns success", func() {
			Expect(vmProvider.ComputeCPUMinFrequency(ctx)).To(Succeed())
		})
	})
}

func initOvfCacheAndLockPoolTests() {

	var (
		expireAfter         = 3 * time.Second
		checkExpireInterval = 1 * time.Second
		maxItems            = 3

		ovfCache    *util.Cache[vsphere.VersionedOVFEnvelope]
		ovfLockPool *util.LockPool[string, *sync.RWMutex]
	)

	BeforeEach(func() {
		ovfCache, ovfLockPool = vsphere.InitOvfCacheAndLockPool(
			expireAfter, checkExpireInterval, maxItems)
	})

	AfterEach(func() {
		ovfCache = nil
		ovfLockPool = nil
	})

	Context("InitOvfCacheAndLockPool", func() {
		It("should clean up lock pool when the item is expired in cache", func() {
			Expect(ovfCache).ToNot(BeNil())
			Expect(ovfLockPool).ToNot(BeNil())

			itemID := "test-item-id"
			res := ovfCache.Put(itemID, vsphere.VersionedOVFEnvelope{})
			Expect(res).To(Equal(util.CachePutResultCreate))
			curItemLock := ovfLockPool.Get(itemID)
			Expect(curItemLock).ToNot(BeNil())

			Eventually(func() bool {
				// ovfLockPool.Get() returns a new lock if the item key is not found.
				// So the lock should be different when the item is expired and deleted from pool.
				return ovfLockPool.Get(itemID) != curItemLock
			}, 5*time.Second, 1*time.Second).Should(BeTrue())
		})
	})
}
