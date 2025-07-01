// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vm_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

func guestIDTests() {
	DescribeTable("IsTaskInfoErrorInvalidGuestID",
		func(taskInfo *vimtypes.TaskInfo, expected bool) {
			Expect(vmutil.IsTaskInfoErrorInvalidGuestID(taskInfo)).To(Equal(expected))
		},
		Entry(
			"taskInfo is nil",
			nil,
			false,
		),
		Entry(
			"taskInfo.Error is nil",
			&vimtypes.TaskInfo{},
			false,
		),
		Entry(
			"taskInfo.Error.Fault is nil",
			&vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{},
			},
			false,
		),
		Entry(
			"taskInfo.Error.Fault is not InvalidArgument",
			&vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.AdminDisabled{},
				},
			},
			false,
		),
		Entry(
			"taskInfo.Error.Fault is InvalidArgument with wrong property",
			&vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.InvalidArgument{
						InvalidProperty: "configSpec.name",
					},
				},
			},
			false,
		),
		Entry(
			"taskInfo.Error.Fault is InvalidArgument with guestID",
			&vimtypes.TaskInfo{
				Error: &vimtypes.LocalizedMethodFault{
					Fault: &vimtypes.InvalidArgument{
						InvalidProperty: vmutil.GuestIDProperty,
					},
				},
			},
			true,
		),
	)
}
