// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("createFastDeploy", func() {
	var (
		ctx        *builder.TestContextForVCSim
		fm         *object.FileManager
		vm         *vmopv1.VirtualMachine
		ds         *object.Datastore
		vmCtx      pkgctx.VirtualMachineContext
		vmDirName  string
		vmDirPath  string
		configSpec vimtypes.VirtualMachineConfigSpec
		createArgs *vmlifecycle.CreateArgs
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{
			WithContentLibrary: true,
		})

		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.FastDeploy = true
		})

		vm = builder.DummyVirtualMachine()
		vm.Name = "fastdeploy-stale-dir-test"
		vm.UID = "stale-dir-test-uid-12345"
		vm.Namespace = pkgcfg.FromContext(ctx).PodNamespace

		fm = object.NewFileManager(ctx.VCClient.Client)
		ds = object.NewDatastore(ctx.VCClient.Client, ctx.Datastore.Reference())
		Expect(ds.FindInventoryPath(ctx)).To(Succeed(),
			"failed to set InventoryPath and DatacenterPath for datastore object")

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}

		vmDirName = string(vm.UID)
		vmDirPath = fmt.Sprintf("[%s] %s", ctx.Datastore.Name(), vmDirName)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		fm = nil
		vm = nil
		ds = nil
		createArgs = nil
	})

	JustBeforeEach(func() {
		configSpec = vimtypes.VirtualMachineConfigSpec{
			Name: vm.Name,
			Files: &vimtypes.VirtualMachineFileInfo{
				VmPathName: fmt.Sprintf("%s/%s.vmx", vmDirPath, vm.Name),
			},
		}

		dcFolders, err := ctx.Datacenter.Folders(ctx)
		Expect(err).ToNot(HaveOccurred(), "failed to fetch datacenter folder properties")
		pools, err := ctx.Finder.ResourcePoolList(ctx, "*")
		Expect(err).ToNot(HaveOccurred(), "failed to list resource pools")
		Expect(pools).ToNot(BeEmpty(), "no resource pools found in the test environment")
		defaultPool := pools[0]

		createArgs = &vmlifecycle.CreateArgs{
			ConfigSpec:        configSpec,
			UseContentLibrary: true,
			Datastores: []vmlifecycle.DatastoreRef{
				{
					MoRef:                            ctx.Datastore.Reference(),
					TopLevelDirectoryCreateSupported: true,
				},
			},
			DatacenterMoID:   ctx.Datacenter.Reference().Value,
			DiskPaths:        []string{ctx.ContentLibraryItem1Disk1Path}, // Only 1 disk
			FilePaths:        []string{ctx.ContentLibraryItem1NVRAMPath},
			ProviderItemID:   ctx.ContentLibraryItem1ID,
			FolderMoID:       dcFolders.VmFolder.Reference().Value,
			ResourcePoolMoID: defaultPool.Reference().Value,
		}
	})

	When("no issues with fast deploy vm creation", func() {
		It("should succeed", func() {
			_, err := vmlifecycle.CreateVirtualMachine(
				vmCtx,
				ctx.Client,
				ctx.RestClient,
				ctx.VCClient.Client,
				ctx.Finder,
				createArgs)
			Expect(err).ToNot(HaveOccurred(), "failed to create a new VM")

			_, dirExistsErr := ds.Stat(vmCtx, vmDirName)
			Expect(dirExistsErr).To(BeNil(), "the directory should exist")
		})
	})

	createWithStaleDirectory := func() {
		By("create a new directory")
		err := fm.MakeDirectory(ctx, vmDirPath, ctx.Datacenter, true)
		Expect(err).To(BeNil(), "creating a new directory failed")

		By("verify directory exists initially")
		_, dirExistsErr := ds.Stat(vmCtx, vmDirName)
		Expect(dirExistsErr).To(BeNil(), "the directory should exist")

		By("create a new VM with stale directory")
		_, err = vmlifecycle.CreateVirtualMachine(
			vmCtx,
			ctx.Client,
			ctx.RestClient,
			ctx.VCClient.Client,
			ctx.Finder,
			createArgs)
		Expect(err).ToNot(HaveOccurred(), "should be able to create a new VM")
	}

	When("there is a stale vm directory with fast deploy vm creation (root directory)", func() {
		It("should delete the stale directory and then succeed", func() {
			createWithStaleDirectory()
		})
	})

	When("there is a stale vm directory with fast deploy vm creation (nested directory)", func() {
		BeforeEach(func() {
			vmDirName = "parent-dir/" + vmDirName
			vmDirPath = fmt.Sprintf("[%s] %s", ctx.Datastore.Name(), vmDirName)
		})

		It("should delete the stale directory and then succeed", func() {
			createWithStaleDirectory()
		})
	})

	When("there is issue with vm creation", func() {
		It("should cleanup the newly created vm directory", func() {

			By("verify no directory exists initially")
			_, dirExistsErr := ds.Stat(vmCtx, vmDirName)
			Expect(errors.As(dirExistsErr, &object.DatastoreNoSuchFileError{})).To(
				BeTrue(), "the directory should not exist")

			// Set ConfigSpec with mismatch disks to force vm creation failure after directory creation
			createArgs.ConfigSpec.DeviceChange = []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: fmt.Sprintf("[%s] %s/disk-0.vmdk", ctx.Datastore.Name(), vmDirName),
								},
							},
						},
						CapacityInKB: 1024,
					},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
					Device: &vimtypes.VirtualDisk{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
								VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
									FileName: fmt.Sprintf("[%s] %s/disk-1.vmdk", ctx.Datastore.Name(), vmDirName),
								},
							},
						},
						CapacityInKB: 1024,
					},
				},
			}

			By("create a VM with mismatch disks")
			_, err := vmlifecycle.CreateVirtualMachine(
				vmCtx,
				ctx.Client,
				ctx.RestClient,
				ctx.VCClient.Client,
				ctx.Finder,
				createArgs)
			Expect(err).To(HaveOccurred(), "should fail to create a new VM")

			By("verify no directory exists after")
			_, dirExistsErr = ds.Stat(vmCtx, vmDirName)
			Expect(errors.As(dirExistsErr, &object.DatastoreNoSuchFileError{})).To(
				BeTrue(), "the directory should not exist after cleanup")
		})
	})

})
