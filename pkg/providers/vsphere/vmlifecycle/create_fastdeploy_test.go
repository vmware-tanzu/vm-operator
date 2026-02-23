// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"errors"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("createFastDeploy", func() {

	var (
		ctx   *builder.TestContextForVCSim
		vmCtx pkgctx.VirtualMachineContext
		vm    *vmopv1.VirtualMachine
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{
			WithContentLibrary: true,
		})

		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.FastDeploy = true
		})

		vm = builder.DummyVirtualMachine()
		vm.Name = "fastdeploy-cleanup-test"
		vm.UID = types.UID("cleanup-test-uid-12345")
		vm.Namespace = pkgcfg.FromContext(ctx).PodNamespace

		vmCtx = pkgctx.VirtualMachineContext{
			Context: ctx,
			Logger:  suite.GetLogger().WithValues("vmName", vm.Name),
			VM:      vm,
		}
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("when directory is created successfully", func() {
		// Note: We only test TLD-supported datastores here. Testing non-TLD datastores
		// (vSAN) is limited because vcsim may not enforce the "non-empty directory"
		// restriction that real vSphere/vSAN does, so we cannot validate the two-step
		// cleanup approach (DeleteDatastoreFile + DeleteDirectory) in the simulator.
		Context("with TLD-supported datastore (using FileManager.MakeDirectory)", func() {
			It("should cleanup directory when error occurs after directory creation", func() {
				// Similar test but for TLD-supported datastores
				// This uses FileManager.MakeDirectory instead of CreateDirectory

				vmicName := pkgutil.VMIName(ctx.ContentLibraryItemID)
				vmic := vmopv1.VirtualMachineImageCache{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: pkgcfg.FromContext(ctx).PodNamespace,
						Name:      vmicName,
					},
					Status: vmopv1.VirtualMachineImageCacheStatus{
						OVF: &vmopv1.VirtualMachineImageCacheOVFStatus{
							ConfigMapName:   vmicName,
							ProviderVersion: ctx.ContentLibraryItemVersion,
						},
						Conditions: []metav1.Condition{
							{
								Type:   vmopv1.VirtualMachineImageCacheConditionHardwareReady,
								Status: metav1.ConditionTrue,
							},
							{
								Type:   vmopv1.VirtualMachineImageCacheConditionFilesReady,
								Status: metav1.ConditionTrue,
							},
						},
						Locations: []vmopv1.VirtualMachineImageCacheLocationStatus{
							{
								DatacenterID: ctx.Datacenter.Reference().Value,
								DatastoreID:  ctx.Datastore.Reference().Value,
								ProfileID:    ctx.StorageProfileID,
								Files: []vmopv1.VirtualMachineImageCacheFileStatus{
									{
										ID:       ctx.ContentLibraryItemDiskPath,
										Type:     vmopv1.VirtualMachineImageCacheFileTypeDisk,
										DiskType: vmopv1.VolumeTypeClassic,
									},
									{
										ID:   ctx.ContentLibraryItemNVRAMPath,
										Type: vmopv1.VirtualMachineImageCacheFileTypeOther,
									},
								},
								Conditions: []metav1.Condition{
									{
										Type:   vmopv1.ReadyConditionType,
										Status: metav1.ConditionTrue,
									},
								},
							},
						},
					},
				}

				vmicm := corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: vmic.Namespace,
						Name:      vmic.Name,
					},
					Data: map[string]string{
						"value": `<?xml version="1.0" encoding="UTF-8"?>
<Envelope xmlns="http://schemas.dmtf.org/ovf/envelope/1">
  <References>
    <File ovf:href="disk.vmdk" ovf:id="file1"/>
  </References>
  <DiskSection>
    <Info>Virtual disk information</Info>
    <Disk ovf:capacity="1024" ovf:diskId="vmdisk1" ovf:fileRef="file1"/>
  </DiskSection>
  <VirtualSystem>
    <Name>test-vm</Name>
    <VirtualHardwareSection>
      <System>
        <vssd:ElementName>Virtual Hardware Family</vssd:ElementName>
        <vssd:InstanceID>0</vssd:InstanceID>
        <vssd:VirtualSystemIdentifier>test-vm</vssd:VirtualSystemIdentifier>
        <vssd:VirtualSystemType>vmx-11</vssd:VirtualSystemType>
      </System>
    </VirtualHardwareSection>
  </VirtualSystem>
</Envelope>`,
					},
				}

				Expect(ctx.Client.Create(ctx, &vmic)).To(Succeed())
				Expect(ctx.Client.Create(ctx, &vmicm)).To(Succeed())
				Expect(ctx.Client.Status().Update(ctx, &vmic)).To(Succeed())

				vmDirName := string(vm.UID)
				vmDirPath := fmt.Sprintf("[%s] %s", ctx.Datastore.Name(), vmDirName)

				// Verify directory doesn't exist initially
				// DatastoreFileExists returns nil when file exists, os.ErrNotExist when it doesn't
				err := pkgutil.DatastoreFileExists(ctx, ctx.VCClient.Client, vmDirPath, ctx.Datacenter)
				Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue(), "Directory should not exist before creation")

				// Create ConfigSpec with 2 disks (mismatch)
				configSpec := vimtypes.VirtualMachineConfigSpec{
					Name: vm.Name,
					Files: &vimtypes.VirtualMachineFileInfo{
						VmPathName: fmt.Sprintf("[%s] %s/%s.vmx", ctx.Datastore.Name(), vmDirName, vm.Name),
					},
					DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
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
					},
				}

				createArgs := &vmlifecycle.CreateArgs{
					ConfigSpec:        configSpec,
					UseContentLibrary: true,
					Datastores: []vmlifecycle.DatastoreRef{
						{
							MoRef:                            ctx.Datastore.Reference(),
							TopLevelDirectoryCreateSupported: true, // TLD-supported
						},
					},
					DatacenterMoID: ctx.Datacenter.Reference().Value,
					DiskPaths:      []string{ctx.ContentLibraryItemDiskPath}, // Only 1 disk
					FilePaths:      []string{ctx.ContentLibraryItemNVRAMPath},
					ProviderItemID: ctx.ContentLibraryItemID,
				}

				// Attempt creation - should fail with "invalid disk count"
				_, err = vmlifecycle.CreateVirtualMachine(
					vmCtx,
					ctx.Client,
					ctx.RestClient,
					ctx.VCClient.Client,
					ctx.Finder,
					createArgs)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid disk count"))

				// Verify directory was cleaned up
				// DatastoreFileExists returns nil when file exists, os.ErrNotExist when it doesn't
				err = pkgutil.DatastoreFileExists(ctx, ctx.VCClient.Client, vmDirPath, ctx.Datacenter)
				Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue(), "Directory should be cleaned up after error (should not exist)")
			})
		})
	})
})
