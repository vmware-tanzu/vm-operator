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
		func(expected, current, edit *vimtypes.VirtualUSBController) {
			current.Key = 42
			e := resize.MatchVirtualUSBController(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualUSBController{},
			&vimtypes.VirtualUSBController{},
			nil,
		),
		Entry("#2",
			&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr},
			&vimtypes.VirtualUSBController{},
			&vimtypes.VirtualUSBController{AutoConnectDevices: truePtr},
		),
		Entry("#3",
			&vimtypes.VirtualUSBController{EhciEnabled: falsePtr},
			&vimtypes.VirtualUSBController{EhciEnabled: truePtr},
			&vimtypes.VirtualUSBController{EhciEnabled: falsePtr},
		),
	)

	DescribeTable("MatchVirtualUSBXHCIController",
		func(expected, current, edit *vimtypes.VirtualUSBXHCIController) {
			current.Key = 42
			e := resize.MatchVirtualUSBXHCIController(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualUSBXHCIController{},
			&vimtypes.VirtualUSBXHCIController{},
			nil,
		),
		Entry("#2",
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
			&vimtypes.VirtualUSBXHCIController{},
			&vimtypes.VirtualUSBXHCIController{AutoConnectDevices: truePtr},
		),
	)

	DescribeTable("MatchVirtualMachineVMCIDevice",
		func(expected, current, edit *vimtypes.VirtualMachineVMCIDevice) {
			current.Key = 42
			e := resize.MatchVirtualMachineVMCIDevice(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{},
			nil,
		),
		Entry("#2",
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
			nil,
		),
		Entry("#3",
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{AllowUnrestrictedCommunication: truePtr},
		),
		Entry("#4",
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
		),
		Entry("#5",
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: falsePtr},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
		),
		Entry("#6",
			&vimtypes.VirtualMachineVMCIDevice{},
			&vimtypes.VirtualMachineVMCIDevice{FilterEnable: truePtr},
			nil,
		),
		Entry("#7",
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
		Entry("#8",
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
		func(expected, current, edit *vimtypes.VirtualMachineVideoCard) {
			current.Key = 42
			e := resize.MatchVirtualMachineVideoCard(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualMachineVideoCard{},
			&vimtypes.VirtualMachineVideoCard{},
			nil,
		),
		Entry("#2",
			&vimtypes.VirtualMachineVideoCard{NumDisplays: 2},
			&vimtypes.VirtualMachineVideoCard{NumDisplays: 2},
			nil,
		),
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualParallelPort) {
			current.Key = 42
			e := resize.MatchVirtualParallelPort(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualParallelPort{},
			&vimtypes.VirtualParallelPort{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
	)

	DescribeTable("MatchVirtualPointingDevice",
		func(expected, current, edit *vimtypes.VirtualPointingDevice) {
			current.Key = 42
			e := resize.MatchVirtualPointingDevice(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualPointingDevice{},
			&vimtypes.VirtualPointingDevice{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualPrecisionClock) {
			current.Key = 42
			e := resize.MatchVirtualPrecisionClock(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualPrecisionClock{},
			&vimtypes.VirtualPrecisionClock{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualSCSIPassthrough) {
			current.Key = 42
			e := resize.MatchVirtualSCSIPassthrough(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualSCSIPassthrough{},
			&vimtypes.VirtualSCSIPassthrough{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualSerialPort) {
			current.Key = 42
			e := resize.MatchVirtualSerialPort(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualSerialPort{},
			&vimtypes.VirtualSerialPort{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		Entry("#4",
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
		Entry("#5",
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
		Entry("#6",
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
	)

	DescribeTable("MatchVirtualEnsoniq1371",
		func(expected, current, edit *vimtypes.VirtualEnsoniq1371) {
			current.Key = 42
			e := resize.MatchVirtualEnsoniq1371(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualEnsoniq1371{},
			&vimtypes.VirtualEnsoniq1371{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualHdAudioCard) {
			current.Key = 42
			e := resize.MatchVirtualHdAudioCard(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualHdAudioCard{},
			&vimtypes.VirtualHdAudioCard{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualSoundBlaster16) {
			current.Key = 42
			e := resize.MatchVirtualSoundBlaster16(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualSoundBlaster16{},
			&vimtypes.VirtualSoundBlaster16{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		func(expected, current, edit *vimtypes.VirtualTPM) {
			current.Key = 42
			e := resize.MatchVirtualTPM(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualTPM{},
			&vimtypes.VirtualTPM{},
			nil,
		),
		Entry("#2",
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
		Entry("#3",
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
		Entry("#4",
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
		func(expected, current, edit *vimtypes.VirtualWDT) {
			current.Key = 42
			e := resize.MatchVirtualWDT(expected, current)
			if edit != nil {
				edit.Key = 42
				Expect(e).To(Equal(edit))
			} else {
				Expect(e).To(BeNil())
			}
		},

		Entry("#1",
			&vimtypes.VirtualWDT{},
			&vimtypes.VirtualWDT{},
			nil,
		),
		Entry("#2",
			&vimtypes.VirtualWDT{RunOnBoot: true},
			&vimtypes.VirtualWDT{},
			&vimtypes.VirtualWDT{RunOnBoot: true},
		),
		Entry("#3",
			&vimtypes.VirtualWDT{RunOnBoot: true},
			&vimtypes.VirtualWDT{RunOnBoot: true},
			nil,
		),
	)

})
