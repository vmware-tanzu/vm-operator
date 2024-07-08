// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package resize_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
)

var _ = Describe("CreateResizeConfigSpec Devices", func() {

	const (
		AddOp    = vimtypes.VirtualDeviceConfigSpecOperationAdd
		RemoveOp = vimtypes.VirtualDeviceConfigSpecOperationRemove
		EditOp   = vimtypes.VirtualDeviceConfigSpecOperationEdit
	)

	ctx := context.Background()
	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	DescribeTableSubtree("ConfigInfo.Hardware.Devices",
		func(device, otherDevice giveMeDeviceFn) {
			var (
				ci             vimtypes.VirtualMachineConfigInfo
				cs, expectedCS vimtypes.VirtualMachineConfigSpec
			)

			BeforeEach(func() {
				ci = vimtypes.VirtualMachineConfigInfo{}
				cs = vimtypes.VirtualMachineConfigSpec{}
				expectedCS = vimtypes.VirtualMachineConfigSpec{}
			})

			JustBeforeEach(func() {
				// Add another device so we can assert that it doesn't get removed.
				d := deviceOfType(&vimtypes.VirtualVmxnet3{})()
				ci.Hardware.Device = append(ci.Hardware.Device, d)

				actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
				Expect(err).ToNot(HaveOccurred())

				actualCSDC := actualCS.DeviceChange
				actualCS.DeviceChange = nil
				expectedCSDC := expectedCS.DeviceChange
				expectedCS.DeviceChange = nil

				// TBD: Might be easier to switch away from DeepEqual() here 'cause of the device Key annoyances.
				Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
				Expect(actualCSDC).To(ConsistOf(expectedCSDC))
			})

			Context("Adds Device", func() {
				BeforeEach(func() {
					cs.DeviceChange = append(cs.DeviceChange, devChangeEntry(AddOp, device(), -200))
					expectedCS.DeviceChange = append(expectedCS.DeviceChange, devChangeEntry(AddOp, device(), -200))
				})

				It("DoIt", func() {})
			})

			Context("Removes Device", func() {
				BeforeEach(func() {
					d := device()
					d.GetVirtualDevice().Key = 5000
					ci.Hardware.Device = append(ci.Hardware.Device, d)
					expectedCS.DeviceChange = append(expectedCS.DeviceChange, devChangeEntry(RemoveOp, device(), 5000))
				})

				It("DoIt", func() {})
			})

			Context("Keeps Device", func() {
				BeforeEach(func() {
					ci.Hardware.Device = append(ci.Hardware.Device, device())
					cs.DeviceChange = append(expectedCS.DeviceChange, devChangeEntry(AddOp, device()))
					// expectedCS.DeviceChange expected to be empty.
				})

				It("DoIt", func() {})
			})

			Context("Removes & Adds Devices", func() {
				BeforeEach(func() {
					if otherDevice == nil {
						Skip("Need second device to do both add and remove")
					}

					// Add existing device.
					d := device()
					d.GetVirtualDevice().Key = 100
					ci.Hardware.Device = append(ci.Hardware.Device, d)

					// Add new, desired device.
					cs.DeviceChange = append(cs.DeviceChange, devChangeEntry(AddOp, otherDevice(), -200))

					// Expect to remove existing and add new devices.
					expectedCS.DeviceChange = append(expectedCS.DeviceChange,
						devChangeEntry(RemoveOp, device(), 100),
						devChangeEntry(AddOp, otherDevice(), -200))
				})

				It("DoIt", func() {})
			})
		},

		Entry("vGPU",
			vGPUDevice("my-vgpu"),
			vGPUDevice("my-other-vgpu")),

		Entry("DDPIO #1",
			ddpioDevice("my-label,", 100, 101),
			ddpioDevice("my-other-label,", 100, 101)),
		Entry("DDPIO #2",
			ddpioDevice("my-label,", 100, 101),
			ddpioDevice("my-label,", 200, 101)),
		Entry("DDPIO #3",
			ddpioDevice("my-label,", 100, 101),
			ddpioDevice("my-label,", 100, 201)),

		Entry("SoundCard #1",
			deviceOfType(&vimtypes.VirtualEnsoniq1371{}),
			deviceOfType(&vimtypes.VirtualSoundBlaster16{})),
		Entry("SoundCard #2",
			deviceOfType(&vimtypes.VirtualSoundBlaster16{}),
			deviceOfType(&vimtypes.VirtualHdAudioCard{})),
		Entry("SoundCard #3",
			deviceOfType(&vimtypes.VirtualSoundBlaster16{}),
			deviceOfType(&vimtypes.VirtualHdAudioCard{})),

		Entry("USB Controller #1",
			deviceOfType(&vimtypes.VirtualUSBController{}),
			nil),

		Entry("USB XHCI Controller #1",
			deviceOfType(&vimtypes.VirtualUSBXHCIController{}),
			nil),
	)

	Context("ConfigInfo.Hardware.Devices Edits", func() {

		DescribeTable("Edits",
			func(curDev, expectedDev, editDev vimtypes.BaseVirtualDevice) {
				var (
					ci             vimtypes.VirtualMachineConfigInfo
					cs, expectedCS vimtypes.VirtualMachineConfigSpec
				)

				ci.Hardware.Device = append(ci.Hardware.Device, curDev)
				cs.DeviceChange = append(cs.DeviceChange, devChangeEntry(AddOp, expectedDev))
				expectedCS.DeviceChange = append(expectedCS.DeviceChange, devChangeEntry(EditOp, editDev))

				actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
				Expect(err).ToNot(HaveOccurred())

				actualCSDC := actualCS.DeviceChange
				actualCS.DeviceChange = nil
				expectedCSDC := expectedCS.DeviceChange
				expectedCS.DeviceChange = nil

				// TBD: Might be easier to switch away from DeepEqual() here 'cause of the device Key annoyances.
				Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
				Expect(actualCSDC).To(ConsistOf(expectedCSDC))
			},

			Entry("USB Controller #1",
				&vimtypes.VirtualUSBController{AutoConnectDevices: falsePtr},
				&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr},
				&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr}),
			Entry("USB Controller #2",
				&vimtypes.VirtualUSBController{EhciEnabled: falsePtr},
				&vimtypes.VirtualUSBController{EhciEnabled: truePtr},
				&vimtypes.VirtualUSBController{EhciEnabled: truePtr}),
			Entry("USB Controller #3",
				&vimtypes.VirtualUSBController{AutoConnectDevices: falsePtr, EhciEnabled: falsePtr},
				&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr, EhciEnabled: falsePtr},
				&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr, EhciEnabled: falsePtr}),

			Entry("USB XHCI Controller #1",
				&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: falsePtr},
				&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
				&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr}),
		)
	})
})

func devChangeEntry(
	op vimtypes.VirtualDeviceConfigSpecOperation, dev vimtypes.BaseVirtualDevice,
	devKey ...int32) vimtypes.BaseVirtualDeviceConfigSpec {

	if len(devKey) == 1 {
		dev.GetVirtualDevice().Key = devKey[0]
	}

	return &vimtypes.VirtualDeviceConfigSpec{
		Operation: op,
		Device:    dev,
	}
}

// We want to return a new instance each time so the same object isn't on both actual and expected lists.
type giveMeDeviceFn = func() vimtypes.BaseVirtualDevice

func deviceOfType(dev vimtypes.BaseVirtualDevice) giveMeDeviceFn {
	return func() vimtypes.BaseVirtualDevice {
		t := reflect.ValueOf(dev).Elem().Type()
		return (reflect.New(t).Elem().Addr()).Interface().(vimtypes.BaseVirtualDevice)
	}
}

func vGPUDevice(profileName string) giveMeDeviceFn {
	return func() vimtypes.BaseVirtualDevice {
		return &vimtypes.VirtualPCIPassthrough{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
					Vgpu: profileName,
				},
			},
		}
	}
}

func ddpioDevice(label string, vendorID, deviceID int32) giveMeDeviceFn {
	return func() vimtypes.BaseVirtualDevice {
		return &vimtypes.VirtualPCIPassthrough{
			VirtualDevice: vimtypes.VirtualDevice{
				Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
					CustomLabel: label,
					AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{
						{
							VendorId: vendorID,
							DeviceId: deviceID,
						},
					},
				},
			},
		}
	}
}
