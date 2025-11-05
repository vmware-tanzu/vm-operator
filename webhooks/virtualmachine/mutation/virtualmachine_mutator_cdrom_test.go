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

	// Helper function to create a CD-ROM spec with placement info.
	cdromSpec := func(name string, controllerType vmopv1.VirtualControllerType, busNumber, unitNumber *int32) vmopv1.VirtualMachineCdromSpec {
		return builder.DummyCdromSpec(name, "", "", controllerType, busNumber, unitNumber, nil, nil)
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
			setupCdromSpecs("cdrom1")
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

		Context("When VM has CD-ROM without controller info", func() {
			BeforeEach(func() {
				setupCdromSpecs("cdrom1")
			})

			It("should assign IDE controller by default", func() {
				expectMutationSuccess()
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
			})
		})

		Context("When VM has CD-ROM with partial controller info", func() {
			DescribeTable("should not mutate",
				func(setupCdrom func() vmopv1.VirtualMachineCdromSpec) {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{setupCdrom()}
					expectNoMutation()
				},
				Entry("when only controller type is set",
					func() vmopv1.VirtualMachineCdromSpec {
						return cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, nil, nil)
					}),
				Entry("when only bus number is set",
					func() vmopv1.VirtualMachineCdromSpec {
						return cdromSpec("cdrom1", "", ptr.To(int32(0)), nil)
					}),
				Entry("when only unit number is set",
					func() vmopv1.VirtualMachineCdromSpec {
						return cdromSpec("cdrom1", "", nil, ptr.To(int32(0)))
					}),
			)
		})

		Context("When VM has CD-ROM with controller type and bus number but no unit number", func() {
			Context("IDE controller", func() {
				It("should add controller and assign unit number when controller does not exist", func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), nil),
					}
					expectMutationSuccess()
					// Controller should be added to spec.
					assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 1, 0)
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				})

				It("should assign unit number when controller exists in spec", func() {
					vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), nil),
					}
					expectMutationSuccess()
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				})
			})

			Context("SATA controller", func() {
				It("should add controller and assign unit number when controller does not exist", func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), nil),
					}
					expectMutationSuccess()
					// Controller should be added to spec.
					assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
				})

				It("should assign unit number when controller exists in spec", func() {
					vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), nil),
					}
					expectMutationSuccess()
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
				})
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

		Context("When VM has controllers in status with occupied slots", func() {
			Context("IDE controller", func() {
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
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), nil),
					}
				})

				It("should avoid occupied slot used by non-CD-ROM device", func() {
					wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					// Should assign unit 1 since unit 0 is occupied by a disk.
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 1)
				})
			})

			Context("SATA controller", func() {
				BeforeEach(func() {
					vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
						Controllers: []vmopv1.VirtualControllerStatus{
							{
								Type:      vmopv1.VirtualControllerTypeSATA,
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
					vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), nil),
					}
				})

				It("should avoid occupied slot used by non-CD-ROM device", func() {
					wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, nil)
					Expect(err).ToNot(HaveOccurred())
					Expect(wasMutated).To(BeTrue())
					// Should assign unit 1 since unit 0 is occupied by a disk.
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 1)
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

		Context("When VM has CD-ROM with complete controller info", func() {
			Context("IDE controller", func() {
				It("should not mutate when CD-ROM has complete placement info", func() {
					vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
					}

					// Should return false because CD-ROM already has complete placement.
					expectNoMutation()
					// cdrom1 should keep its explicit placement unchanged.
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				})

				It("should update usedSlotMap when all placement values are specified", func() {
					vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
						// This second CD-ROM should get unit 1 since unit 0 is now used.
						cdromSpec("cdrom2", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), nil),
					}

					expectMutationSuccess()
					// cdrom1 should keep its explicit placement.
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
					// cdrom2 should get unit 1 since unit 0 is used by cdrom1.
					assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)
				})

				It("should handle multiple CD-ROMs with explicit placement updating usedSlotMap", func() {
					vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(1))),
						cdromSpec("cdrom2", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
						// This should be auto-assigned and skip the occupied slots.
						{Name: "cdrom3"},
					}

					expectMutationSuccess()
					// cdrom1 and cdrom2 should keep their explicit placements.
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 1)
					assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 0)
					// cdrom3 should get a new IDE controller since bus 0 is full.
					assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 1, 0)
					assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
				})
			})

			Context("SATA controller", func() {
				It("should not mutate when CD-ROM has complete placement info", func() {
					vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(5))),
					}

					// Should return false because CD-ROM already has complete placement.
					expectNoMutation()
					// cdrom1 should keep its explicit placement unchanged.
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 5)
				})

				It("should update usedSlotMap when all placement values are specified", func() {
					vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(5))),
						// This second CD-ROM should get unit 0 since we start from 0.
						cdromSpec("cdrom2", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), nil),
					}

					expectMutationSuccess()
					// cdrom1 should keep its explicit placement.
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 5)
					// cdrom2 should get unit 0 (first available slot).
					assertCdromController(1, vmopv1.VirtualControllerTypeSATA, 0, 0)
				})

				It("should handle multiple SATA CD-ROMs with explicit placement", func() {
					vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(0))),
						cdromSpec("cdrom2", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(2))),
						// This should get unit 1 (between the two occupied slots).
						cdromSpec("cdrom3", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), nil),
					}

					expectMutationSuccess()
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
					assertCdromController(1, vmopv1.VirtualControllerTypeSATA, 0, 2)
					// cdrom3 should get unit 1 (first available after 0).
					assertCdromController(2, vmopv1.VirtualControllerTypeSATA, 0, 1)
				})
			})

			Context("Mixed IDE and SATA controllers", func() {
				It("should properly track slots across different controller types", func() {
					vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						// Explicit IDE placement.
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
						// Explicit SATA placement.
						cdromSpec("cdrom2", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(0))),
						// Auto-assign should get IDE bus 0, unit 1.
						{Name: "cdrom3"},
					}

					expectMutationSuccess()
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
					assertCdromController(1, vmopv1.VirtualControllerTypeSATA, 0, 0)
					// cdrom3 should use IDE bus 0, unit 1 (IDE is tried first).
					assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 0, 1)
				})
			})

			Context("When controller doesn't exist in spec", func() {
				It("should add IDE controller when CD-ROM has complete placement info", func() {
					// No controllers in spec initially.
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
						// This should get unit 1 since unit 0 is used.
						{Name: "cdrom2"},
					}

					expectMutationSuccess()
					// Controller should be added to spec.
					assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 1, 0)
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
					assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)
				})

				It("should add SATA controller when CD-ROM has complete placement info", func() {
					// No controllers in spec initially.
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(3))),
						// This should get IDE bus 0, unit 0 (IDE is tried first).
						{Name: "cdrom2"},
					}

					expectMutationSuccess()
					// SATA controller should be added for cdrom1.
					assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
					// IDE controller should be added for cdrom2.
					assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 1, 0)
					assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 3)
					assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 0)
				})

				It("should add multiple controllers for multiple CD-ROMs with complete placement", func() {
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
						cdromSpec("cdrom2", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(1)), ptr.To(int32(0))),
						cdromSpec("cdrom3", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(0))),
					}

					expectMutationSuccess()
					// Should create 2 IDE controllers and 1 SATA controller.
					assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
					assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
					assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
					assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 1, 0)
					assertCdromController(2, vmopv1.VirtualControllerTypeSATA, 0, 0)
				})
			})
		})

		Context("When VM has mixed CD-ROM configurations", func() {
			BeforeEach(func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Complete controller info is processed to update usedSlotMap.
					cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
					// Incomplete info should be processed by the mutator.
					{Name: "cdrom2"},
					// Unit number with incomplete controller info should be skipped.
					cdromSpec("cdrom3", vmopv1.VirtualControllerTypeIDE, nil, ptr.To(int32(1))),
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

				// cdrom2 should get unit 1 since unit 0 is now used by cdrom1.
				Expect(vm.Spec.Hardware.Cdrom[1].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[1].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
				Expect(vm.Spec.Hardware.Cdrom[1].UnitNumber).To(Equal(ptr.To(int32(1))))

				// cdrom3 should remain unchanged since it has unit number with incomplete controller info.
				Expect(vm.Spec.Hardware.Cdrom[2].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[2].ControllerBusNumber).To(BeNil())
				Expect(vm.Spec.Hardware.Cdrom[2].UnitNumber).To(Equal(ptr.To(int32(1))))
			})
		})
	})
})
