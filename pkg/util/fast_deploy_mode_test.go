// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = DescribeTable("IsOnlinePromoteDisksSupported",
	func(in vimtypes.VirtualMachineConfigSpec, out bool) {
		Expect(pkgutil.IsOnlinePromoteDisksSupported(in)).To(Equal(out))
	},

	Entry("empty config spec",
		vimtypes.VirtualMachineConfigSpec{},
		true,
	),

	Entry("config spec with no device changes",
		vimtypes.VirtualMachineConfigSpec{
			Name: "test-vm",
		},
		true,
	),

	Entry("config spec with nil device change",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{nil},
		},
		true,
	),

	Entry("config spec with non-PCI device",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualDisk{},
				},
			},
		},
		true,
	),

	Entry("config spec with VirtualPCIPassthrough with Vmiop backing (supported)",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{},
						},
					},
				},
			},
		},
		true,
	),

	Entry("config spec with VirtualPCIPassthrough with DVX backing (supported)",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualPCIPassthroughDvxBackingInfo{},
						},
					},
				},
			},
		},
		true,
	),

	Entry("config spec with VirtualPCIPassthrough with device backing (not supported)",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualPCIPassthroughDeviceBackingInfo{},
						},
					},
				},
			},
		},
		false,
	),

	Entry("config spec with VirtualPCIPassthrough with dynamic backing (not supported)",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{},
						},
					},
				},
			},
		},
		false,
	),

	Entry("config spec with multiple devices, none unsupported",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualDisk{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{},
						},
					},
				},
			},
		},
		true,
	),

	Entry("config spec with multiple devices, one unsupported",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualDisk{},
				},
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: &vimtypes.VirtualPCIPassthroughDeviceBackingInfo{},
						},
					},
				},
			},
		},
		false,
	),

	Entry("config spec with VirtualPCIPassthrough but nil backing",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: &vimtypes.VirtualPCIPassthrough{
						VirtualDevice: vimtypes.VirtualDevice{
							Backing: nil,
						},
					},
				},
			},
		},
		true,
	),

	Entry("config spec with nil device in config spec",
		vimtypes.VirtualMachineConfigSpec{
			DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
				&vimtypes.VirtualDeviceConfigSpec{
					Device: nil,
				},
			},
		},
		true,
	),
)
