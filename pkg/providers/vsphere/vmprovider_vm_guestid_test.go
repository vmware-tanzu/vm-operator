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

func defaultGuestIDIfEmptyTests() {
	const (
		otherGuest   = string(vimtypes.VirtualMachineGuestOsIdentifierOtherGuest)
		otherGuest64 = string(vimtypes.VirtualMachineGuestOsIdentifierOtherGuest64)
		ubuntu64     = "ubuntu64Guest"
		otherLinux64 = string(vimtypes.VirtualMachineGuestOsIdentifierOtherLinux64Guest)
	)

	DescribeTable("DefaultGuestIDIfEmpty",
		func(
			vmSpecGuestID string,
			createGuestID string,
			imgGuestID string,
			wantChanged bool,
			wantGuestID string,
		) {
			configSpec := vimtypes.VirtualMachineConfigSpec{
				GuestId: createGuestID,
			}

			got := vsphere.DefaultGuestIDIfEmpty(
				vmSpecGuestID, imgGuestID, &configSpec)
			Expect(got).To(Equal(wantChanged))
			Expect(configSpec.GuestId).To(Equal(wantGuestID))
		},
		Entry("defaults to otherGuest when all are empty (standard VM class)",
			"", "", "",
			true, otherGuest,
		),
		Entry("defaults to otherGuest when class has otherGuest64 (UI-created VM class)",
			"", otherGuest64, "",
			true, otherGuest,
		),
		Entry("no change when VM spec has GuestID",
			ubuntu64, "", "",
			false, "",
		),
		Entry("no change when configSpec has explicit GuestId",
			"", otherLinux64, "",
			false, otherLinux64,
		),
		Entry("no change when image has GuestId",
			"", "", otherLinux64,
			false, "",
		),
		Entry("no change when all are set",
			ubuntu64, ubuntu64, ubuntu64,
			false, ubuntu64,
		),
	)
}
