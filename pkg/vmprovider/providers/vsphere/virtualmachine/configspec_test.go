// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	goctx "context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

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
		configSpec      *vimtypes.VirtualMachineConfigSpec
		classConfigSpec *vimtypes.VirtualMachineConfigSpec
		err             error
	)

	BeforeEach(func() {
		vmClass := builder.DummyVirtualMachineClass()
		vmClassSpec = &vmClass.Spec
		minCPUFreq = 2500
	})

	It("Basic ConfigSpec assertions", func() {
		configSpec = virtualmachine.CreateConfigSpec(
			vmName,
			vmClassSpec,
			minCPUFreq,
			nil)
		Expect(configSpec).ToNot(BeNil())
		Expect(err).To(BeNil())
		Expect(configSpec.Name).To(Equal(vmName))
		Expect(configSpec.Annotation).ToNot(BeEmpty())
		Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
		Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
		Expect(configSpec.CpuAllocation).ToNot(BeNil())
		Expect(configSpec.MemoryAllocation).ToNot(BeNil())
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
			}
		})

		JustBeforeEach(func() {
			configSpec = virtualmachine.CreateConfigSpec(
				vmName,
				vmClassSpec,
				minCPUFreq,
				classConfigSpec)
			Expect(configSpec).ToNot(BeNil())
		})

		It("Returns expected config spec", func() {
			Expect(configSpec.Name).To(Equal(vmName))
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).ToNot(Equal("test-annotation"))
			Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
			Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
			Expect(configSpec.CpuAllocation).ToNot(BeNil())
			Expect(configSpec.MemoryAllocation).ToNot(BeNil())
			Expect(configSpec.DeviceChange).To(HaveLen(1))
			dSpec := configSpec.DeviceChange[0].GetVirtualDeviceConfigSpec()
			_, ok := dSpec.Device.(*vimtypes.VirtualE1000)
			Expect(ok).To(BeTrue())

		})
	})
})

var _ = Describe("CreateConfigSpecForPlacement", func() {

	var (
		vmCtx               context.VirtualMachineContext
		vmClassSpec         *vmopv1.VirtualMachineClassSpec
		minCPUFreq          uint64
		storageClassesToIDs map[string]string
		configSpec          *vimtypes.VirtualMachineConfigSpec
		classConfigSpec     *vimtypes.VirtualMachineConfigSpec
	)

	BeforeEach(func() {
		vmClass := builder.DummyVirtualMachineClass()
		vmClassSpec = &vmClass.Spec
		minCPUFreq = 2500
		storageClassesToIDs = map[string]string{}

		vm := builder.DummyVirtualMachine()
		vmCtx = context.VirtualMachineContext{
			Context: goctx.Background(),
			Logger:  logr.New(logf.NullLogSink{}),
			VM:      vm,
		}
	})

	JustBeforeEach(func() {
		configSpec = virtualmachine.CreateConfigSpecForPlacement(
			vmCtx,
			vmClassSpec,
			minCPUFreq,
			storageClassesToIDs,
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

		It("Placement ConfigSpec contains expected field set sans ethernet device from class config spec", func() {
			Expect(configSpec.Annotation).ToNot(BeEmpty())
			Expect(configSpec.Annotation).ToNot(Equal("test-annotation"))
			Expect(configSpec.NumCPUs).To(BeEquivalentTo(vmClassSpec.Hardware.Cpus))
			Expect(configSpec.MemoryMB).To(BeEquivalentTo(4 * 1024))
			Expect(configSpec.CpuAllocation).ToNot(BeNil())
			Expect(configSpec.MemoryAllocation).ToNot(BeNil())
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
