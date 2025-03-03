// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package resize_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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
		func(device, otherDevice giveMeDeviceFn, isDefaultDev bool) {
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
				// Include the deprecated VMIROM device that we don't care about to
				// ensure it doesn't get removed.
				d := &vimtypes.VirtualMachineVMIROM{}
				ci.Hardware.Device = append(ci.Hardware.Device, d)

				actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
				Expect(err).ToNot(HaveOccurred())

				actualCSDC := actualCS.DeviceChange
				actualCS.DeviceChange = nil
				expectedCSDC := expectedCS.DeviceChange
				expectedCS.DeviceChange = nil

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
					if isDefaultDev {
						Skip("is a default device")
					}

					d := device()
					d.GetVirtualDevice().Key = 5000
					ci.Hardware.Device = append(ci.Hardware.Device, d)
					expectedCS.DeviceChange = append(expectedCS.DeviceChange, devChangeEntry(RemoveOp, device(), 5000))
				})

				It("DoIt", func() {})
			})

			Context("Does not remove or edit default device", func() {
				BeforeEach(func() {
					if !isDefaultDev {
						Skip("is not a default device")
					}

					d := device()
					d.GetVirtualDevice().Key = 5000
					ci.Hardware.Device = append(ci.Hardware.Device, d)
					// expectedCS.DeviceChange expected to be empty.
				})

				It("DoIt", func() {})
			})

			Context("Keeps Identical Device", func() {
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
						Skip("Need second device to do remove & add")
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

		Entry("VirtualUSBController #1",
			deviceOfType(&vimtypes.VirtualUSBController{}),
			nil,
			false),
		Entry("VirtualUSBXHCIController #1",
			deviceOfType(&vimtypes.VirtualUSBXHCIController{}),
			nil,
			false),
		Entry("VirtualMachineVMCIDevice #1",
			deviceOfType(&vimtypes.VirtualMachineVMCIDevice{}),
			nil,
			true),
		Entry("VirtualMachineVideoCard #1",
			deviceOfType(&vimtypes.VirtualMachineVideoCard{}),
			nil,
			true),
		Entry("VirtualParallelPort #1",
			deviceOfType(&vimtypes.VirtualParallelPort{}),
			nil,
			false),
		XEntry("VirtualPointingDevice #1",
			deviceOfType(&vimtypes.VirtualPointingDevice{}),
			nil,
			true),
		Entry("VirtualPrecisionClock #1",
			deviceOfType(&vimtypes.VirtualPrecisionClock{}),
			nil,
			false),
		Entry("VirtualSCSIPassthrough #1",
			deviceOfType(&vimtypes.VirtualSCSIPassthrough{}),
			nil,
			false),
		Entry("VirtualSerialPort #1",
			deviceOfType(&vimtypes.VirtualSerialPort{}),
			nil,
			false),
		Entry("SoundCard #1",
			deviceOfType(&vimtypes.VirtualEnsoniq1371{}),
			deviceOfType(&vimtypes.VirtualSoundBlaster16{}),
			false),
		Entry("SoundCard #2",
			deviceOfType(&vimtypes.VirtualSoundBlaster16{}),
			deviceOfType(&vimtypes.VirtualHdAudioCard{}),
			false),
		Entry("SoundCard #3",
			deviceOfType(&vimtypes.VirtualSoundBlaster16{}),
			deviceOfType(&vimtypes.VirtualHdAudioCard{}),
			false),
		Entry("VirtualTPM #1",
			deviceOfType(&vimtypes.VirtualTPM{}),
			nil,
			false),
		Entry("VirtualWDT #1",
			deviceOfType(&vimtypes.VirtualWDT{}),
			nil,
			false),

		Entry("vGPU",
			vGPUDevice("my-vgpu"),
			vGPUDevice("my-other-vgpu"),
			false),
		Entry("DDPIO #1",
			ddpioDevice("my-label,", 100, 101),
			ddpioDevice("my-other-label,", 100, 101),
			false),
		Entry("DDPIO #2",
			ddpioDevice("my-label,", 100, 101),
			ddpioDevice("my-label,", 200, 101),
			false),
		Entry("DDPIO #3",
			ddpioDevice("my-label,", 100, 101),
			ddpioDevice("my-label,", 100, 201),
			false),
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
				if editDev != nil {
					expectedCS.DeviceChange = append(expectedCS.DeviceChange, devChangeEntry(EditOp, editDev))
				}

				actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
				Expect(err).ToNot(HaveOccurred())

				actualCSDC := actualCS.DeviceChange
				actualCS.DeviceChange = nil
				expectedCSDC := expectedCS.DeviceChange
				expectedCS.DeviceChange = nil

				Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
				Expect(actualCSDC).To(ConsistOf(expectedCSDC))
			},

			// Just use one type to test zipVirtualDevicesOfType().
			Entry("USB Controller #1",
				&vimtypes.VirtualUSBController{AutoConnectDevices: falsePtr},
				&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr},
				&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr}),
			Entry("USB Controller #2",
				&vimtypes.VirtualUSBController{EhciEnabled: falsePtr},
				&vimtypes.VirtualUSBController{EhciEnabled: truePtr},
				&vimtypes.VirtualUSBController{EhciEnabled: truePtr}),
			Entry("USB Controller #3",
				&vimtypes.VirtualUSBController{EhciEnabled: truePtr},
				&vimtypes.VirtualUSBController{EhciEnabled: truePtr},
				nil),
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

var _ = Describe("Match Devices", func() {

	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	DescribeTable("MatchVirtualUSBController",
		func(expected, current, expectedEdit *vimtypes.VirtualUSBController) {
			current.Key = 42
			edit := resize.MatchVirtualUSBController(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No Change",
			&vimtypes.VirtualUSBController{},
			&vimtypes.VirtualUSBController{},
			nil,
		),
		Entry("#2 - AutoConnectDevices change",
			&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr},
			&vimtypes.VirtualUSBController{},
			&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr},
		),
		Entry("#3 - EhciEnabled change",
			&vimtypes.VirtualUSBController{EhciEnabled: falsePtr},
			&vimtypes.VirtualUSBController{EhciEnabled: truePtr},
			&vimtypes.VirtualUSBController{EhciEnabled: falsePtr},
		),
	)

	DescribeTable("MatchVirtualUSBXHCIController",
		func(expected, current, expectedEdit *vimtypes.VirtualUSBXHCIController) {
			current.Key = 42
			edit := resize.MatchVirtualUSBXHCIController(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualUSBXHCIController{},
			&vimtypes.VirtualUSBXHCIController{},
			nil,
		),
		Entry("#2 - No change",
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
			nil,
		),
		Entry("#3 - AutoConnectDevices set",
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
			&vimtypes.VirtualUSBXHCIController{},
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
		),
		Entry("#4 - AutoConnectDevices changed",
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: falsePtr},
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
		),
	)

	DescribeTable("MatchVirtualMachineVMCIDevice",
		func(expected, current, expectedEdit *vimtypes.VirtualMachineVMCIDevice) {
			current.Key = 42
			edit := resize.MatchVirtualMachineVMCIDevice(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{},
			nil,
		),
		Entry("#2 - No change",
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
			nil,
		),
		Entry("#3 - AllowUnrestrictedCommunication changed",
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
		),
		Entry("#4 - FilterEnable set",
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
		),
		Entry("#5 - FilterEnable changed",
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: falsePtr},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
		),
		Entry("#6 - FilterEnable not changed",
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
			nil,
		),
		Entry("#7 - FilterInfo not changed",
			&vimtypes.VirtualMachineVMCIDevice{
				FilterInfo: &vimtypes.VirtualMachineVMCIDeviceFilterInfo{
					Filters: []vimtypes.VirtualMachineVMCIDeviceFilterSpec{
						{
							Rank: 99,
						},
					},
				},
			},
			&vimtypes.VirtualMachineVMCIDevice{
				FilterInfo: &vimtypes.VirtualMachineVMCIDeviceFilterInfo{
					Filters: []vimtypes.VirtualMachineVMCIDeviceFilterSpec{
						{
							Rank: 99,
						},
					},
				},
			},
			nil,
		),
		Entry("#8 - FilterInfo set",
			&vimtypes.VirtualMachineVMCIDevice{
				FilterInfo: &vimtypes.VirtualMachineVMCIDeviceFilterInfo{
					Filters: []vimtypes.VirtualMachineVMCIDeviceFilterSpec{
						{
							Rank: 99,
						},
					},
				},
			},
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{
				FilterInfo: &vimtypes.VirtualMachineVMCIDeviceFilterInfo{
					Filters: []vimtypes.VirtualMachineVMCIDeviceFilterSpec{
						{
							Rank: 99,
						},
					},
				},
			},
		),
	)

	DescribeTable("MatchVirtualMachineVideoCard",
		func(expected, current, expectedEdit *vimtypes.VirtualMachineVideoCard) {
			current.Key = 42
			edit := resize.MatchVirtualMachineVideoCard(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualMachineVideoCard{},
			&vimtypes.VirtualMachineVideoCard{},
			nil,
		),
		Entry("#2 - No change",
			&vimtypes.VirtualMachineVideoCard{NumDisplays: 2},
			&vimtypes.VirtualMachineVideoCard{NumDisplays: 2},
			nil,
		),
		Entry("#3 - Fields set",
			&vimtypes.VirtualMachineVideoCard{
				VideoRamSizeInKB:       1,
				NumDisplays:            2,
				UseAutoDetect:          truePtr,
				Enable3DSupport:        truePtr,
				Use3dRenderer:          "yes",
				GraphicsMemorySizeInKB: 3,
			},
			&vimtypes.VirtualMachineVideoCard{},
			&vimtypes.VirtualMachineVideoCard{
				VideoRamSizeInKB:       1,
				NumDisplays:            2,
				UseAutoDetect:          truePtr,
				Enable3DSupport:        truePtr,
				Use3dRenderer:          "yes",
				GraphicsMemorySizeInKB: 3,
			},
		),
	)

	DescribeTable("MatchVirtualParallelPort",
		func(expected, current, expectedEdit *vimtypes.VirtualParallelPort) {
			current.Key = 42
			edit := resize.MatchVirtualParallelPort(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualParallelPort{},
			&vimtypes.VirtualParallelPort{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
			&vimtypes.VirtualParallelPort{},
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
		),
		Entry("#3 - Backing set",
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortFileBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "foo",
						},
					},
				},
			},
			&vimtypes.VirtualParallelPort{},
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortFileBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "foo",
						},
					},
				},
			},
		),
		Entry("#4 - Backing changed",
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortFileBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "new",
						},
					},
				},
			},
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortFileBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "old",
						},
					},
				},
			},
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortFileBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "new",
						},
					},
				},
			},
		),
		Entry("#5 - Backing changed",
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "new",
						},
					},
				},
			},
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortFileBackingInfo{
						VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
							FileName: "old",
						},
					},
				},
			},
			&vimtypes.VirtualParallelPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualParallelPortDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "new",
						},
					},
				},
			},
		),
	)

	DescribeTable("MatchVirtualPointingDevice",
		func(expected, current, expectedEdit *vimtypes.VirtualPointingDevice) {
			current.Key = 42
			edit := resize.MatchVirtualPointingDevice(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualPointingDevice{},
			&vimtypes.VirtualPointingDevice{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualPointingDevice{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPointingDeviceDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
			&vimtypes.VirtualPointingDevice{},
			&vimtypes.VirtualPointingDevice{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPointingDeviceDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
		),
		Entry("#3 - Backing not changed",
			&vimtypes.VirtualPointingDevice{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPointingDeviceDeviceBackingInfo{
						HostPointingDevice: "foo",
					},
				},
			},
			&vimtypes.VirtualPointingDevice{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPointingDeviceDeviceBackingInfo{
						HostPointingDevice: "foo",
					},
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualPrecisionClock",
		func(expected, current, expectedEdit *vimtypes.VirtualPrecisionClock) {
			current.Key = 42
			edit := resize.MatchVirtualPrecisionClock(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualPrecisionClock{},
			&vimtypes.VirtualPrecisionClock{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualPrecisionClock{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPrecisionClockSystemClockBackingInfo{
						Protocol: "foo",
					},
				},
			},
			&vimtypes.VirtualPrecisionClock{},
			&vimtypes.VirtualPrecisionClock{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPrecisionClockSystemClockBackingInfo{
						Protocol: "foo",
					},
				},
			},
		),
		Entry("#3 - Backing not changed",
			&vimtypes.VirtualPrecisionClock{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPrecisionClockSystemClockBackingInfo{
						Protocol: "foo",
					},
				},
			},
			&vimtypes.VirtualPrecisionClock{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualPrecisionClockSystemClockBackingInfo{
						Protocol: "foo",
					},
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualSCSIPassthrough",
		func(expected, current, expectedEdit *vimtypes.VirtualSCSIPassthrough) {
			current.Key = 42
			edit := resize.MatchVirtualSCSIPassthrough(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualSCSIPassthrough{},
			&vimtypes.VirtualSCSIPassthrough{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualSCSIPassthrough{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
			&vimtypes.VirtualSCSIPassthrough{},
			&vimtypes.VirtualSCSIPassthrough{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
		),
		Entry("#3 - Backing not changed",
			&vimtypes.VirtualSCSIPassthrough{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
			&vimtypes.VirtualSCSIPassthrough{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSCSIPassthroughDeviceBackingInfo{
						VirtualDeviceDeviceBackingInfo: vimtypes.VirtualDeviceDeviceBackingInfo{
							DeviceName: "foo",
						},
					},
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualSerialPort",
		func(expected, current, expectedEdit *vimtypes.VirtualSerialPort) {
			current.Key = 42
			edit := resize.MatchVirtualSerialPort(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortDeviceBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortDeviceBackingInfo{},
				},
			},
		),
		Entry("#3 - Backing set",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortFileBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortFileBackingInfo{},
				},
			},
		),
		Entry("#4 - Backing set",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortPipeBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortPipeBackingInfo{},
				},
			},
		),
		Entry("#5 - Backing set",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortThinPrintBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortThinPrintBackingInfo{},
				},
			},
		),
		Entry("#6 - Backing set",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
		),
		Entry("#7 - Backing changed",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortDeviceBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortDeviceBackingInfo{},
				},
			},
		),
		Entry("#8 - Backing changed",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortFileBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortFileBackingInfo{},
				},
			},
		),
		Entry("#9 - Backing changed",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortPipeBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortPipeBackingInfo{},
				},
			},
		),
		Entry("#10 - Backing changed",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortThinPrintBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortThinPrintBackingInfo{},
				},
			},
		),
		Entry("#11 - Backing changed",
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortThinPrintBackingInfo{},
				},
			},
			&vimtypes.VirtualSerialPort{
				VirtualDevice: vimtypes.VirtualDevice{
					Backing: &vimtypes.VirtualSerialPortURIBackingInfo{},
				},
			},
		),
	)

	DescribeTable("MatchVirtualEnsoniq1371",
		func(expected, current, expectedEdit *vimtypes.VirtualEnsoniq1371) {
			current.Key = 42
			edit := resize.MatchVirtualEnsoniq1371(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualEnsoniq1371{},
			&vimtypes.VirtualEnsoniq1371{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualEnsoniq1371{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			&vimtypes.VirtualEnsoniq1371{},
			&vimtypes.VirtualEnsoniq1371{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
		),
		Entry("#3 - No backing change",
			&vimtypes.VirtualEnsoniq1371{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			&vimtypes.VirtualEnsoniq1371{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualHdAudioCard",
		func(expected, current, expectedEdit *vimtypes.VirtualHdAudioCard) {
			current.Key = 42
			edit := resize.MatchVirtualHdAudioCard(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualHdAudioCard{},
			&vimtypes.VirtualHdAudioCard{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualHdAudioCard{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			&vimtypes.VirtualHdAudioCard{},
			&vimtypes.VirtualHdAudioCard{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
		),
		Entry("#3 - No backing change",
			&vimtypes.VirtualHdAudioCard{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			&vimtypes.VirtualHdAudioCard{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualSoundBlaster16",
		func(expected, current, expectedEdit *vimtypes.VirtualSoundBlaster16) {
			current.Key = 42
			edit := resize.MatchVirtualSoundBlaster16(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualSoundBlaster16{},
			&vimtypes.VirtualSoundBlaster16{},
			nil,
		),
		Entry("#2 - Backing set",
			&vimtypes.VirtualSoundBlaster16{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			&vimtypes.VirtualSoundBlaster16{},
			&vimtypes.VirtualSoundBlaster16{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
		),
		Entry("#3 - No backing change",
			&vimtypes.VirtualSoundBlaster16{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			&vimtypes.VirtualSoundBlaster16{
				VirtualSoundCard: vimtypes.VirtualSoundCard{
					VirtualDevice: vimtypes.VirtualDevice{
						Backing: &vimtypes.VirtualSoundCardDeviceBackingInfo{},
					},
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualTPM",
		func(expected, current, expectedEdit *vimtypes.VirtualTPM) {
			current.Key = 42
			edit := resize.MatchVirtualTPM(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualTPM{},
			&vimtypes.VirtualTPM{},
			nil,
		),
		Entry("#2 - EndorsementKeyCertificateSigningRequest set",
			&vimtypes.VirtualTPM{
				EndorsementKeyCertificateSigningRequest: [][]byte{
					[]byte("foo"),
				},
			},
			&vimtypes.VirtualTPM{},
			&vimtypes.VirtualTPM{
				EndorsementKeyCertificateSigningRequest: [][]byte{
					[]byte("foo"),
				},
			},
		),
		Entry("#3 - EndorsementKeyCertificate set",
			&vimtypes.VirtualTPM{
				EndorsementKeyCertificate: [][]byte{
					[]byte("bar"),
				},
			},
			&vimtypes.VirtualTPM{},
			&vimtypes.VirtualTPM{
				EndorsementKeyCertificate: [][]byte{
					[]byte("bar"),
				},
			},
		),
		Entry("#4 - EndorsementKeyCertificateSigningRequest not changed",
			&vimtypes.VirtualTPM{
				EndorsementKeyCertificateSigningRequest: [][]byte{
					[]byte("foo"),
				},
				EndorsementKeyCertificate: [][]byte{
					[]byte("bar"),
				},
			},
			&vimtypes.VirtualTPM{
				EndorsementKeyCertificateSigningRequest: [][]byte{
					[]byte("foo"),
				},
				EndorsementKeyCertificate: [][]byte{
					[]byte("bar"),
				},
			},
			nil,
		),
	)

	DescribeTable("MatchVirtualWDT",
		func(expected, current, expectedEdit *vimtypes.VirtualWDT) {
			current.Key = 42
			edit := resize.MatchVirtualWDT(expected, current)
			if expectedEdit != nil {
				expectedEdit.Key = 42
				Expect(edit).To(Equal(expectedEdit))
			} else {
				Expect(edit).To(BeNil())
			}
		},

		Entry("#1 - No change",
			&vimtypes.VirtualWDT{},
			&vimtypes.VirtualWDT{},
			nil,
		),
		Entry("#2 - RunOnBoot set",
			&vimtypes.VirtualWDT{RunOnBoot: true},
			&vimtypes.VirtualWDT{},
			&vimtypes.VirtualWDT{RunOnBoot: true},
		),
		Entry("#3 - RunOnBoot not changed",
			&vimtypes.VirtualWDT{RunOnBoot: true},
			&vimtypes.VirtualWDT{RunOnBoot: true},
			nil,
		),
	)

})

var _ = Describe("Cmp(Ptr)Edit", func() {

	Context("CmpEdit", func() {

		DescribeTable("CmpEdit",
			func(a, b, expectedC int, expectedEdit bool) {
				c := 0
				edit := false
				resize.CmpEdit(a, b, &c, &edit)

				Expect(edit).To(Equal(expectedEdit))
				if expectedEdit {
					Expect(c).To(Equal(expectedC))
				} else {
					Expect(c).To(BeZero())
				}
			},

			Entry("Same", 1, 1, -1, false),
			Entry("Diff", 1, 2, 2, true),
		)
	})

	Context("cmpPtrEdit", func() {

		DescribeTable("CmpPtrEdit",
			func(a, b *int, expectedEdit bool) {
				var editP = new(int)
				var origEditP = editP
				var edit bool
				resize.CmpPtrEdit(a, b, &editP, &edit)
				Expect(edit).To(Equal(expectedEdit))
				if expectedEdit {
					Expect(editP).To(Equal(b))
				} else {
					Expect(editP).To(BeIdenticalTo(origEditP))
				}
			},
			Entry("current and expected are nil", nil, nil, false),
			Entry("current is nil", nil, ptr.To(1), true),
			Entry("current is not nil and expected is nil", ptr.To(1), nil, false),
			Entry("current and expected are the same", ptr.To(1), ptr.To(1), false),
			Entry("current and expected are different", ptr.To(1), ptr.To(2), true),
		)
	})
})
