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

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
)

type ConfigSpec = vimtypes.VirtualMachineConfigSpec
type ConfigInfo = vimtypes.VirtualMachineConfigInfo

var _ = Describe("CreateResizeConfigSpec", func() {

	ctx := context.Background()
	truePtr, falsePtr := vimtypes.NewBool(true), vimtypes.NewBool(false)

	DescribeTable("ConfigInfo",
		func(
			ci vimtypes.VirtualMachineConfigInfo,
			cs, expectedCS vimtypes.VirtualMachineConfigSpec) {

			actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
			Expect(err).ToNot(HaveOccurred())
			Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
		},

		Entry("Empty needs no updating",
			ConfigInfo{},
			ConfigSpec{},
			ConfigSpec{}),

		Entry("Annotation is currently set",
			ConfigInfo{Annotation: "my-annotation"},
			ConfigSpec{},
			ConfigSpec{}),
		Entry("Annotation is currently unset",
			ConfigInfo{},
			ConfigSpec{Annotation: "my-annotation"},
			ConfigSpec{Annotation: "my-annotation"}),

		Entry("ManagedBy is currently set",
			ConfigInfo{ManagedBy: &vimtypes.ManagedByInfo{Type: "my-managed-by"}},
			ConfigSpec{},
			ConfigSpec{}),
		Entry("ManagedBy is currently unset",
			ConfigInfo{},
			ConfigSpec{ManagedBy: &vimtypes.ManagedByInfo{Type: "my-managed-by"}},
			ConfigSpec{ManagedBy: &vimtypes.ManagedByInfo{Type: "my-managed-by"}}),

		Entry("NumCPUs needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCPU: 2}},
			ConfigSpec{NumCPUs: 4},
			ConfigSpec{NumCPUs: 4}),
		Entry("NumCpus does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCPU: 4}},
			ConfigSpec{NumCPUs: 4},
			ConfigSpec{}),

		Entry("NumCoresPerSocket needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCoresPerSocket: 2}},
			ConfigSpec{NumCoresPerSocket: 4},
			ConfigSpec{NumCoresPerSocket: 4}),
		Entry("NumCoresPerSocket does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{NumCoresPerSocket: 4}},
			ConfigSpec{NumCoresPerSocket: 4},
			ConfigSpec{}),

		Entry("MemoryMB needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MemoryMB: 512}},
			ConfigSpec{MemoryMB: 1024},
			ConfigSpec{MemoryMB: 1024}),
		Entry("MemoryMB does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MemoryMB: 1024}},
			ConfigSpec{MemoryMB: 1024},
			ConfigSpec{}),

		Entry("VirtualICH7MPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualICH7MPresent: truePtr}},
			ConfigSpec{VirtualICH7MPresent: falsePtr},
			ConfigSpec{VirtualICH7MPresent: falsePtr}),
		Entry("VirtualICH7MPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualICH7MPresent: nil}},
			ConfigSpec{VirtualICH7MPresent: falsePtr},
			ConfigSpec{VirtualICH7MPresent: falsePtr}),
		Entry("VirtualICH7MPresent does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualICH7MPresent: truePtr}},
			ConfigSpec{VirtualICH7MPresent: truePtr},
			ConfigSpec{}),

		Entry("VirtualSMCPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualSMCPresent: truePtr}},
			ConfigSpec{VirtualSMCPresent: falsePtr},
			ConfigSpec{VirtualSMCPresent: falsePtr}),
		Entry("VirtualSMCPresent needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualSMCPresent: nil}},
			ConfigSpec{VirtualSMCPresent: falsePtr},
			ConfigSpec{VirtualSMCPresent: falsePtr}),
		Entry("VirtualSMCPresent does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{VirtualSMCPresent: truePtr}},
			ConfigSpec{VirtualSMCPresent: truePtr},
			ConfigSpec{}),

		Entry("MotherboardLayout needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MotherboardLayout: "foo"}},
			ConfigSpec{MotherboardLayout: "i440bxHostBridge"},
			ConfigSpec{MotherboardLayout: "i440bxHostBridge"}),
		Entry("MotherboardLayout does not needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{MotherboardLayout: "i440bxHostBridge"}},
			ConfigSpec{MotherboardLayout: "i440bxHostBridge"},
			ConfigSpec{}),

		Entry("SimultaneousThreads needs updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{SimultaneousThreads: 8}},
			ConfigSpec{SimultaneousThreads: 16},
			ConfigSpec{SimultaneousThreads: 16}),
		Entry("SimultaneousThreads does not need updating",
			ConfigInfo{Hardware: vimtypes.VirtualHardware{SimultaneousThreads: 8}},
			ConfigSpec{SimultaneousThreads: 8},
			ConfigSpec{}),

		Entry("CPU allocation (reservation, limit, shares) settings needs updating",
			ConfigInfo{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(100)),
					Limit:       ptr.To(int64(100)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelNormal},
				}},
			ConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(200)),
					Limit:       ptr.To(int64(200)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelCustom, Shares: 50},
				}},
			ConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(200)),
					Limit:       ptr.To(int64(200)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelCustom, Shares: 50},
				}}),
		Entry("CPU allocation (reservation, limit, shares) settings needs updating - empty to values set ",
			ConfigInfo{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{}},
			ConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(200)),
					Limit:       ptr.To(int64(200)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelNormal},
				}},
			ConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(200)),
					Limit:       ptr.To(int64(200)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelNormal},
				}}),
		Entry("CPU allocation (reservation,limit,shares) settings does not need updating",
			ConfigInfo{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(100)),
					Limit:       ptr.To(int64(100)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelNormal},
				}},
			ConfigSpec{
				CpuAllocation: &vimtypes.ResourceAllocationInfo{
					Reservation: ptr.To(int64(100)),
					Limit:       ptr.To(int64(100)),
					Shares:      &vimtypes.SharesInfo{Level: vimtypes.SharesLevelNormal},
				}},
			ConfigSpec{}),

		Entry("CPU Hot Add/Remove needs updating false to true",
			ConfigInfo{CpuHotAddEnabled: falsePtr, CpuHotRemoveEnabled: falsePtr},
			ConfigSpec{CpuHotAddEnabled: truePtr, CpuHotRemoveEnabled: truePtr},
			ConfigSpec{CpuHotAddEnabled: truePtr, CpuHotRemoveEnabled: truePtr}),
		Entry("CPU Hot Add/Remove needs updating nil to true",
			ConfigInfo{},
			ConfigSpec{CpuHotAddEnabled: truePtr, CpuHotRemoveEnabled: truePtr},
			ConfigSpec{CpuHotAddEnabled: truePtr, CpuHotRemoveEnabled: truePtr}),
		Entry("CPU Hot Add/Remove does not need updating",
			ConfigInfo{CpuHotAddEnabled: falsePtr, CpuHotRemoveEnabled: falsePtr},
			ConfigSpec{CpuHotAddEnabled: falsePtr, CpuHotRemoveEnabled: falsePtr},
			ConfigSpec{}),

		Entry("CPU affinity settings needs updating value change",
			ConfigInfo{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{2, 3}}},
			ConfigSpec{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{1, 3}}},
			ConfigSpec{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{1, 3}}}),
		Entry("CPU affinity settings needs updating - remove existing",
			ConfigInfo{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{2, 3}}},
			ConfigSpec{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{}}},
			ConfigSpec{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{}}}),
		Entry("CPU affinity settings does not need updating",
			ConfigInfo{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{1, 2, 3}}},
			ConfigSpec{CpuAffinity: &vimtypes.VirtualMachineAffinityInfo{AffinitySet: []int32{3, 1, 2}}},
			ConfigSpec{}),

		Entry("CPU perf counter settings does not need updating",
			ConfigInfo{VPMCEnabled: falsePtr},
			ConfigSpec{VPMCEnabled: falsePtr},
			ConfigSpec{}),
		Entry("CPU perf counter settings needs updating",
			ConfigInfo{VPMCEnabled: falsePtr},
			ConfigSpec{VPMCEnabled: truePtr},
			ConfigSpec{VPMCEnabled: truePtr}),

		Entry("Latency sensitivity settings needs updating",
			ConfigInfo{LatencySensitivity: &vimtypes.LatencySensitivity{Level: vimtypes.LatencySensitivitySensitivityLevelLow}},
			ConfigSpec{LatencySensitivity: &vimtypes.LatencySensitivity{Level: vimtypes.LatencySensitivitySensitivityLevelMedium}},
			ConfigSpec{LatencySensitivity: &vimtypes.LatencySensitivity{Level: vimtypes.LatencySensitivitySensitivityLevelMedium}}),
		Entry("Latency sensitivity settings does not need updating",
			ConfigInfo{LatencySensitivity: &vimtypes.LatencySensitivity{Level: vimtypes.LatencySensitivitySensitivityLevelLow}},
			ConfigSpec{LatencySensitivity: &vimtypes.LatencySensitivity{Level: vimtypes.LatencySensitivitySensitivityLevelLow}},
			ConfigSpec{}),

		Entry("Extra Config setting needs updating -- existing key",
			ConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "foo", Value: "bar"}}},
			ConfigSpec{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "foo", Value: "bar1"}}},
			ConfigSpec{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "foo", Value: "bar1"}}}),
		Entry("Extra Config setting needs updating -- new key",
			ConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "foo", Value: "bar"}}},
			ConfigSpec{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "bat", Value: "man"}}},
			ConfigSpec{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "bat", Value: "man"}}}),
		Entry("Extra Config setting does not need updating",
			ConfigInfo{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "foo", Value: "bar"}}},
			ConfigSpec{ExtraConfig: []vimtypes.BaseOptionValue{&vimtypes.OptionValue{Key: "foo", Value: "bar"}}},
			ConfigSpec{}),

		Entry("Console Preferences needs updating -- configInfo console preferences nil",
			ConfigInfo{},
			ConfigSpec{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					EnterFullScreenOnPowerOn: truePtr,
					PowerOnWhenOpened:        falsePtr,
				}},
			ConfigSpec{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					EnterFullScreenOnPowerOn: truePtr,
					PowerOnWhenOpened:        falsePtr,
				}}),
		Entry("Console Preferences needs updating -- configInfo console preferences set",
			ConfigInfo{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					PowerOnWhenOpened: truePtr,
				}},
			ConfigSpec{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					EnterFullScreenOnPowerOn: truePtr,
					PowerOnWhenOpened:        falsePtr,
				}},
			ConfigSpec{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					EnterFullScreenOnPowerOn: truePtr,
					PowerOnWhenOpened:        falsePtr,
				}}),
		Entry("Console Preferences does not need updating",
			ConfigInfo{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					EnterFullScreenOnPowerOn: truePtr,
					PowerOnWhenOpened:        falsePtr,
				}},
			ConfigSpec{
				ConsolePreferences: &vimtypes.VirtualMachineConsolePreferences{
					EnterFullScreenOnPowerOn: truePtr,
					PowerOnWhenOpened:        falsePtr,
				}},
			ConfigSpec{}),

		Entry("CbrcCacheEnabled flag does not need updating",
			ConfigInfo{Flags: vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: truePtr,
			}},
			ConfigSpec{Flags: &vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: truePtr,
			}},
			ConfigSpec{}),
		Entry("CbrcCacheEnabled flag needs updating -- configInfo has no flags",
			ConfigInfo{},
			ConfigSpec{Flags: &vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: truePtr,
			}},
			ConfigSpec{Flags: &vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: truePtr,
			}}),
		Entry("CbrcCacheEnabled flag needs updating",
			ConfigInfo{Flags: vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: falsePtr,
			}},
			ConfigSpec{Flags: &vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: truePtr,
			}},
			ConfigSpec{Flags: &vimtypes.VirtualMachineFlagInfo{
				CbrcCacheEnabled: truePtr,
			}}),
	)

	type giveMeDeviceFn = func() vimtypes.BaseVirtualDevice

	vmxnet3Device := func() giveMeDeviceFn {
		return func() vimtypes.BaseVirtualDevice {
			// Just a dummy device w/o backing until we need that.
			return &vimtypes.VirtualVmxnet3{}
		}
	}

	vGPUDevice := func(profileName string) giveMeDeviceFn {
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

	ddpioDevice := func(label string, vendorID, deviceID int32) giveMeDeviceFn {
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

	DescribeTableSubtree("ConfigInfo.Hardware.Devices",
		func(device, otherDevice giveMeDeviceFn) {
			const (
				AddOp    = vimtypes.VirtualDeviceConfigSpecOperationAdd
				RemoveOp = vimtypes.VirtualDeviceConfigSpecOperationRemove
			)

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
				d := vmxnet3Device()()
				ci.Hardware.Device = append(ci.Hardware.Device, d)

				actualCS, err := resize.CreateResizeConfigSpec(ctx, ci, cs)
				Expect(err).ToNot(HaveOccurred())

				// TBD: Might be easier to switch away from DeepEqual() here 'cause of the device Key annoyances.
				Expect(reflect.DeepEqual(actualCS, expectedCS)).To(BeTrue(), cmp.Diff(actualCS, expectedCS))
			})

			devChangeEntry := func(
				op vimtypes.VirtualDeviceConfigSpecOperation,
				dev vimtypes.BaseVirtualDevice,
				devKey ...int32) vimtypes.BaseVirtualDeviceConfigSpec {

				if len(devKey) == 1 {
					dev.GetVirtualDevice().Key = devKey[0]
				}

				return &vimtypes.VirtualDeviceConfigSpec{
					Operation: op,
					Device:    dev,
				}
			}

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
	)
})
