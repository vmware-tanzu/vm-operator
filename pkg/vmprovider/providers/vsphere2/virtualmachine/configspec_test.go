// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	goctx "context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("CreateConfigSpec", func() {
	const vmName = "dummy-vm"

	var (
		vm              *vmopv1.VirtualMachine
		vmCtx           context.VirtualMachineContextA2
		vmClassSpec     *vmopv1.VirtualMachineClassSpec
		vmImageStatus   *vmopv1.VirtualMachineImageStatus
		minCPUFreq      uint64
		configSpec      *vimtypes.VirtualMachineConfigSpec
		classConfigSpec *vimtypes.VirtualMachineConfigSpec
		err             error
	)

	BeforeEach(func() {
		vmClass := builder.DummyVirtualMachineClassA2()
		vmClassSpec = &vmClass.Spec
		vmImageStatus = &vmopv1.VirtualMachineImageStatus{Firmware: "efi"}
		minCPUFreq = 2500

		vm = builder.DummyVirtualMachineA2()
		vm.Name = vmName
		vmCtx = context.VirtualMachineContextA2{
			Context: goctx.Background(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	It("Basic ConfigSpec assertions", func() {
		configSpec = virtualmachine.CreateConfigSpec(
			vmCtx,
			nil,
			vmClassSpec,
			vmImageStatus,
			minCPUFreq)

		Expect(configSpec).ToNot(BeNil())
		Expect(err).To(BeNil())
		Expect(configSpec.Name).To(Equal(vmName))
		Expect(configSpec.Annotation).ToNot(BeEmpty())
		Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
		Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
		Expect(configSpec.CpuAllocation).ToNot(BeNil())
		Expect(configSpec.MemoryAllocation).ToNot(BeNil())
		Expect(configSpec.Firmware).To(Equal(vmImageStatus.Firmware))
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
				vmCtx,
				classConfigSpec,
				vmClassSpec,
				vmImageStatus,
				minCPUFreq)
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
			Expect(configSpec.Firmware).To(Equal(vmImageStatus.Firmware))
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimtypes.VirtualE1000)
			Expect(ok).To(BeTrue())
		})

		When("Image firmware is empty", func() {
			BeforeEach(func() {
				vmImageStatus = &vmopv1.VirtualMachineImageStatus{}
			})

			It("config spec has the firmware from the class", func() {
				Expect(configSpec.Firmware).ToNot(Equal(vmImageStatus.Firmware))
				Expect(configSpec.Firmware).To(Equal(classConfigSpec.Firmware))
			})
		})

		When("vm has a valid firmware override annotation", func() {
			BeforeEach(func() {
				vm.Annotations[constants.FirmwareOverrideAnnotation] = "efi"
			})

			It("config spec has overridden firmware annotation", func() {
				Expect(configSpec.Firmware).To(Equal(vm.Annotations[constants.FirmwareOverrideAnnotation]))
			})
		})

		When("vm has an invalid firmware override annotation", func() {
			BeforeEach(func() {
				vm.Annotations[constants.FirmwareOverrideAnnotation] = "foo"
			})

			It("config spec doesn't have the invalid val", func() {
				Expect(configSpec.Firmware).ToNot(Equal(vm.Annotations[constants.FirmwareOverrideAnnotation]))
			})
		})
	})
})

var _ = Describe("CreateConfigSpecForPlacement", func() {

	var (
		vmCtx               context.VirtualMachineContextA2
		storageClassesToIDs map[string]string
		baseConfigSpec      *vimtypes.VirtualMachineConfigSpec
		configSpec          *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		baseConfigSpec = &vimtypes.VirtualMachineConfigSpec{}
		storageClassesToIDs = map[string]string{}

		vm := builder.DummyVirtualMachineA2()
		vmCtx = context.VirtualMachineContextA2{
			Context: goctx.Background(),
			Logger:  suite.GetLogger().WithValues("vmName", vm.GetName()),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		configSpec = virtualmachine.CreateConfigSpecForPlacement(
			vmCtx,
			baseConfigSpec,
			storageClassesToIDs)
		Expect(configSpec).ToNot(BeNil())
	})

	Context("Returns expected ConfigSpec", func() {
		BeforeEach(func() {
			baseConfigSpec = &vimtypes.VirtualMachineConfigSpec{
				Name:       "dummy-VM",
				Annotation: "test-annotation",
				NumCPUs:    42,
				MemoryMB:   4096,
				Firmware:   "secret-sauce",
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

		It("Placement ConfigSpec contains expected field set sans ethernet device from class config spec", func() {
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).To(Equal(baseConfigSpec.Annotation))
			Expect(configSpec.NumCPUs).To(Equal(baseConfigSpec.NumCPUs))
			Expect(configSpec.MemoryMB).To(Equal(baseConfigSpec.MemoryMB))
			Expect(configSpec.CpuAllocation).To(Equal(baseConfigSpec.CpuAllocation))
			Expect(configSpec.MemoryAllocation).To(Equal(baseConfigSpec.MemoryAllocation))
			Expect(configSpec.Firmware).To(Equal(baseConfigSpec.Firmware))

			Expect(configSpec.DeviceChange).To(HaveLen(2))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			dSpec1 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
			_, ok = dSpec1.Device.(*vimtypes.VirtualDisk)
			Expect(ok).To(BeTrue())
		})
	})

	Context("When InstanceStorage is configured", func() {
		const storagePolicyID = "storage-id-42"
		var oldIsInstanceStorageFSSEnabled func() bool

		BeforeEach(func() {
			oldIsInstanceStorageFSSEnabled = lib.IsInstanceStorageFSSEnabled
			lib.IsInstanceStorageFSSEnabled = func() bool { return true }

			builder.AddDummyInstanceStorageVolumeA2(vmCtx.VM)
			storageClassesToIDs[builder.DummyStorageClassName] = storagePolicyID
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
})

var _ = Describe("ConfigSpecFromVMClassDevices", func() {

	var (
		vmClassSpec *vmopv1.VirtualMachineClassSpec
		configSpec  *vimtypes.VirtualMachineConfigSpec
	)

	Context("when Class specifies GPU/DDPIO in Hardware", func() {

		BeforeEach(func() {
			vmClassSpec = &vmopv1.VirtualMachineClassSpec{}

			vmClassSpec.Hardware.Devices.VGPUDevices = []vmopv1.VGPUDevice{{
				ProfileName: "createplacementspec-profile",
			}}

			vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices = []vmopv1.DynamicDirectPathIODevice{{
				VendorID:    20,
				DeviceID:    30,
				CustomLabel: "createplacementspec-label",
			}}
		})

		JustBeforeEach(func() {
			configSpec = virtualmachine.ConfigSpecFromVMClassDevices(vmClassSpec)
		})

		It("Returns expected ConfigSpec", func() {
			Expect(configSpec).ToNot(BeNil())
			Expect(configSpec.DeviceChange).To(HaveLen(2)) // One each for GPU an DDPIO above

			dSpec1 := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			dev1, ok := dSpec1.Device.(*vimtypes.VirtualPCIPassthrough)
			Expect(ok).To(BeTrue())
			pciDev1 := dev1.GetVirtualDevice()
			pciBacking1, ok1 := pciDev1.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
			Expect(ok1).To(BeTrue())
			Expect(pciBacking1.Vgpu).To(Equal(vmClassSpec.Hardware.Devices.VGPUDevices[0].ProfileName))

			dSpec2 := configSpec.DeviceChange[1].GetVirtualDeviceConfigSpec()
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
