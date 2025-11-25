// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

var _ = Describe("MutateCdromControllerOnUpdate", func() {
	var (
		ctx *builder.UnitTestContextForMutatingWebhook
		vm  *vmopv1.VirtualMachine
	)

	callMutator := func() (bool, error) {
		// Create oldVM with proper schema upgrade annotations at call time
		oldVM := vm.DeepCopy()
		if oldVM.Annotations == nil {
			oldVM.Annotations = make(map[string]string)
		}

		oldVM.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey] = pkgcfg.FromContext(ctx).BuildVersion
		oldVM.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey] = vmopv1.GroupVersion.Version
		return mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, oldVM)
	}

	assertCdromController := func(index int, controllerType vmopv1.VirtualControllerType, busNumber, unitNumber int32) {
		Expect(vm.Spec.Hardware.Cdrom[index].ControllerType).To(Equal(controllerType))
		Expect(vm.Spec.Hardware.Cdrom[index].ControllerBusNumber).To(Equal(ptr.To(busNumber)))
		Expect(vm.Spec.Hardware.Cdrom[index].UnitNumber).To(Equal(ptr.To(unitNumber)))
	}

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

	setupCdromSpecs := func(names ...string) {
		vm.Spec.Hardware.Cdrom = make([]vmopv1.VirtualMachineCdromSpec, len(names))
		for i, name := range names {
			vm.Spec.Hardware.Cdrom[i] = vmopv1.VirtualMachineCdromSpec{Name: name}
		}
	}

	cdromSpec := func(name string, controllerType vmopv1.VirtualControllerType, busNumber, unitNumber *int32) vmopv1.VirtualMachineCdromSpec {
		return builder.DummyCdromSpec(name, "", "", controllerType, busNumber, unitNumber, nil, nil)
	}

	expectMutationSuccess := func() {
		wasMutated, err := callMutator()
		Expect(err).ToNot(HaveOccurred())
		Expect(wasMutated).To(BeTrue())
	}

	expectNoMutation := func() {
		wasMutated, err := callMutator()
		Expect(err).ToNot(HaveOccurred())
		Expect(wasMutated).To(BeFalse())
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
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = false
			})
			setupCdromSpecs("cdrom1")
		})

		It("should not mutate", func() {
			expectNoMutation()
		})
	})

	Context("When VM schema has not been upgraded", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
			})
			setupCdromSpecs("cdrom1")
		})

		It("should not mutate when oldVM does not have upgrade annotations", func() {
			oldVM := vm.DeepCopy()
			oldVM.Annotations = nil

			wasMutated, err := mutation.MutateCdromControllerOnUpdate(&ctx.WebhookRequestContext, nil, vm, oldVM)
			Expect(err).ToNot(HaveOccurred())
			Expect(wasMutated).To(BeFalse())
		})

	})

	Context("When VMSharedDisks feature is enabled", func() {
		BeforeEach(func() {
			pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
				config.Features.VMSharedDisks = true
				config.BuildVersion = "v1"
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
			DescribeTable("should handle partial placement with controller type and bus number",
				func(ctrlType vmopv1.VirtualControllerType, setupController func()) {
					setupController()
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", ctrlType, ptr.To(int32(0)), nil),
					}
					expectMutationSuccess()
					assertCdromController(0, ctrlType, 0, 0)
				},
				Entry("IDE controller - creates controller when not exists",
					vmopv1.VirtualControllerTypeIDE,
					func() {}), // No setup needed, controller will be created
				Entry("IDE controller - uses existing controller",
					vmopv1.VirtualControllerTypeIDE,
					func() {
						vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{{BusNumber: 0}}
					}),
				Entry("SATA controller - creates controller when not exists",
					vmopv1.VirtualControllerTypeSATA,
					func() {}), // No setup needed, controller will be created
				Entry("SATA controller - uses existing controller",
					vmopv1.VirtualControllerTypeSATA,
					func() {
						vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{{BusNumber: 0}}
					}),
			)
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
			DescribeTable("should avoid occupied slots used by non-CD-ROM devices",
				func(ctrlType vmopv1.VirtualControllerType, setupController func()) {
					setupController()
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", ctrlType, ptr.To(int32(0)), nil),
					}
					vm.Status.Hardware.Controllers = []vmopv1.VirtualControllerStatus{
						{
							Type:      ctrlType,
							BusNumber: 0,
							Devices: []vmopv1.VirtualDeviceStatus{
								{Type: vmopv1.VirtualDeviceTypeDisk, UnitNumber: 0},
							},
						},
					}

					expectMutationSuccess()
					assertCdromController(0, ctrlType, 0, 1)
				},
				Entry("IDE controller",
					vmopv1.VirtualControllerTypeIDE,
					func() {
						vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{{BusNumber: 0}}
					}),
				Entry("SATA controller",
					vmopv1.VirtualControllerTypeSATA,
					func() {
						vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{{BusNumber: 0}}
					}),
			)
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
			It("should only process CD-ROMs that need controller assignment", func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
					{Name: "cdrom2"},
					cdromSpec("cdrom3", vmopv1.VirtualControllerTypeIDE, nil, ptr.To(int32(1))),
				}

				wasMutated, err := callMutator()
				Expect(err).ToNot(HaveOccurred())
				Expect(wasMutated).To(BeTrue())

				// cdrom1 should remain unchanged since it has complete info.
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)

				// cdrom2 should get unit 1 since unit 0 is now used by cdrom1.
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)

				// cdrom3 should remain unchanged since it has unit number with incomplete controller info.
				Expect(vm.Spec.Hardware.Cdrom[2].ControllerType).To(Equal(vmopv1.VirtualControllerTypeIDE))
				Expect(vm.Spec.Hardware.Cdrom[2].ControllerBusNumber).To(BeNil())
				Expect(vm.Spec.Hardware.Cdrom[2].UnitNumber).To(Equal(ptr.To(int32(1))))
			})
		})

		Context("When explicit and implicit placements are mixed", func() {
			It("should process explicit placements first to reserve slots", func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Implicit - should be processed after explicit
					{Name: "cdrom1"},
					// Explicit - should reserve IDE 0:1
					cdromSpec("cdrom2", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(1))),
					// Implicit - should be processed after explicit
					{Name: "cdrom3"},
				}

				expectMutationSuccess()
				// cdrom1 should get IDE 0:0 (first available slot)
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				// cdrom2 should keep its explicit placement IDE 0:1
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)
				// cdrom3 should get IDE 1:0 (IDE 0 is full, create new controller)
				assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 1, 0)
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
			})

			It("should handle multiple explicit placements with gaps for implicit", func() {
				vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Explicit placements creating gaps: slots 0, 3, 5
					cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(0))),
					cdromSpec("cdrom2", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(3))),
					cdromSpec("cdrom3", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(5))),
					// Implicit placements - will try IDE first (preferred), not fill SATA gaps
					{Name: "cdrom4"},
					{Name: "cdrom5"},
				}

				expectMutationSuccess()
				assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 0)
				assertCdromController(1, vmopv1.VirtualControllerTypeSATA, 0, 3)
				assertCdromController(2, vmopv1.VirtualControllerTypeSATA, 0, 5)
				// cdrom4 should get IDE 0:0 (implicit CDs prefer IDE first)
				assertCdromController(3, vmopv1.VirtualControllerTypeIDE, 0, 0)
				// cdrom5 should get IDE 0:1
				assertCdromController(4, vmopv1.VirtualControllerTypeIDE, 0, 1)
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 1, 0)
			})

			It("should handle explicit placements on non-existent controllers", func() {
				// No controllers in spec initially
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Implicit
					{Name: "cdrom1"},
					// Explicit on non-existent SATA controller
					cdromSpec("cdrom2", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(0))),
					// Implicit - should prefer IDE first
					{Name: "cdrom3"},
					// Explicit on non-existent IDE controller
					cdromSpec("cdrom4", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(1)), ptr.To(int32(0))),
				}

				expectMutationSuccess()
				// cdrom1 should get IDE 1:1 (explicit created IDE 1, so reuse it with available slot 1)
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 1, 1)
				// cdrom2 keeps its explicit SATA 0:0 (controller created)
				assertCdromController(1, vmopv1.VirtualControllerTypeSATA, 0, 0)
				// cdrom3 should get IDE 0:0 (no more slots on IDE 1, create IDE 0)
				assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 0, 0)
				// cdrom4 keeps its explicit IDE 1:0 (controller created)
				assertCdromController(3, vmopv1.VirtualControllerTypeIDE, 1, 0)
				// IDE controllers: created in order [1, 0] because explicit (IDE 1) processed first
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 1, 0)
				assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
			})

			It("should handle explicit placements across multiple controller types", func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Explicit IDE placements - fill IDE 0 completely
					cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
					cdromSpec("cdrom2", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(1))),
					// Explicit SATA placement
					cdromSpec("cdrom3", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(10))),
					// Implicit - IDE is full, should use SATA
					{Name: "cdrom4"},
					{Name: "cdrom5"},
				}

				expectMutationSuccess()
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 0, 0)
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)
				assertCdromController(2, vmopv1.VirtualControllerTypeSATA, 0, 10)
				// cdrom4 should get IDE 1:0 (IDE 0 is full, create IDE 1)
				assertCdromController(3, vmopv1.VirtualControllerTypeIDE, 1, 0)
				// cdrom5 should get IDE 1:1
				assertCdromController(4, vmopv1.VirtualControllerTypeIDE, 1, 1)
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
			})

			It("should correctly track slots when explicit placement is on different bus", func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
					{BusNumber: 1},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Explicit placement on IDE bus 1
					cdromSpec("cdrom1", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(1)), ptr.To(int32(0))),
					// Implicit - should prefer bus 0 (lower bus number)
					{Name: "cdrom2"},
					{Name: "cdrom3"},
					// Explicit placement on IDE bus 0
					cdromSpec("cdrom4", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
					// Implicit
					{Name: "cdrom5"},
				}

				expectMutationSuccess()
				// After processing: IDE 0 has slots 0,1 occupied; IDE 1 has slots 0,1 occupied
				// IDE is exhausted (max 2 controllers, each with 2 slots = 4 total, all used)
				// cdrom5 must fall back to SATA
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 1, 0)
				// cdrom2 should get IDE 0:1 (explicit cdrom4 will take 0:0, so first implicit gets 0:1)
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 1)
				// cdrom3 should get IDE 1:1 (bus 0 is full, bus 1 has slot 1 available)
				assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 1, 1)
				// cdrom4 keeps explicit IDE 0:0
				assertCdromController(3, vmopv1.VirtualControllerTypeIDE, 0, 0)
				// cdrom5 should fallback to SATA 0:0 (all IDE exhausted)
				assertCdromController(4, vmopv1.VirtualControllerTypeSATA, 0, 0)
				// IDE controllers exist, SATA controller created
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
				assertControllerCreated(vmopv1.VirtualControllerTypeSATA, 1, 0)
			})

			It("should handle explicit placements filling up entire controller", func() {
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Implicit
					{Name: "cdrom1"},
					// Explicit placements filling IDE 0 completely (slots 0 and 1)
					cdromSpec("cdrom2", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(0))),
					cdromSpec("cdrom3", vmopv1.VirtualControllerTypeIDE, ptr.To(int32(0)), ptr.To(int32(1))),
					// Implicit - should create new controller
					{Name: "cdrom4"},
				}

				expectMutationSuccess()
				// cdrom1 should get IDE 1:0 (bus 0 will be full after explicit placements)
				assertCdromController(0, vmopv1.VirtualControllerTypeIDE, 1, 0)
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 0)
				assertCdromController(2, vmopv1.VirtualControllerTypeIDE, 0, 1)
				// cdrom4 should get IDE 1:1
				assertCdromController(3, vmopv1.VirtualControllerTypeIDE, 1, 1)
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 2, 0, 1)
			})

			It("should process explicit placements even when mixed with partial specs", func() {
				vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
					{BusNumber: 0},
				}
				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					// Explicit
					cdromSpec("cdrom1", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(5))),
					// Implicit - will try IDE first (preferred)
					{Name: "cdrom2"},
					// Partial (has controller and bus, missing unit) - implicit
					cdromSpec("cdrom3", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), nil),
					// Explicit
					cdromSpec("cdrom4", vmopv1.VirtualControllerTypeSATA, ptr.To(int32(0)), ptr.To(int32(2))),
					// Implicit
					{Name: "cdrom5"},
				}

				expectMutationSuccess()
				assertCdromController(0, vmopv1.VirtualControllerTypeSATA, 0, 5)
				// cdrom2 should get IDE 0:0 (implicit prefers IDE)
				assertCdromController(1, vmopv1.VirtualControllerTypeIDE, 0, 0)
				// cdrom3 should get SATA 0:0 (partial with SATA specified, first available)
				assertCdromController(2, vmopv1.VirtualControllerTypeSATA, 0, 0)
				assertCdromController(3, vmopv1.VirtualControllerTypeSATA, 0, 2)
				// cdrom5 should get IDE 0:1 (implicit prefers IDE)
				assertCdromController(4, vmopv1.VirtualControllerTypeIDE, 0, 1)
				assertControllerCreated(vmopv1.VirtualControllerTypeIDE, 1, 0)
			})
		})

		Context("Controller slot exhaustion", func() {
			It("should skip CD-ROM when both IDE and SATA controllers are full", func() {
				// Fill all IDE controllers (2 controllers × 2 slots = 4 slots)
				vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{
					{BusNumber: 0}, {BusNumber: 1},
				}
				// Fill all SATA controllers (4 controllers × 30 slots = 120 slots)
				// We'll simulate by filling slots 0-29 on each of the 4 SATA controllers
				vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
					{BusNumber: 0}, {BusNumber: 1}, {BusNumber: 2}, {BusNumber: 3},
				}

				// Fill all IDE slots with non-CD-ROM devices
				ideControllers := []vmopv1.VirtualControllerStatus{
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
				}

				// Fill all SATA slots with non-CD-ROM devices
				sataControllers := []vmopv1.VirtualControllerStatus{}
				for busNum := int32(0); busNum < 4; busNum++ {
					devices := []vmopv1.VirtualDeviceStatus{}
					for unitNum := int32(0); unitNum < 30; unitNum++ {
						devices = append(devices, vmopv1.VirtualDeviceStatus{
							Type:       vmopv1.VirtualDeviceTypeDisk,
							UnitNumber: unitNum,
						})
					}
					sataControllers = append(sataControllers, vmopv1.VirtualControllerStatus{
						Type:      vmopv1.VirtualControllerTypeSATA,
						BusNumber: busNum,
						Devices:   devices,
					})
				}

				vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: append(ideControllers, sataControllers...),
				}

				vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{Name: "cdrom1"},
				}

				expectNoMutation()
				// CD-ROM should remain unassigned
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(BeEmpty())
				Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(BeNil())
				Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(BeNil())
			})
		})

		Context("Partial placement with full controller", func() {
			DescribeTable("should skip CD-ROM when specified controller is full",
				func(ctrlType vmopv1.VirtualControllerType, maxSlots int32, setupController func()) {
					setupController()

					// Fill all slots with non-CD-ROM devices
					devices := []vmopv1.VirtualDeviceStatus{}
					for unitNum := int32(0); unitNum < maxSlots; unitNum++ {
						devices = append(devices, vmopv1.VirtualDeviceStatus{
							Type:       vmopv1.VirtualDeviceTypeDisk,
							UnitNumber: unitNum,
						})
					}
					vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
						Controllers: []vmopv1.VirtualControllerStatus{
							{
								Type:      ctrlType,
								BusNumber: 0,
								Devices:   devices,
							},
						},
					}
					vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
						cdromSpec("cdrom1", ctrlType, ptr.To(int32(0)), nil),
					}

					expectNoMutation()
					Expect(vm.Spec.Hardware.Cdrom[0].ControllerType).To(Equal(ctrlType))
					Expect(vm.Spec.Hardware.Cdrom[0].ControllerBusNumber).To(Equal(ptr.To(int32(0))))
					Expect(vm.Spec.Hardware.Cdrom[0].UnitNumber).To(BeNil())
				},
				Entry("IDE controller with 2 slots full",
					vmopv1.VirtualControllerTypeIDE,
					int32(2),
					func() {
						vm.Spec.Hardware.IDEControllers = []vmopv1.IDEControllerSpec{{BusNumber: 0}}
					}),
				Entry("SATA controller with 30 slots full",
					vmopv1.VirtualControllerTypeSATA,
					int32(30),
					func() {
						vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{{BusNumber: 0}}
					}),
			)
		})

	})
})
