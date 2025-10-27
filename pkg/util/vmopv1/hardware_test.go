// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// controllerTestParams defines parameters for controller type agnostic tests.
type controllerTestParams struct {
	controllerType   vmopv1.VirtualControllerType
	createController func(int32) vmopv1util.ControllerSpec
	maxSlots         int32
	reservedUnit     int32
}

// testParamsForController returns test parameters for a given controller type.
func testParamsForController(controllerType vmopv1.VirtualControllerType) controllerTestParams {
	switch controllerType {
	case vmopv1.VirtualControllerTypeSCSI:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeSCSI,
			createController: func(busNum int32) vmopv1util.ControllerSpec {
				return vmopv1.SCSIControllerSpec{
					BusNumber: busNum,
					Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
				}
			},
			maxSlots: vmopv1.SCSIControllerSpec{
				Type: vmopv1.SCSIControllerTypeParaVirtualSCSI,
			}.MaxSlots(),
			reservedUnit: 7,
		}
	case vmopv1.VirtualControllerTypeSATA:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeSATA,
			createController: func(busNum int32) vmopv1util.ControllerSpec {
				return vmopv1.SATAControllerSpec{
					BusNumber: busNum,
				}
			},
			maxSlots:     vmopv1.SATAControllerSpec{}.MaxSlots(),
			reservedUnit: -1,
		}
	case vmopv1.VirtualControllerTypeNVME:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeNVME,
			createController: func(busNum int32) vmopv1util.ControllerSpec {
				return vmopv1.NVMEControllerSpec{
					BusNumber: busNum,
				}
			},
			maxSlots:     vmopv1.NVMEControllerSpec{}.MaxSlots(),
			reservedUnit: -1,
		}
	case vmopv1.VirtualControllerTypeIDE:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeIDE,
			createController: func(busNum int32) vmopv1util.ControllerSpec {
				return vmopv1.IDEControllerSpec{
					BusNumber: busNum,
				}
			},
			maxSlots:     vmopv1.IDEControllerSpec{}.MaxSlots(),
			reservedUnit: -1,
		}
	}
	return controllerTestParams{}
}

var _ = Describe("NextAvailableUnitNumber", func() {
	DescribeTable("should return 0 when no slots are occupied",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			controller := params.createController(0)
			occupiedSlots := sets.New[int32]()

			unitNumber := vmopv1util.NextAvailableUnitNumber(controller, occupiedSlots)
			Expect(unitNumber).To(Equal(int32(0)))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	DescribeTable("should return 1 when slot 0 is occupied",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			controller := params.createController(0)
			occupiedSlots := sets.New[int32](0)

			unitNumber := vmopv1util.NextAvailableUnitNumber(controller, occupiedSlots)
			Expect(unitNumber).To(Equal(int32(1)))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	DescribeTable("should return first available slot with scattered occupied slots",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			controller := params.createController(0)
			occupiedSlots := sets.New[int32](
				0,
				2,
			)

			unitNumber := vmopv1util.NextAvailableUnitNumber(controller, occupiedSlots)
			Expect(unitNumber).To(Equal(int32(1)))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	DescribeTable("should return -1 when all slots are occupied",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			controller := params.createController(0)
			occupiedSlots := sets.New[int32]()

			// Fill all slots except reserved.
			for i := int32(0); i < params.maxSlots; i++ {
				if i != params.reservedUnit {
					occupiedSlots.Insert(i)
				}
			}

			unitNumber := vmopv1util.NextAvailableUnitNumber(controller, occupiedSlots)
			Expect(unitNumber).To(Equal(int32(-1)))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	DescribeTable("should treat nil map as empty",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			controller := params.createController(0)

			unitNumber := vmopv1util.NextAvailableUnitNumber(controller, nil)
			Expect(unitNumber).To(Equal(int32(0)))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	Context("SCSI-specific reserved unit number tests", func() {
		var (
			controller    vmopv1.SCSIControllerSpec
			occupiedSlots sets.Set[int32]
		)

		BeforeEach(func() {
			controller = vmopv1.SCSIControllerSpec{
				Type: vmopv1.SCSIControllerTypeParaVirtualSCSI,
			}
			occupiedSlots = sets.New[int32]()
		})

		It("should skip reserved slot 7 when slots 0-6 are occupied", func() {
			for i := int32(0); i < 7; i++ {
				occupiedSlots.Insert(i)
			}

			unitNumber := vmopv1util.NextAvailableUnitNumber(&controller, occupiedSlots)
			Expect(unitNumber).To(Equal(int32(8)))
		})

		It("should return -1 when all slots except 7 are occupied", func() {
			// ParaVirtual SCSI has 64 slots (0-63), with slot 7 reserved.
			for i := int32(0); i < 64; i++ {
				if i != 7 {
					occupiedSlots.Insert(i)
				}
			}

			unitNumber := vmopv1util.NextAvailableUnitNumber(&controller, occupiedSlots)
			Expect(unitNumber).To(Equal(int32(-1)))
		})

		Context("LsiLogic SCSI controller", func() {
			BeforeEach(func() {
				controller.Type = vmopv1.SCSIControllerTypeLsiLogic
			})

			It("should respect the 16 slot limit", func() {
				// Occupy all 16 slots except 7.
				for i := int32(0); i < 16; i++ {
					if i != 7 {
						occupiedSlots.Insert(i)
					}
				}

				unitNumber := vmopv1util.NextAvailableUnitNumber(&controller, occupiedSlots)
				Expect(unitNumber).To(Equal(int32(-1)))
			})

			It("should return first available within limit", func() {
				occupiedSlots.Insert(0)
				occupiedSlots.Insert(1)

				unitNumber := vmopv1util.NextAvailableUnitNumber(&controller, occupiedSlots)
				Expect(unitNumber).To(Equal(int32(2)))
			})
		})
	})
})

var _ = Describe("GenerateControllerID", func() {
	DescribeTable("should generate correct controller ID",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			busNumber := int32(2)
			controller := params.createController(busNumber)

			controllerID := vmopv1util.GenerateControllerID(controller)

			Expect(controllerID.ControllerType).To(Equal(params.controllerType))
			Expect(controllerID.BusNumber).To(Equal(busNumber))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	It("should return invalid controller ID for nil controller", func() {
		controllerID := vmopv1util.GenerateControllerID(nil)

		Expect(controllerID.BusNumber).To(Equal(int32(-1)))
	})

	It("should return invalid controller ID for unsupported type", func() {
		type unsupportedController struct{}

		controllerID := vmopv1util.GenerateControllerID(unsupportedController{})

		Expect(controllerID.BusNumber).To(Equal(int32(-1)))
	})
})

var _ = Describe("GetControllerSharingMode", func() {
	It("should return sharing mode for SCSI controller", func() {
		controller := vmopv1.SCSIControllerSpec{
			SharingMode: vmopv1.VirtualControllerSharingModePhysical,
		}

		sharingMode := vmopv1util.GetControllerSharingMode(controller)
		Expect(sharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
	})

	It("should return sharing mode for NVME controller", func() {
		controller := vmopv1.NVMEControllerSpec{
			SharingMode: vmopv1.VirtualControllerSharingModeVirtual,
		}

		sharingMode := vmopv1util.GetControllerSharingMode(controller)
		Expect(sharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
	})

	It("should return None for SATA controller (no sharing mode support)", func() {
		controller := vmopv1.SATAControllerSpec{}

		sharingMode := vmopv1util.GetControllerSharingMode(controller)
		Expect(sharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
	})

	It("should return None for IDE controller (no sharing mode support)", func() {
		controller := vmopv1.IDEControllerSpec{}

		sharingMode := vmopv1util.GetControllerSharingMode(controller)
		Expect(sharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
	})

	It("should return None for unsupported controller type", func() {
		type unsupportedController struct{}

		sharingMode := vmopv1util.GetControllerSharingMode(unsupportedController{})
		Expect(sharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
	})
})

var _ = Describe("CreateNewController", func() {
	DescribeTable("should create controller with correct type and bus number",
		func(controllerType vmopv1.VirtualControllerType) {
			params := testParamsForController(controllerType)
			busNumber := int32(3)
			controllerID := pkgutil.ControllerID{
				ControllerType: params.controllerType,
				BusNumber:      busNumber,
			}

			controller := vmopv1util.CreateNewController(
				controllerID,
				vmopv1.VirtualControllerSharingModeNone,
			)

			Expect(controller).ToNot(BeNil())
			generatedID := vmopv1util.GenerateControllerID(controller)
			Expect(generatedID.ControllerType).To(Equal(params.controllerType))
			Expect(generatedID.BusNumber).To(Equal(busNumber))
		},
		Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
		Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
		Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
		Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
	)

	It("should create SCSI controller with ParaVirtualSCSI type by default", func() {
		controllerID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSCSI,
			BusNumber:      0,
		}

		controller := vmopv1util.CreateNewController(
			controllerID,
			vmopv1.VirtualControllerSharingModeNone,
		)

		scsiController, ok := controller.(vmopv1.SCSIControllerSpec)
		Expect(ok).To(BeTrue())
		Expect(scsiController.Type).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))
	})

	It("should create SCSI controller with specified sharing mode", func() {
		controllerID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSCSI,
			BusNumber:      0,
		}

		controller := vmopv1util.CreateNewController(
			controllerID,
			vmopv1.VirtualControllerSharingModePhysical,
		)

		scsiController, ok := controller.(vmopv1.SCSIControllerSpec)
		Expect(ok).To(BeTrue())
		Expect(scsiController.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
	})

	It("should create NVME controller with specified sharing mode", func() {
		controllerID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeNVME,
			BusNumber:      1,
		}

		controller := vmopv1util.CreateNewController(
			controllerID,
			vmopv1.VirtualControllerSharingModeVirtual,
		)

		nvmeController, ok := controller.(vmopv1.NVMEControllerSpec)
		Expect(ok).To(BeTrue())
		Expect(nvmeController.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
	})

	It("should return nil for unsupported controller type", func() {
		controllerID := pkgutil.ControllerID{
			ControllerType: "UnsupportedType",
			BusNumber:      0,
		}

		controller := vmopv1util.CreateNewController(
			controllerID,
			vmopv1.VirtualControllerSharingModeNone,
		)

		Expect(controller).To(BeNil())
	})
})
