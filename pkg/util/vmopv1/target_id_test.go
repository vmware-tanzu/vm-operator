// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

type fakeTargetID struct {
	controllerType      vmopv1.VirtualControllerType
	controllerBusNumber *int32
	unitNumber          *int32
}

func (v fakeTargetID) GetControllerType() vmopv1.VirtualControllerType {
	return v.controllerType
}
func (v fakeTargetID) GetControllerBusNumber() *int32 {
	return v.controllerBusNumber
}
func (v fakeTargetID) GetUnitNumber() *int32 {
	return v.unitNumber
}

var _ = DescribeTable("GetTargetID",
	func(in vmopv1util.HasTargetID, out string) {
		Expect(vmopv1util.GetTargetID(in)).To(Equal(out))
	},
	Entry(
		"empty",
		fakeTargetID{},
		"",
	),
	Entry(
		"empty controller type",
		fakeTargetID{
			controllerType:      "",
			controllerBusNumber: ptr.To[int32](1),
			unitNumber:          ptr.To[int32](2),
		},
		"",
	),
	Entry(
		"empty controller bus number",
		fakeTargetID{
			controllerType:      vmopv1.VirtualControllerTypeSCSI,
			controllerBusNumber: nil,
			unitNumber:          ptr.To[int32](2),
		},
		"",
	),
	Entry(
		"empty unit number",
		fakeTargetID{
			controllerType:      vmopv1.VirtualControllerTypeSCSI,
			controllerBusNumber: ptr.To[int32](1),
			unitNumber:          nil,
		},
		"",
	),
	Entry(
		"valid input",
		fakeTargetID{
			controllerType:      vmopv1.VirtualControllerTypeSCSI,
			controllerBusNumber: ptr.To[int32](1),
			unitNumber:          ptr.To[int32](2),
		},
		"SCSI:1:2",
	),
)

var _ = Describe("TargetID", func() {
	DescribeTable("String",
		func(in vmopv1util.TargetID, out string) {
			Expect(in.String()).To(Equal(out))
		},
		Entry(
			"SCSI",
			vmopv1util.TargetID{
				ControllerType: vmopv1.VirtualControllerTypeSCSI,
				ControllerBus:  1,
				UnitNumber:     2,
			},
			"SCSI:1:2",
		),
		Entry(
			"IDE",
			vmopv1util.TargetID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				ControllerBus:  1,
				UnitNumber:     2,
			},
			"IDE:1:2",
		),
		Entry(
			"SATA",
			vmopv1util.TargetID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				ControllerBus:  1,
				UnitNumber:     2,
			},
			"SATA:1:2",
		),
		Entry(
			"NVME",
			vmopv1util.TargetID{
				ControllerType: vmopv1.VirtualControllerTypeNVME,
				ControllerBus:  1,
				UnitNumber:     2,
			},
			"NVME:1:2",
		),
	)
})
