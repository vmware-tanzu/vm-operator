// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
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
			for i := int32(0); i < controller.ReservedUnitNumber(); i++ {
				occupiedSlots.Insert(i)
			}

			unitNumber := vmopv1util.NextAvailableUnitNumber(&controller, occupiedSlots)
			Expect(unitNumber).To(Equal(controller.ReservedUnitNumber() + 1))
		})

		It("should return -1 when all slots except 7 are occupied", func() {
			for i := int32(0); i < controller.MaxSlots(); i++ {
				if i != controller.ReservedUnitNumber() {
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
				for i := int32(0); i < controller.MaxSlots(); i++ {
					if i != controller.ReservedUnitNumber() {
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

			controller := vmopv1util.CreateNewController(
				params.controllerType,
				busNumber,
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
		controller := vmopv1util.CreateNewController(
			vmopv1.VirtualControllerTypeSCSI,
			0,
			vmopv1.VirtualControllerSharingModeNone,
		)

		scsiController, ok := controller.(vmopv1.SCSIControllerSpec)
		Expect(ok).To(BeTrue())
		Expect(scsiController.Type).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))
	})

	It("should create SCSI controller with specified sharing mode", func() {
		controller := vmopv1util.CreateNewController(
			vmopv1.VirtualControllerTypeSCSI,
			0,
			vmopv1.VirtualControllerSharingModePhysical,
		)

		scsiController, ok := controller.(vmopv1.SCSIControllerSpec)
		Expect(ok).To(BeTrue())
		Expect(scsiController.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
	})

	It("should create NVME controller with specified sharing mode", func() {
		controller := vmopv1util.CreateNewController(
			vmopv1.VirtualControllerTypeNVME,
			1,
			vmopv1.VirtualControllerSharingModeVirtual,
		)

		nvmeController, ok := controller.(vmopv1.NVMEControllerSpec)
		Expect(ok).To(BeTrue())
		Expect(nvmeController.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
	})

	It("should return nil for unsupported controller type", func() {
		controller := vmopv1util.CreateNewController(
			"UnsupportedType",
			0,
			vmopv1.VirtualControllerSharingModeNone,
		)

		Expect(controller).To(BeNil())
	})
})

var _ = Describe("ControllerSpecs", func() {
	var (
		vm              vmopv1.VirtualMachine
		controllerSpecs vmopv1util.ControllerSpecs
	)

	BeforeEach(func() {
		vm = vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Hardware: &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					},
					SATAControllers: []vmopv1.SATAControllerSpec{
						{
							BusNumber: 0,
						},
					},
					NVMEControllers: []vmopv1.NVMEControllerSpec{
						{
							BusNumber:   0,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
					IDEControllers: []vmopv1.IDEControllerSpec{
						{
							BusNumber: 0,
						},
					},
				},
			},
		}
		controllerSpecs = vmopv1util.NewControllerSpecs(vm)
	})

	Describe("NewControllerSpecs", func() {
		It("should create ControllerSpecs from VM hardware spec", func() {
			Expect(controllerSpecs).ToNot(BeNil())
		})

		It("should handle VM with nil hardware spec", func() {
			vmWithoutHardware := vmopv1.VirtualMachine{}
			specs := vmopv1util.NewControllerSpecs(vmWithoutHardware)
			Expect(specs).ToNot(BeNil())
			Expect(specs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(0))
		})

		It("should load all controller types", func() {
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(2))
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSATA)).To(Equal(1))
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeNVME)).To(Equal(1))
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeIDE)).To(Equal(1))
		})
	})

	Describe("Get", func() {
		It("should retrieve existing controller by type and bus number", func() {
			controller, ok := controllerSpecs.Get(vmopv1.VirtualControllerTypeSCSI, 0)
			Expect(ok).To(BeTrue())
			Expect(controller).ToNot(BeNil())

			scsiController, ok := controller.(vmopv1.SCSIControllerSpec)
			Expect(ok).To(BeTrue())
			Expect(scsiController.BusNumber).To(Equal(int32(0)))
			Expect(scsiController.Type).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))
		})

		It("should return false for non-existent controller type", func() {
			controller, ok := controllerSpecs.Get(vmopv1.VirtualControllerTypeSCSI, 99)
			Expect(ok).To(BeFalse())
			Expect(controller).To(BeNil())
		})

		It("should return false for non-existent bus number", func() {
			emptyVM := vmopv1.VirtualMachine{}
			emptySpecs := vmopv1util.NewControllerSpecs(emptyVM)
			controller, ok := emptySpecs.Get(vmopv1.VirtualControllerTypeSCSI, 0)
			Expect(ok).To(BeFalse())
			Expect(controller).To(BeNil())
		})

		It("should retrieve controllers with different sharing modes", func() {
			controller0, ok := controllerSpecs.Get(vmopv1.VirtualControllerTypeSCSI, 0)
			Expect(ok).To(BeTrue())

			controller1, ok := controllerSpecs.Get(vmopv1.VirtualControllerTypeSCSI, 1)
			Expect(ok).To(BeTrue())

			scsi0, ok := controller0.(vmopv1.SCSIControllerSpec)
			Expect(ok).To(BeTrue())
			Expect(scsi0.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))

			scsi1, ok := controller1.(vmopv1.SCSIControllerSpec)
			Expect(ok).To(BeTrue())
			Expect(scsi1.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
		})
	})

	Describe("Set", func() {
		It("should add a new controller", func() {
			newController := vmopv1.SCSIControllerSpec{
				BusNumber:   2,
				Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
				SharingMode: vmopv1.VirtualControllerSharingModeVirtual,
			}

			controllerSpecs.Set(vmopv1.VirtualControllerTypeSCSI, 2, newController)

			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(3))
			retrieved, ok := controllerSpecs.Get(vmopv1.VirtualControllerTypeSCSI, 2)
			Expect(ok).To(BeTrue())
			Expect(retrieved).ToNot(BeNil())

			scsiController, ok := retrieved.(vmopv1.SCSIControllerSpec)
			Expect(ok).To(BeTrue())
			Expect(scsiController.BusNumber).To(Equal(int32(2)))
			Expect(scsiController.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
		})

		It("should update an existing controller", func() {
			updatedController := vmopv1.SCSIControllerSpec{
				BusNumber:   0,
				Type:        vmopv1.SCSIControllerTypeLsiLogic,
				SharingMode: vmopv1.VirtualControllerSharingModeVirtual,
			}

			controllerSpecs.Set(vmopv1.VirtualControllerTypeSCSI, 0, updatedController)

			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(2))

			retrieved, ok := controllerSpecs.Get(vmopv1.VirtualControllerTypeSCSI, 0)
			Expect(ok).To(BeTrue())
			scsiController, ok := retrieved.(vmopv1.SCSIControllerSpec)
			Expect(ok).To(BeTrue())
			Expect(scsiController.Type).To(Equal(vmopv1.SCSIControllerTypeLsiLogic))
			Expect(scsiController.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeVirtual))
		})

		It("should add controller of new type", func() {
			emptyVM := vmopv1.VirtualMachine{}
			emptySpecs := vmopv1util.NewControllerSpecs(emptyVM)

			newController := vmopv1.SATAControllerSpec{
				BusNumber: 0,
			}

			emptySpecs.Set(vmopv1.VirtualControllerTypeSATA, 0, newController)

			Expect(emptySpecs.CountControllers(vmopv1.VirtualControllerTypeSATA)).To(Equal(1))
			retrieved, ok := emptySpecs.Get(vmopv1.VirtualControllerTypeSATA, 0)
			Expect(ok).To(BeTrue())
			Expect(retrieved).ToNot(BeNil())
		})
	})

	Describe("CountControllers", func() {
		It("should return correct count for each controller type", func() {
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(2))
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSATA)).To(Equal(1))
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeNVME)).To(Equal(1))
			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeIDE)).To(Equal(1))
		})

		It("should return 0 for controller type with no controllers", func() {
			emptyVM := vmopv1.VirtualMachine{}
			emptySpecs := vmopv1util.NewControllerSpecs(emptyVM)

			Expect(emptySpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(0))
			Expect(emptySpecs.CountControllers(vmopv1.VirtualControllerTypeSATA)).To(Equal(0))
			Expect(emptySpecs.CountControllers(vmopv1.VirtualControllerTypeNVME)).To(Equal(0))
			Expect(emptySpecs.CountControllers(vmopv1.VirtualControllerTypeIDE)).To(Equal(0))
		})

		It("should update count after adding controllers", func() {
			initialCount := controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)

			newController := vmopv1.SCSIControllerSpec{
				BusNumber: 3,
				Type:      vmopv1.SCSIControllerTypeParaVirtualSCSI,
			}
			controllerSpecs.Set(vmopv1.VirtualControllerTypeSCSI, 3, newController)

			Expect(controllerSpecs.CountControllers(vmopv1.VirtualControllerTypeSCSI)).To(Equal(initialCount + 1))
		})
	})
})
