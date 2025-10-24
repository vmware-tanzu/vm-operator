// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

var _ = Describe("MutateCdromControllerOnUpdate", func() {
	var (
		ctx *builder.UnitTestContextForMutatingWebhook
		vm  *vmopv1.VirtualMachine
	)

	// Helper function to call the mutator and return results.
	callMutator := func() (bool, error) {
		return mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
	}

	// Helper function to assert CD-ROM controller assignment.
	assertCdromController := func(index int, controllerType vmopv1.VirtualControllerType, busNumber, unitNumber int32) {
		Expect(vm.Spec.Hardware.Cdrom[index].ControllerType).To(Equal(controllerType))
		Expect(vm.Spec.Hardware.Cdrom[index].ControllerBusNumber).To(Equal(ptr.To(busNumber)))
		Expect(vm.Spec.Hardware.Cdrom[index].UnitNumber).To(Equal(ptr.To(unitNumber)))
	}

	// Helper function to assert controller creation.
	assertControllerCreated := func(controllerType vmopv1.VirtualControllerType, expectedCount int, expectedBusNumbers ...int32) {
		switch controllerType {
		case vmopv1.VirtualControllerTypeIDE:
			Expect(vm.Spec.Hardware.IDEControllers).To(HaveLen(expectedCount))
			for i, busNum := range expectedBusNumbers {
				Expect(vm.Spec.Hardware.IDEControllers[i].BusNumber).To(Equal(busNum))
			}
		case vmopv1.VirtualControllerTypeSATA:
			Expect(vm.Spec.Hardware.SATAControllers).To(HaveLen(expectedCount))
			for i, busNum := range expectedBusNumbers {
				Expect(vm.Spec.Hardware.SATAControllers[i].BusNumber).To(Equal(busNum))
			}
		}
	}

	// Helper function to set up CD-ROM specs with names.
	setupCdromSpecs := func(names ...string) {
		vm.Spec.Hardware.Cdrom = make([]vmopv1.VirtualMachineCdromSpec, len(names))
		for i, name := range names {
			vm.Spec.Hardware.Cdrom[i] = vmopv1.VirtualMachineCdromSpec{Name: name}
		}
	}

	// Helper function for common test pattern: call mutator and assert success.
	expectMutationSuccess := func() bool {
		wasMutated, err := callMutator()
		Expect(err).ToNot(HaveOccurred())
		Expect(wasMutated).To(BeTrue())
		return wasMutated
	}

	// Helper function for common test pattern: call mutator and assert no mutation.
	expectNoMutation := func() bool {
		wasMutated, err := callMutator()
		Expect(err).ToNot(HaveOccurred())
		Expect(wasMutated).To(BeFalse())
		return wasMutated
	}

	BeforeEach(func() {
		vm = builder.DummyVirtualMachine()
		vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
		vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{}

		obj, err := builder.ToUnstructured(vm)
		Expect(err).ToNot(HaveOccurred())
		ctx = suite.NewUnitTestContextForMutatingWebhook(obj)
	})

	Context("When VMSharedDisks feature is disabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx.Context, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = false
			})
		})

		It("should not mutate", func() {
			expectNoMutation()
		})
	})

	Context("When VMSharedDisks feature is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx.Context, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
		})

		Context("When VM has no CD-ROM specs", func() {
			It("should not mutate when no CD-ROM specs exist", func() {
				expectNoMutation()
			})
		})

		Context("When VM has CD-ROM without controller info", func() {
			BeforeEach(func() {
				setupCdromSpecs("cdrom1")
			})

			It("should assign IDE controller by default", func() {
				expectMutationSuccess()
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
			})
		})

		Context("When VM has CD-ROM with only controller type", func() {
			Context("IDE controller type", func() {
				BeforeEach(func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						{
							Name:           "cdrom1",
							ControllerType: vmopv1.VirtualControllerTypeIDE,
						},
					}
				})

				It("should assign IDE bus number and unit number", func() {
					wasMutated, err := callMutator()
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				})
			})

			Context("SATA controller type", func() {
				BeforeEach(func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						{
							Name:           "cdrom1",
							ControllerType: vmopv1.VirtualControllerTypeSATA,
						},
					}
				})

				It("should assign SATA bus number and unit number", func() {
					wasMutated, err := callMutator()
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
				})
			})
		})

		Context("When VM has CD-ROM with only bus number", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:                "cdrom1",
						ControllerBusNumber: ptr.To(int32(0)),
					},
				}
			})

			It("should assign IDE controller type by default and unit number", func() {
				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
			})
		})

		Context("When VM has CD-ROM with unit number but no controller info", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:       "cdrom1",
						UnitNumber: ptr.To(int32(0)),
					},
				}
			})

			It("should not mutate when unit number is set but controller info is incomplete", func() {
				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeFalse())
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(BeEmpty())
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(BeNil())
				Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(Equal(ptr.To(int32(0))))
			})
		})

		Context("When VM has CD-ROM with complete controller info", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:                "cdrom1",
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(1)),
					},
				}
			})

			It("should create IDE controller if not exists", func() {
				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 1, 0)
			})
		})

		Context("When VM has multiple CD-ROMs", func() {
			BeforeEach(func() {
				setupCdromSpecs("cdrom1", "cdrom2", "cdrom3", "cdrom4", "cdrom5")
			})

			It("should create IDE controllers first, then SATA controller", func() {
				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())

				// Should have 2 IDE controllers (bus 0 and 1) and 1 SATA controller (bus 0).
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
				assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)

				// First 2 CD-ROMs should be on IDE bus 0, units 0 and 1.
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)

				// Next 2 CD-ROMs should be on IDE bus 1, units 0 and 1.
				assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 1, 0)
				assertCdromController(3, vmopv1.VirtualControllerTypeIDE, 1, 1)

				// Last CD-ROM should be on SATA bus 0, unit 0.
				assertCdromController(4, vmopv1.VirtualControllerTypeSATA, 0, 0)
			})
		})

		Context("When VM has existing IDE controllers in spec", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:           "cdrom1",
						ControllerType: vmopv1.VirtualControllerTypeIDE,
					},
				}
			})

			It("should use existing IDE controller", func() {
				wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
			})
		})

		Context("When VM has existing SATA controllers in spec", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:           "cdrom1",
						ControllerType: vmopv1.VirtualControllerTypeSATA,
					},
				}
			})

			It("should use existing SATA controller", func() {
				wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
			})
		})

		Context("When VM has controllers in status", func() {
			BeforeEach(func() {
				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeIDE,
							BusNumber: 0,
							Devices: []vmopv1.VirtualDeviceStatus{
								{
									Type:       vmopv1.VirtualDeviceTypeDisk,
									UnitNumber: 0,
								},
							},
						},
					},
				}
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:           "cdrom1",
						ControllerType: vmopv1.VirtualControllerTypeIDE,
					},
				}
			})

			It("should avoid occupied slot used by non-CD-ROM device", func() {
				wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				// Should assign unit 1 since unit 0 is occupied by a disk.
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
				Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(Equal(ptr.To(int32(1))))
			})
		})

		Context("When SATA controller needs to be created", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:           "cdrom1",
						ControllerType: vmopv1.VirtualControllerTypeSATA,
					},
				}
			})

			It("should create SATA controller", func() {
				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
				assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
			})
		})

		Context("When VM has nil hardware spec", func() {
			It("should initialize hardware spec and assign controller", func() {
				// Set hardware to nil and CD-ROM specs for testing.
				vm.Spec.Hardware = nil
				vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					Cdrom: []vmopv1.VirtualMachineCdromSpec{
						{Name: "cdrom1"},
					},
				}

				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())
				Expect(vm.Spec.Hardware).ToNot(BeNil())
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
			})
		})

		Context("When VM has CD-ROM with unit number but incomplete controller info", func() {
			Context("Unit number with only controller type", func() {
				BeforeEach(func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						{
							Name:           "cdrom1",
							UnitNumber:     ptr.To(int32(0)),
							ControllerType: vmopv1.VirtualControllerTypeIDE,
							// ControllerBusNumber is set to nil for testing.
						},
					}
				})

				It("should skip assignment when unit number is set but controller bus number is missing", func() {
					wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
					Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(BeNil())
					Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(Equal(ptr.To(int32(0))))
				})
			})

			Context("Unit number with only bus number", func() {
				BeforeEach(func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						{
							Name:                "cdrom1",
							UnitNumber:          ptr.To(int32(0)),
							ControllerBusNumber: ptr.To(int32(0)),
							// ControllerType is left empty for testing.
						},
					}
				})

				It("should skip assignment when unit number is set but controller type is missing", func() {
					wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeFalse())
					Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(BeEmpty())
					Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
					Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(Equal(ptr.To(int32(0))))
				})
			})
		})

		Context("When no slots are available", func() {
			BeforeEach(func() {
				// Fill all IDE slots with non-CD-ROM devices to test slot exhaustion.
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0}, {BusNumber: 1},
				}
				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						// Fill all IDE slots (2 slots per controller, 2 controllers = 4 slots total) with disk devices.
						{
							Type:      vmopv1.VirtualControllerTypeIDE,
							BusNumber: 0,
							Devices: []vmopv1.VirtualDeviceStatus{
								{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 0},
								{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 1},
							},
						},
						{
							Type:      vmopv1.VirtualControllerTypeIDE,
							BusNumber: 1,
							Devices: []vmopv1.VirtualDeviceStatus{
								{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 0},
								{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 1},
							},
						},
					},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{Name: "cdrom1"},
				}
			})

			It("should fall back to SATA controller when IDE slots are full", func() {
				expectMutationSuccess()
				assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
				assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
			})
		})

		Context("When VM has mixed CD-ROM configurations", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Complete controller info should not be processed by the mutator.
					{
						Name:                "cdrom1",
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(0)),
					},
					// Incomplete info should be processed by the mutator.
					{Name: "cdrom2"},
					// Unit number with incomplete controller info should be skipped.
					{
						Name:           "cdrom3",
						UnitNumber:     ptr.To(int32(1)),
						ControllerType: vmopv1.VirtualControllerTypeIDE,
						// Missing ControllerBusNumber for testing incomplete info.
					},
				}
			})

			It("should only process CD-ROMs that need controller assignment", func() {
				wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())

				// cdrom1 should remain unchanged since it has complete info.
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
				Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(Equal(ptr.To(int32(0))))

				// cdrom2 should get assigned since it has incomplete info.
				Expect(vm.Spec.Hardware.Cdrom[1].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[1].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
				Expect(vm.Spec.Hardware.Cdrom[1].UnitNumber).To(Equal(ptr.To(int32(0))))

				// cdrom3 should remain unchanged since it has unit number with incomplete controller info.
				Expect(vm.Spec.Hardware.Cdrom[2].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[2].ControllerBusNumber).To(BeNil())
				Expect(vm.Spec.Hardware.Cdrom[2].UnitNumber).To(Equal(ptr.To(int32(1))))
			})
		})
	})
})
