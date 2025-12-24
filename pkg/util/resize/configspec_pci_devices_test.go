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
		err                       error

		// Variables related to vGPU devices.
		backingInfo1, backingInfo2 *vimtypes.VirtualPCIPassthroughVmiopBackingInfo
		deviceKey1, deviceKey2     int32
		vGPUDevice1, vGPUDevice2   vimtypes.BaseVirtualDevice

		// Variables related to dynamicDirectPathIO devices.
		allowedDev1, allowedDev2                         vimtypes.VirtualPCIPassthroughAllowedDevice
		backingInfo3, backingInfo4                       *vimtypes.VirtualPCIPassthroughDynamicBackingInfo
		deviceKey3, deviceKey4                           int32
		dynamicDirectPathIODev1, dynamicDirectPathIODev2 vimtypes.BaseVirtualDevice

		// Variables related to DVX devices.
		backingInfo5, backingInfo6 *vimtypes.VirtualPCIPassthroughDvxBackingInfo
		deviceKey5, deviceKey6     int32
		dvxDevice1, dvxDevice2     vimtypes.BaseVirtualDevice
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

		deviceKey5 = int32(-204)
		deviceKey6 = int32(-205)
		backingInfo5 = &vimtypes.VirtualPCIPassthroughDvxBackingInfo{
			DeviceClass: "hello",
			ConfigParams: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{
					Key:   "hello-key1",
					Value: "hello-val1",
				},
				&vimtypes.OptionValue{
					Key:   "hello-key2",
					Value: "hello-val2",
				},
			},
		}
		backingInfo6 = &vimtypes.VirtualPCIPassthroughDvxBackingInfo{
			DeviceClass: "world",
			ConfigParams: []vimtypes.BaseOptionValue{
				&vimtypes.OptionValue{
					Key:   "world-key1",
					Value: "world-val1",
				},
				&vimtypes.OptionValue{
					Key:   "world-key2",
					Value: "world-val2",
				},
			},
		}
		dvxDevice1 = virtualmachine.CreatePCIPassThroughDevice(deviceKey5, backingInfo5)
		dvxDevice2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey6, backingInfo6)
	})

	JustBeforeEach(func() {
		deviceChanges = resize.ComparePCIDevices(expectedList, currentList)
	})

	AfterEach(func() {
		currentList = nil
		expectedList = nil
	})

	When("No devices", func() {
		It("returns empty list", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(deviceChanges).To(BeEmpty())
		})
	})

	When("Adding vGPU, dynamicDirectPathIO, and DVX devices with different backing info", func() {
		BeforeEach(func() {
			expectedList = append(expectedList, vGPUDevice1)
			expectedList = append(expectedList, vGPUDevice2)
			expectedList = append(expectedList, dynamicDirectPathIODev1)
			expectedList = append(expectedList, dynamicDirectPathIODev2)
			expectedList = append(expectedList, dvxDevice1)
			expectedList = append(expectedList, dvxDevice2)
		})

		It("Should return add device changes", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(len(deviceChanges)).To(Equal(len(expectedList)))

			for idx, dev := range deviceChanges {
				configSpec := dev.GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			}
		})
	})

	When("Adding vGPU, dynamicDirectPathIO, and DVX devices with same backing info", func() {
		BeforeEach(func() {
			expectedList = append(expectedList, vGPUDevice1)
			// Creating a vGPUDevice with same backingInfo1 but different deviceKey.
			vGPUDevice2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey2, backingInfo1)
			expectedList = append(expectedList, vGPUDevice2)
			expectedList = append(expectedList, dynamicDirectPathIODev1)
			// Creating a dynamicDirectPathIO device with same backingInfo3 but different deviceKey.
			dynamicDirectPathIODev2 = virtualmachine.CreatePCIPassThroughDevice(deviceKey4, backingInfo3)
			expectedList = append(expectedList, dynamicDirectPathIODev2)
			// Creating a DVX device with same backingInfo5 but different deviceKey.
			dvxDevice1 = virtualmachine.CreatePCIPassThroughDevice(-1000, backingInfo5)
			expectedList = append(expectedList, dvxDevice1)
		})

		It("Should return add device changes", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(len(deviceChanges)).To(Equal(len(expectedList)))

			for idx, dev := range deviceChanges {
				configSpec := dev.GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[idx].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			}
		})
	})

	When("Expected and current lists have DDPIO devices with different custom labels", func() {
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

		It("should return add and remove device changes", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(len(deviceChanges)).To(Equal(2))

			configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

			configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
		})
	})

	When("Expected and current lists have DVX devices with different device classes", func() {
		BeforeEach(func() {
			expectedList = []vimtypes.BaseVirtualDevice{dvxDevice1}
			backingInfo := &vimtypes.VirtualPCIPassthroughDvxBackingInfo{
				DeviceClass: "fubar",
				ConfigParams: []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   "hello-key1",
						Value: "hello-val1",
					},
					&vimtypes.OptionValue{
						Key:   "hello-key2",
						Value: "hello-val2",
					},
				},
			}
			newDevice := virtualmachine.CreatePCIPassThroughDevice(deviceKey5, backingInfo)
			currentList = []vimtypes.BaseVirtualDevice{newDevice}
		})

		It("should return add and remove device changes", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(len(deviceChanges)).To(Equal(2))

			configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

			configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
		})
	})

	When("Expected and current lists have DVX devices with different config params", func() {
		BeforeEach(func() {
			expectedList = []vimtypes.BaseVirtualDevice{dvxDevice1}
			backingInfo := &vimtypes.VirtualPCIPassthroughDvxBackingInfo{
				DeviceClass: "hello",
				ConfigParams: []vimtypes.BaseOptionValue{
					&vimtypes.OptionValue{
						Key:   "hello-key1",
						Value: "hello-val1",
					},
				},
			}
			newDevice := virtualmachine.CreatePCIPassThroughDevice(deviceKey5, backingInfo)
			currentList = []vimtypes.BaseVirtualDevice{newDevice}
		})

		It("should return add and remove device changes", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(len(deviceChanges)).To(Equal(2))

			configSpec := deviceChanges[0].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))

			configSpec = deviceChanges[1].GetVirtualDeviceConfigSpec()
			Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[0].GetVirtualDevice().Key))
			Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
		})
	})

	When("Expected and current pciDevices list have different devices", func() {
		BeforeEach(func() {
			currentList = append(currentList, vGPUDevice1)
			expectedList = append(expectedList, vGPUDevice2)
			currentList = append(currentList, dynamicDirectPathIODev1)
			expectedList = append(expectedList, dynamicDirectPathIODev2)
			currentList = append(currentList, dvxDevice1)
			expectedList = append(expectedList, dvxDevice2)
		})

		It("Should return add and remove device changes", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(len(deviceChanges)).To(Equal(6))

			for i := 0; i < 3; i++ {
				configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(currentList[i].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationRemove))
			}

			for i := 3; i < 6; i++ {
				configSpec := deviceChanges[i].GetVirtualDeviceConfigSpec()
				Expect(configSpec.Device.GetVirtualDevice().Key).To(Equal(expectedList[i-3].GetVirtualDevice().Key))
				Expect(configSpec.Operation).To(Equal(vimtypes.VirtualDeviceConfigSpecOperationAdd))
			}
		})
	})

	When("Expected and current pciDevices list have same devices", func() {
		BeforeEach(func() {
			currentList = append(currentList, vGPUDevice1)
			expectedList = append(expectedList, vGPUDevice1)
			currentList = append(currentList, dynamicDirectPathIODev1)
			expectedList = append(expectedList, dynamicDirectPathIODev1)
			currentList = append(currentList, dvxDevice1)
			expectedList = append(expectedList, dvxDevice1)
		})

		It("returns empty list", func() {
			Expect(err).ToNot(HaveOccurred())
			Expect(deviceChanges).To(BeEmpty())
		})
	})
})
