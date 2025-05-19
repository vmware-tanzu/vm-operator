// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
)

var _ = Describe("ComparePCIDevices", func() {

	var (
		currentList, expectedList object.VirtualDeviceList
		deviceChanges             []vimtypes.BaseVirtualDeviceConfigSpec

		// Variables related to vGPU devices.
		backingInfo1, backingInfo2 *vimtypes.VirtualPCIPassthroughVmiopBackingInfo
		deviceKey1, deviceKey2     int32
		vGPUDevice1, vGPUDevice2   vimtypes.BaseVirtualDevice

		// Variables related to dynamicDirectPathIO devices.
		allowedDev1, allowedDev2                         vimtypes.VirtualPCIPassthroughAllowedDevice
		backingInfo3, backingInfo4                       *vimtypes.VirtualPCIPassthroughDynamicBackingInfo
		deviceKey3, deviceKey4                           int32
		dynamicDirectPathIODev1, dynamicDirectPathIODev2 vimtypes.BaseVirtualDevice
	)

	BeforeEach(func() {
		backingInfo1 = &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: "mockup-vmiop1"}
		backingInfo2 = &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{Vgpu: "mockup-vmiop2"}
		deviceKey1 = int32(-200)
		deviceKey2 = int32(-201)
		vGPUDevice1 = virtualmachine.CreatePCIPassThroughDevice(deviceKey1, backingInfo1)
		vGPUDevice2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey2, backingInfo2)

		allowedDev1 = vimtypes.VirtualPCIPassthroughAllowedDevice{
			VendorId: 1000,
			DeviceId: 100,
		}
		allowedDev2 = vimtypes.VirtualPCIPassthroughAllowedDevice{
			VendorId: 2000,
			DeviceId: 200,
		}
		backingInfo3 = &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
			AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{allowedDev1},
			CustomLabel:   "sampleLabel3",
		}
		backingInfo4 = &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
			AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{allowedDev2},
			CustomLabel:   "sampleLabel4",
		}
		deviceKey3 = int32(-202)
		deviceKey4 = int32(-203)
		dynamicDirectPathIODev1 = virtualmachine.CreatePCIPassThroughDevice(deviceKey3, backingInfo3)
		dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfo4)
	})

	JustBeforeEach(func() {
		deviceChanges = resize.ComparePCIDevices(expectedList, currentList)
	})

	AfterEach(func() {
		currentList = nil
		expectedList = nil
	})

	Context("No devices", func() {
		It("returns empty list", func() {
			Expect(deviceChanges).To(BeEmpty())
		})
	})

	Context("Adding vGPU and dynamicDirectPathIO devices with different backing info", func() {
		BeforeEach(func() {
			expectedList = append(expectedList, vGPUDevice1)
			expectedList = append(expectedList, vGPUDevice2)
			expectedList = append(expectedList, dynamicDirectPathIODev1)
			expectedList = append(expectedList, dynamicDirectPathIODev2)
		})

		It("returns add device changes", func() {
			Expect(deviceChanges).To(HaveLen(len(expectedList)))

			for idx, dev := range deviceChanges {
				configSpec := dev.GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			}
		})
	})

	Context("Adding vGPU and dynamicDirectPathIO devices with same backing info", func() {
		BeforeEach(func() {
			expectedList = append(expectedList, vGPUDevice1)
			// Creating a vGPUDevice with same backingInfo1 but different deviceKey.
			vGPUDevice2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey2, backingInfo1)
			expectedList = append(expectedList, vGPUDevice2)
			expectedList = append(expectedList, dynamicDirectPathIODev1)
			// Creating a dynamicDirectPathIO device with same backingInfo3 but different deviceKey.
			dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfo3)
			expectedList = append(expectedList, dynamicDirectPathIODev2)
		})

		It("return add device changes", func() {
			Expect(deviceChanges).To(HaveLen(len(expectedList)))

			for idx, dev := range deviceChanges {
				configSpec := dev.GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			}
		})
	})

	Context("Expected and current lists have DDPIO devices with different custom labels", func() {
		BeforeEach(func() {
			expectedList = []vimtypes.BaseVirtualDevice{dynamicDirectPathIODev1}
			// Creating a dynamicDirectPathIO device with same backing info except for the custom label.
			backingInfoDiffCustomLabel := &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
				AllowedDevice: backingInfo3.AllowedDevice,
				CustomLabel:   "DifferentLabel",
			}
			dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfoDiffCustomLabel)
			currentList = []vimtypes.BaseVirtualDevice{dynamicDirectPathIODev2}
		})

		It("returns expected add and remove device changes", func() {
			Expect(deviceChanges).To(HaveLen(2))

			configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

			configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
		})
	})

	Context("Expected and current pciDevices list have different devices", func() {
		BeforeEach(func() {
			currentList = append(currentList, vGPUDevice1)
			expectedList = append(expectedList, vGPUDevice2)
			currentList = append(currentList, dynamicDirectPathIODev1)
			expectedList = append(expectedList, dynamicDirectPathIODev2)
		})

		It("returns expected add and remove device changes", func() {
			Expect(deviceChanges).To(HaveLen(4))

			for i := 0; i < 2; i++ {
				configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[i].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
			}

			for i := 2; i < 4; i++ {
				configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[i-2].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			}
		})
	})

	Context("Expected and current pciDevices list have same devices", func() {
		BeforeEach(func() {
			currentList = append(currentList, vGPUDevice1)
			expectedList = append(expectedList, vGPUDevice1)
			currentList = append(currentList, dynamicDirectPathIODev1)
			expectedList = append(expectedList, dynamicDirectPathIODev1)
		})

		It("returns empty list", func() {
			Expect(deviceChanges).To(BeEmpty())
		})
	})
})
