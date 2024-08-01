// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/library"
	"k8s.io/apimachinery/pkg/types"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vsphere "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func cpuFreqTests() {

	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
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

func syncVirtualMachineImageTests() {
	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig.WithContentLibrary = true
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	When("content library item is an unexpected K8s object type", func() {
		It("should return an error", func() {
			err := vmProvider.SyncVirtualMachineImage(ctx, &imgregv1a1.ContentLibrary{}, &vmopv1.VirtualMachineImage{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("unexpected content library item K8s object type %T", &imgregv1a1.ContentLibrary{})))
		})
	})

	When("content library item is not an OVF type", func() {
		It("should exit early without updating VM Image status", func() {
			isoItem := &imgregv1a1.ContentLibraryItem{
				Status: imgregv1a1.ContentLibraryItemStatus{
					Type: imgregv1a1.ContentLibraryItemTypeIso,
				},
			}
			var vmi vmopv1.VirtualMachineImage
			Expect(vmProvider.SyncVirtualMachineImage(ctx, isoItem, &vmi)).To(Succeed())
			Expect(vmi.Status).To(Equal(vmopv1.VirtualMachineImageStatus{}))
		})
	})

	When("content library item is an OVF type but it fails to get the OVF envelope", func() {
		It("should return an error", func() {
			ovfItem := &imgregv1a1.ContentLibraryItem{
				Spec: imgregv1a1.ContentLibraryItemSpec{
					// Use an invalid item ID to fail to get the OVF envelope.
					UUID: "invalid-library-ID",
				},
				Status: imgregv1a1.ContentLibraryItemStatus{
					Type: imgregv1a1.ContentLibraryItemTypeOvf,
				},
			}
			err := vmProvider.SyncVirtualMachineImage(ctx, ovfItem, &vmopv1.VirtualMachineImage{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get OVF envelope for library item \"invalid-library-ID\""))
		})
	})

	When("content library item is an OVF type but the OVF envelope is nil", func() {
		It("should return an error", func() {
			libraryItem := library.Item{
				Name: "test-image-ovf-empty",
				// Use a non-OVF type here to cause the retrieved OVF envelope to be nil.
				Type:      "empty",
				LibraryID: ctx.ContentLibraryID,
			}
			itemID := builder.CreateContentLibraryItem(
				ctx,
				library.NewManager(ctx.RestClient),
				libraryItem,
				"",
			)
			Expect(itemID).NotTo(BeEmpty())

			ovfItem := &imgregv1a1.ContentLibraryItem{
				Spec: imgregv1a1.ContentLibraryItemSpec{
					UUID: types.UID(itemID),
				},
				Status: imgregv1a1.ContentLibraryItemStatus{
					Type: imgregv1a1.ContentLibraryItemTypeOvf,
				},
			}
			err := vmProvider.SyncVirtualMachineImage(ctx, ovfItem, &vmopv1.VirtualMachineImage{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("OVF envelope is nil for library item %q", itemID)))
		})
	})

	When("content library item is an OVF type with valid OVF envelope", func() {
		It("should return success and update VM Image status accordingly", func() {
			ovfItem := &imgregv1a1.ContentLibraryItem{
				Spec: imgregv1a1.ContentLibraryItemSpec{
					UUID: types.UID(ctx.ContentLibraryItemID),
				},
				Status: imgregv1a1.ContentLibraryItemStatus{
					Type: imgregv1a1.ContentLibraryItemTypeOvf,
				},
			}
			var vmi vmopv1.VirtualMachineImage
			Expect(vmProvider.SyncVirtualMachineImage(ctx, ovfItem, &vmi)).To(Succeed())
			Expect(vmi.Status.Firmware).To(Equal("efi"))
			Expect(vmi.Status.HardwareVersion).NotTo(BeNil())
			Expect(*vmi.Status.HardwareVersion).To(Equal(int32(9)))
			Expect(vmi.Status.OSInfo.ID).To(Equal("36"))
			Expect(vmi.Status.OSInfo.Type).To(Equal("otherLinuxGuest"))
			Expect(len(vmi.Status.DiskInfo)).To(Equal(1))
			Expect(vmi.Status.DiskInfo[0].Capacity.String()).To(Equal("30Mi"))
			Expect(vmi.Status.DiskInfo[0].Size.String()).To(Equal("18304Ki"))
		})
	})
}
