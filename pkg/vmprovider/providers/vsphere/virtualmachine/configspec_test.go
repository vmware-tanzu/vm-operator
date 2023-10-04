// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CreateConfigSpec", func() {
	const vmName = "dummy-vm"

	var (
		vmClassSpec     *vmopv1.VirtualMachineClassSpec
		minCPUFreq      uint64
		firmware        string
		configSpec      *vimtypes.VirtualMachineConfigSpec
		classConfigSpec *vimtypes.VirtualMachineConfigSpec
		err             error
	)

	BeforeEach(func() {
		vmClass := builder.DummyVirtualMachineClass()
		vmClassSpec = &vmClass.Spec
		minCPUFreq = 2500
		firmware = "efi"
	})

	It("Basic ConfigSpec assertions", func() {
		configSpec = virtualmachine.CreateConfigSpec(
			vmName,
			vmClassSpec,
			minCPUFreq,
			firmware,
			nil)
		Expect(configSpec).ToNot(BeNil())
		Expect(err).To(BeNil())
		Expect(configSpec.Name).To(Equal(vmName))
		Expect(configSpec.Annotation).ToNot(BeEmpty())
		Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
		Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
		Expect(configSpec.CpuAllocation).ToNot(BeNil())
		Expect(configSpec.MemoryAllocation).ToNot(BeNil())
		Expect(configSpec.Firmware).To(Equal(firmware))
	})

	Context("Use VM Class ConfigSpec", func() {
		BeforeEach(func() {
			classConfigSpec = &vimtypes.VirtualMachineConfigSpec{
				Name:       "dont-use-this-dummy-VM",
				Annotation: "test-annotation",
				DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
					&vimtypes.VirtualDeviceConfigSpec{
						Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
						Device: &vimtypes.VirtualE1000{
							VirtualEthernetCard: vimtypes.VirtualEthernetCard{
								VirtualDevice: vimtypes.VirtualDevice{
									Key: 4000,
								},
							},
						},
					},
				},
				Firmware: "bios",
			}
		})

		JustBeforeEach(func() {
			configSpec = virtualmachine.CreateConfigSpec(
				vmName,
				vmClassSpec,
				minCPUFreq,
				firmware,
				classConfigSpec)
			Expect(configSpec).ToNot(BeNil())
		})

		It("Returns expected config spec", func() {
			Expect(configSpec.Name).To(Equal(vmName))
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).To(Equal("test-annotation"))
			Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
			Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
			Expect(configSpec.CpuAllocation).ToNot(BeNil())
			Expect(configSpec.MemoryAllocation).ToNot(BeNil())
			Expect(configSpec.Firmware).To(Equal(firmware))
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimtypes.VirtualE1000)
			Expect(ok).To(BeTrue())

		})

		When("image firmware is empty", func() {
			BeforeEach(func() {
				firmware = ""
			})

			It("config spec has the firmware from the class", func() {
				Expect(configSpec.Firmware).ToNot(Equal(firmware))
				Expect(configSpec.Firmware).To(Equal(classConfigSpec.Firmware))
			})
		})
	})
})

var _ = Describe("CreateConfigSpecForPlacement", func() {

	var (
		vmCtx               context.VirtualMachineContext
		vmClassSpec         *vmopv1.VirtualMachineClassSpec
		minCPUFreq          uint64
		firmware            string
		storageClassesToIDs map[string]string
		configSpec          *vimtypes.VirtualMachineConfigSpec
		classConfigSpec     *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		vmClass := builder.DummyVirtualMachineClass()
		vmClassSpec = &vmClass.Spec
		minCPUFreq = 2500
		firmware = "efi"
		storageClassesToIDs = map[string]string{}

		vm := builder.DummyVirtualMachine()
		vmCtx = context.VirtualMachineContext{
			Context: goctx.Background(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		configSpec = virtualmachine.CreateConfigSpecForPlacement(
			vmCtx,
			vmClassSpec,
			minCPUFreq,
			storageClassesToIDs,
			firmware,
			classConfigSpec)
		Expect(configSpec).ToNot(BeNil())
	})

	Context("When InstanceStorage is configured", func() {
		const storagePolicyID = "storage-id-42"
		var oldIsInstanceStorageFSSEnabled func() bool

		BeforeEach(func() {
			oldIsInstanceStorageFSSEnabled = lib.IsInstanceStorageFSSEnabled
			lib.IsInstanceStorageFSSEnabled = func() bool { return true }

			builder.AddDummyInstanceStorageVolume(vmCtx.VM)
			storageClassesToIDs[builder.DummyStorageClassName] = storagePolicyID
			classConfigSpec = nil
		})

		AfterEach(func() {
			lib.IsInstanceStorageFSSEnabled = oldIsInstanceStorageFSSEnabled
		})

		It("ConfigSpec contains expected InstanceStorage devices", func() {
			Expect(configSpec.DeviceChange).To(HaveLen(3))
			assertInstanceStorageDeviceChange(configSpec.DeviceChange[1], 256, storagePolicyID)
			assertInstanceStorageDeviceChange(configSpec.DeviceChange[2], 512, storagePolicyID)
		})
	})

	Context("when DaynDate FSS is enabled", func() {
		var oldDaynDateFSSFn func() bool

		BeforeEach(func() {
			oldDaynDateFSSFn = lib.IsVMClassAsConfigFSSDaynDateEnabled
			lib.IsVMClassAsConfigFSSDaynDateEnabled = func() bool { return true }
		})

		AfterEach(func() {
			lib.IsVMClassAsConfigFSSDaynDateEnabled = oldDaynDateFSSFn
		})

		Context("When class config spec is not nil", func() {
			BeforeEach(func() {
				classConfigSpec = &vimtypes.VirtualMachineConfigSpec{
					Name:       "dummy-VM",
					Annotation: "test-annotation",
					DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
							Device: &vimtypes.VirtualPCIPassthrough{
								VirtualDevice: vimtypes.VirtualDevice{
									Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
										Vgpu: "SampleProfile2",
									},
								},
							},
						},
					},
				}
			})

			AfterEach(func() {
				classConfigSpec = nil
			})

			It("Placement ConfigSpec contains expected field set sans ethernet device from class config spec", func() {
				Expect(configSpec.Annotation).ToNot(BeEmpty())
				Expect(configSpec.Annotation).To(Equal(classConfigSpec.Annotation))
				Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
				Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
				Expect(configSpec.CpuAllocation).ToNot(BeNil())
				Expect(configSpec.MemoryAllocation).ToNot(BeNil())
				Expect(configSpec.Firmware).To(Equal(firmware))
				Expect(configSpec.DeviceChange).To(HaveLen(2))
				dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
				_, ok := dSpec.Device.(*vimtypes.VirtualPCIPassthrough)
				Expect(ok).To(BeTrue())
				dSpec1 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
				_, ok = dSpec1.Device.(*vimtypes.VirtualDisk)
				Expect(ok).To(BeTrue())
			})
		})
	})

	Context("when Class specifies GPUs in Hardware", func() {
		BeforeEach(func() {
			vmClassSpec.Hardware.Devices.VGPUDevices = []vmopv1.VGPUDevice{{
				ProfileName: "createplacementspec-profile",
			}}

			vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices = []vmopv1.DynamicDirectPathIODevice{{
				VendorID:    20,
				DeviceID:    30,
				CustomLabel: "createplacementspec-label",
			}}
		})

		AfterEach(func() {
			vmClassSpec.Hardware.Devices.VGPUDevices = nil
			vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices = nil
		})

		It("Placement ConfigSpec contains the GPU from the VM Class", func() {
			Expect(configSpec.DeviceChange).To(HaveLen(3)) // one dummy disk + 2 Pass through devices

			dspec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dspec.Device.(*vimtypes.VirtualDisk)
			Expect(ok).To(BeTrue())

			dSpec1 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			dev1, ok := dSpec1.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			pciDev1 := dev1.GetVirtualDevice()
			pciBacking1, ok1 := pciDev1.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
			Expect(ok1).To(BeTrue())
			Expect(pciBacking1.Vgpu).To(Equal(vmClassSpec.Hardware.Devices.VGPUDevices[0].ProfileName))

			dSpec2 := configSpec.DeviceChange[2].GetVirtualDeviceConfigSpec()
			dev2, ok2 := dSpec2.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok2).To(BeTrue())
			pciDev2 := dev2.GetVirtualDevice()
			pciBacking2, ok2 := pciDev2.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
			Expect(ok2).To(BeTrue())
			Expect(pciBacking2.AllowedDevice[0].DeviceId).To(BeEquivalentTo(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].DeviceID))
			Expect(pciBacking2.AllowedDevice[0].VendorId).To(BeEquivalentTo(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].VendorID))
			Expect(pciBacking2.CustomLabel).To(Equal(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices[0].CustomLabel))
		})
	})

})

func assertInstanceStorageDeviceChange(
	deviceChange vimtypes.BaseVirtualDeviceConfigSpec,
	expectedSizeGB int,
	expectedStoragePolicyID string) {

	dc := deviceChange.GetVirtualDeviceConfigSpec()
	Expect(dc.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
	Expect(dc.FileOperation).To(Equal(vimtypes.VirtualDeviceConfigSpecFileOperationCreate))

	dev, ok := dc.Device.(*vimtypes.VirtualDisk)
	Expect(ok).To(BeTrue())
	Expect(dev.CapacityInBytes).To(BeEquivalentTo(expectedSizeGB * 1024 * 1024 * 1024))

	Expect(dc.Profile).To(HaveLen(1))
	profile, ok := dc.Profile[0].(*vimtypes.VirtualMachineDefinedProfileSpec)
	Expect(ok).To(BeTrue())
	Expect(profile.ProfileId).To(Equal(expectedStoragePolicyID))
}
