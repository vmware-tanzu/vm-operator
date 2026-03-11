// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
)

func defaultGuestIDForBusLogicTests() {
	const (
		otherLinux64 = string(vimtypes.VirtualMachineGuestOsIdentifierOtherLinux64Guest)
		otherLinux   = string(vimtypes.VirtualMachineGuestOsIdentifierOtherLinuxGuest)
	)

	busLogicSpec := &vimtypes.VirtualDeviceConfigSpec{
		Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
		Device:    &vimtypes.VirtualBusLogicController{VirtualSCSIController: vimtypes.VirtualSCSIController{}},
	}
	otherDeviceSpec := &vimtypes.VirtualDeviceConfigSpec{
		Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
		Device:    &vimtypes.VirtualDisk{},
	}

	DescribeTable("DefaultGuestIDForBusLogicIfNeeded",
		func(
			vmSpecGuestID string,
			createGuestID string,
			deviceChange []vimtypes.BaseVirtualDeviceConfigSpec,
			imgGuestID string,
			wantChanged bool,
			wantGuestID string,
		) {
			configSpec := vimtypes.VirtualMachineConfigSpec{
				GuestId:      createGuestID,
				DeviceChange: deviceChange,
			}

			got := vsphere.DefaultGuestIDForBusLogicIfNeeded(
				vmSpecGuestID, imgGuestID, &configSpec)
			Expect(got).To(Equal(wantChanged))
			Expect(configSpec.GuestId).To(Equal(wantGuestID))
		},
		Entry("defaults to otherLinuxGuest when all conditions met",
			"", otherLinux64,
			[]vimtypes.BaseVirtualDeviceConfigSpec{busLogicSpec},
			otherLinux,
			true, otherLinux,
		),
		Entry("no change when VM spec has GuestID",
			"ubuntu64Guest", otherLinux64,
			[]vimtypes.BaseVirtualDeviceConfigSpec{busLogicSpec},
			"",
			false, otherLinux64,
		),
		Entry("no change when createArgs GuestId is not otherLinux64",
			"", "ubuntu64Guest",
			[]vimtypes.BaseVirtualDeviceConfigSpec{busLogicSpec},
			otherLinux,
			false, "ubuntu64Guest",
		),
		Entry("no change when image GuestId is otherLinux64",
			"", otherLinux64,
			[]vimtypes.BaseVirtualDeviceConfigSpec{busLogicSpec},
			otherLinux64,
			false, otherLinux64,
		),
		Entry("no change when no BusLogic controller",
			"", otherLinux64,
			[]vimtypes.BaseVirtualDeviceConfigSpec{otherDeviceSpec},
			otherLinux,
			false, otherLinux64,
		),
		Entry("no change when DeviceChange is nil",
			"", otherLinux64,
			nil,
			otherLinux,
			false, otherLinux64,
		),
		Entry("no change when DeviceChange is empty",
			"", otherLinux64,
			[]vimtypes.BaseVirtualDeviceConfigSpec{},
			otherLinux,
			false, otherLinux64,
		),
		Entry("defaults when BusLogic is second in DeviceChange",
			"", otherLinux64,
			[]vimtypes.BaseVirtualDeviceConfigSpec{otherDeviceSpec, busLogicSpec},
			"",
			true, otherLinux,
		),
	)
}
