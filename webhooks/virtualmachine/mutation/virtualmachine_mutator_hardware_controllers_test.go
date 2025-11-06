// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

const (
	dummyVMName = "vm-123"
)

func controllerTests() {
	Describe(
		"AddControllersForVolumes",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		controllerMutationTests,
	)
}

// controllerTestParams defines the parameters for controller type agnostic tests.
type controllerTestParams struct {
	controllerType   vmopv1.VirtualControllerType
	getControllers   func(*vmopv1.VirtualMachineHardwareSpec) int
	getController    func(*vmopv1.VirtualMachineHardwareSpec, int) vmopv1util.ControllerSpec
	setControllers   func(*vmopv1.VirtualMachineHardwareSpec, []vmopv1util.ControllerSpec)
	createController func(int32, vmopv1.VirtualControllerSharingMode) vmopv1util.ControllerSpec
}

// controllerTestParamsForType returns the test parameters for a given controller type.
func controllerTestParamsForType(controllerType vmopv1.VirtualControllerType) controllerTestParams {
	switch controllerType {
	case vmopv1.VirtualControllerTypeSCSI:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeSCSI,
			getControllers: func(hw *vmopv1.VirtualMachineHardwareSpec) int {
				return len(hw.SCSIControllers)
			},
			getController: func(hw *vmopv1.VirtualMachineHardwareSpec, idx int) vmopv1util.ControllerSpec {
				return hw.SCSIControllers[idx]
			},
			setControllers: func(hw *vmopv1.VirtualMachineHardwareSpec, controllers []vmopv1util.ControllerSpec) {
				hw.SCSIControllers = make([]vmopv1.SCSIControllerSpec, len(controllers))
				for i, c := range controllers {
					hw.SCSIControllers[i] = c.(vmopv1.SCSIControllerSpec)
				}
			},
			createController: func(busNum int32, sharingMode vmopv1.VirtualControllerSharingMode) vmopv1util.ControllerSpec {
				return vmopv1.SCSIControllerSpec{
					BusNumber:   busNum,
					Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
					SharingMode: sharingMode,
				}
			},
		}
	case vmopv1.VirtualControllerTypeSATA:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeSATA,
			getControllers: func(hw *vmopv1.VirtualMachineHardwareSpec) int {
				return len(hw.SATAControllers)
			},
			getController: func(hw *vmopv1.VirtualMachineHardwareSpec, idx int) vmopv1util.ControllerSpec {
				return hw.SATAControllers[idx]
			},
			setControllers: func(hw *vmopv1.VirtualMachineHardwareSpec, controllers []vmopv1util.ControllerSpec) {
				hw.SATAControllers = make([]vmopv1.SATAControllerSpec, len(controllers))
				for i, c := range controllers {
					hw.SATAControllers[i] = c.(vmopv1.SATAControllerSpec)
				}
			},
			createController: func(busNum int32, sharingMode vmopv1.VirtualControllerSharingMode) vmopv1util.ControllerSpec {
				return vmopv1.SATAControllerSpec{
					BusNumber: busNum,
				}
			},
		}
	case vmopv1.VirtualControllerTypeNVME:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeNVME,
			getControllers: func(hw *vmopv1.VirtualMachineHardwareSpec) int {
				return len(hw.NVMEControllers)
			},
			getController: func(hw *vmopv1.VirtualMachineHardwareSpec, idx int) vmopv1util.ControllerSpec {
				return hw.NVMEControllers[idx]
			},
			setControllers: func(hw *vmopv1.VirtualMachineHardwareSpec, controllers []vmopv1util.ControllerSpec) {
				hw.NVMEControllers = make([]vmopv1.NVMEControllerSpec, len(controllers))
				for i, c := range controllers {
					hw.NVMEControllers[i] = c.(vmopv1.NVMEControllerSpec)
				}
			},
			createController: func(busNum int32, sharingMode vmopv1.VirtualControllerSharingMode) vmopv1util.ControllerSpec {
				return vmopv1.NVMEControllerSpec{
					BusNumber:   busNum,
					SharingMode: sharingMode,
				}
			},
		}
	case vmopv1.VirtualControllerTypeIDE:
		return controllerTestParams{
			controllerType: vmopv1.VirtualControllerTypeIDE,
			getControllers: func(hw *vmopv1.VirtualMachineHardwareSpec) int {
				return len(hw.IDEControllers)
			},
			getController: func(hw *vmopv1.VirtualMachineHardwareSpec, idx int) vmopv1util.ControllerSpec {
				return hw.IDEControllers[idx]
			},
			setControllers: func(hw *vmopv1.VirtualMachineHardwareSpec, controllers []vmopv1util.ControllerSpec) {
				hw.IDEControllers = make([]vmopv1.IDEControllerSpec, len(controllers))
				for i, c := range controllers {
					hw.IDEControllers[i] = c.(vmopv1.IDEControllerSpec)
				}
			},
			createController: func(busNum int32, sharingMode vmopv1.VirtualControllerSharingMode) vmopv1util.ControllerSpec {
				return vmopv1.IDEControllerSpec{
					BusNumber: busNum,
				}
			},
		}
	}
	return controllerTestParams{}
}

func controllerMutationTests() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
		})
	})

	testNonPVCVolumes(func() *unitMutationWebhookContext { return ctx })
	testControllerTypeAgnostic(func() *unitMutationWebhookContext { return ctx })
	testSCSISharingMode(func() *unitMutationWebhookContext { return ctx })
	testMultipleControllerTypes(func() *unitMutationWebhookContext { return ctx })
}

func testNonPVCVolumes(getCtx func() *unitMutationWebhookContext) {
	Context("Non-PVC volumes", func() {
		var ctx *unitMutationWebhookContext

		BeforeEach(func() {
			ctx = getCtx()
			ctx.vm.Status.UniqueID = dummyVMName
			ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "unmanaged-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{},
						},
					},
				},
			}
		})

		It("should not add controllers for unmanaged PVC volumes", func() {
			mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())
		})
	})
}

func testControllerTypeAgnostic(getCtx func() *unitMutationWebhookContext) {
	// Controller Type Agnostic Tests - these tests run for all controller types.
	Context("Controller Type Agnostic Tests", func() {
		var ctx *unitMutationWebhookContext

		BeforeEach(func() {
			ctx = getCtx()
			ctx.vm.Status.UniqueID = dummyVMName
			ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}
			ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
		})

		DescribeTable("should create controller and assign placement when no controllers exist",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "test-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
								ControllerType: controllerType,
							},
						},
					},
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have created one controller.
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(1))
				controller := params.getController(ctx.vm.Spec.Hardware, 0)
				Expect(vmopv1util.GenerateControllerID(controller).BusNumber).To(Equal(int32(1)))

				// Volume should have placement assigned.
				pvc := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc.ControllerType).To(Equal(controllerType))
				Expect(pvc.ControllerBusNumber).ToNot(BeNil())
				Expect(*pvc.ControllerBusNumber).To(Equal(int32(1)))
				Expect(pvc.UnitNumber).ToNot(BeNil())
				Expect(*pvc.UnitNumber).To(Equal(int32(0)))
			},
			Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should create controller with default type when controllerType not specified",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(vmopv1.VirtualControllerTypeSCSI)

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "test-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
								// ControllerType not specified - should default to SCSI.
							},
						},
					},
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have created one SCSI controller.
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(1))

				// Volume should have SCSI type assigned.
				pvc := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
			},
			Entry("defaults to SCSI", vmopv1.VirtualControllerTypeSCSI),
		)

		DescribeTable("should reuse existing controller with available slots",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create an existing controller with available slots.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{
					params.createController(0, vmopv1.VirtualControllerSharingModeNone),
				})

				// Add existing volume in spec at unit 0 to show controller has some capacity used.
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "existing-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "existing-pvc",
								},
								ControllerType:      controllerType,
								ControllerBusNumber: ptr.To(int32(0)),
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
					{
						Name: "test-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
								ControllerType: controllerType,
							},
						},
					},
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should still have only one controller.
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(1))

				// New volume should be assigned to existing controller.
				pvc := ctx.vm.Spec.Volumes[1].PersistentVolumeClaim
				Expect(*pvc.ControllerBusNumber).To(Equal(int32(0)))
				Expect(pvc.UnitNumber).ToNot(BeNil())
				Expect(*pvc.UnitNumber).To(Equal(int32(1))) // Unit 0 is occupied by existing volume.
			},
			Entry("SCSI controller", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA controller", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME controller", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE controller", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should create additional controller when first is at capacity",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create first controller.
				firstController := params.createController(0, vmopv1.VirtualControllerSharingModeNone)
				maxSlots := firstController.MaxSlots()
				reservedUnit := firstController.ReservedUnitNumber()

				// Setup hardware with first controller.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{firstController})

				// Fill first controller to capacity using volumes in spec.
				for i := int32(0); i < maxSlots; i++ {
					if i != reservedUnit {
						ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
							Name: fmt.Sprintf("existing-vol-%d", i),
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: fmt.Sprintf("existing-pvc-%d", i),
									},
									ControllerType:      controllerType,
									ControllerBusNumber: ptr.To(int32(0)),
									UnitNumber:          ptr.To(i),
								},
							},
						})
					}
				}

				// Add new volume.
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "test-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
							ControllerType: controllerType,
						},
					},
				})

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have 2 controllers now.
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(2))

				// Second controller should be at bus 1.
				secondController := params.getController(ctx.vm.Spec.Hardware, 1)
				Expect(vmopv1util.GenerateControllerID(secondController).BusNumber).To(Equal(int32(1)))

				// New volume should be assigned to new controller.
				pvc := ctx.vm.Spec.Volumes[len(ctx.vm.Spec.Volumes)-1].PersistentVolumeClaim
				Expect(*pvc.ControllerBusNumber).To(Equal(int32(1)))
				Expect(*pvc.UnitNumber).To(Equal(int32(0)))
			},
			Entry("SCSI controller at capacity", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA controller at capacity", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME controller at capacity", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE controller at capacity", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should respect MaxCount limit for each controller type",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create a controller to get MaxCount.
				sampleController := params.createController(0, vmopv1.VirtualControllerSharingModeNone)
				maxPerVM := sampleController.MaxCount()

				// Setup hardware with maximum controllers.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}

				// Create max controllers and fill them all using volumes in spec.
				controllers := make([]vmopv1util.ControllerSpec, 0, maxPerVM)
				for busNum := int32(0); busNum < maxPerVM; busNum++ {
					controller := params.createController(busNum, vmopv1.VirtualControllerSharingModeNone)
					controllers = append(controllers, controller)

					// Fill controller to capacity using volumes in spec.
					maxSlots := controller.MaxSlots()
					reservedUnit := controller.ReservedUnitNumber()
					for i := int32(0); i < maxSlots; i++ {
						if i != reservedUnit {
							ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
								Name: fmt.Sprintf("existing-vol-%d-%d", busNum, i),
								VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
									PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
										PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
											ClaimName: fmt.Sprintf("existing-pvc-%d-%d", busNum, i),
										},
										ControllerType:      controllerType,
										ControllerBusNumber: ptr.To(busNum),
										UnitNumber:          ptr.To(i),
									},
								},
							})
						}
					}
				}
				params.setControllers(ctx.vm.Spec.Hardware, controllers)

				// Try to add a new volume.
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "test-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "test-pvc",
							},
							ControllerType: controllerType,
						},
					},
				})

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())

				// Should still have maxPerVM controllers (no new one added).
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(int(maxPerVM)))

				// New volume should not have placement assigned.
				pvc := ctx.vm.Spec.Volumes[len(ctx.vm.Spec.Volumes)-1].PersistentVolumeClaim
				Expect(pvc.ControllerBusNumber).To(BeNil())
				Expect(pvc.UnitNumber).To(BeNil())
			},
			Entry("SCSI MaxCount limit", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA MaxCount limit", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME MaxCount limit", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE MaxCount limit", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should respect explicit controllerBusNumber",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "test-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
								ControllerType:      controllerType,
								ControllerBusNumber: ptr.To(int32(1)),
							},
						},
					},
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have created controller at bus 1.
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(1))
				controller := params.getController(ctx.vm.Spec.Hardware, 0)
				Expect(vmopv1util.GenerateControllerID(controller).BusNumber).To(Equal(int32(1)))

				// Volume should be assigned to bus 1.
				pvc := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(*pvc.ControllerBusNumber).To(Equal(int32(1)))
			},
			Entry("SCSI explicit bus number", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA explicit bus number", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME explicit bus number", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE explicit bus number", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should not modify existing controllerBusNumber",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create existing controller at bus 1.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{
					params.createController(1, vmopv1.VirtualControllerSharingModeNone),
				})

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "test-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
								ControllerType:      controllerType,
								ControllerBusNumber: ptr.To(int32(1)),
							},
						},
					},
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should still have only one controller.
				Expect(params.getControllers(ctx.vm.Spec.Hardware)).To(Equal(1))

				// Volume should still be assigned to bus 1.
				pvc := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(*pvc.ControllerBusNumber).To(Equal(int32(1)))
				Expect(pvc.UnitNumber).ToNot(BeNil())
			},
			Entry("SCSI existing bus number", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA existing bus number", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME existing bus number", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE existing bus number", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should assign sequential unit numbers",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create existing controller.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				controller := params.createController(0, vmopv1.VirtualControllerSharingModeNone)
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{controller})

				// Determine how many volumes to add based on controller capacity.
				maxSlots := controller.MaxSlots()
				reservedUnit := controller.ReservedUnitNumber()
				numVolumes := int(maxSlots)
				if reservedUnit >= 0 {
					numVolumes-- // One slot is reserved.
				}
				// Use minimum of 3 or available slots.
				if numVolumes > 3 {
					numVolumes = 3
				}

				// Add volumes.
				for i := 0; i < numVolumes; i++ {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name: "vol" + string(rune('1'+i)),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc" + string(rune('1'+i)),
								},
								ControllerType: controllerType,
							},
						},
					})
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Build expected unit numbers.
				expectedUnits := make([]int32, 0, numVolumes)
				for i := int32(0); len(expectedUnits) < numVolumes; i++ {
					if i != reservedUnit {
						expectedUnits = append(expectedUnits, i)
					}
				}

				// All volumes should have sequential unit numbers.
				for i := 0; i < numVolumes; i++ {
					pvc := ctx.vm.Spec.Volumes[i].PersistentVolumeClaim
					Expect(pvc.UnitNumber).ToNot(BeNil())
					Expect(*pvc.UnitNumber).To(Equal(expectedUnits[i]))
				}
			},
			Entry("SCSI sequential units", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA sequential units", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME sequential units", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE sequential units", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should skip reserved unit numbers",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create existing controller.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				controller := params.createController(0, vmopv1.VirtualControllerSharingModeNone)
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{controller})

				reservedUnit := controller.ReservedUnitNumber()
				if reservedUnit < 0 {
					Skip("Controller type has no reserved unit number")
				}

				// Add volumes to fill slots around the reserved unit.
				numVolumes := 3
				for i := 0; i < numVolumes; i++ {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name: "vol" + string(rune('1'+i)),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc" + string(rune('1'+i)),
								},
								ControllerType: controllerType,
							},
						},
					})
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Verify no volume is assigned to the reserved unit.
				for i := 0; i < numVolumes; i++ {
					pvc := ctx.vm.Spec.Volumes[i].PersistentVolumeClaim
					Expect(pvc.UnitNumber).ToNot(BeNil())
					Expect(*pvc.UnitNumber).ToNot(Equal(reservedUnit))
				}
			},
			Entry("SCSI reserved unit 7", vmopv1.VirtualControllerTypeSCSI),
		)

		DescribeTable("should ignore disk devices in status and only use spec.volumes",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create existing controller.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				controller := params.createController(0, vmopv1.VirtualControllerSharingModeNone)
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{controller})

				// Add status with disk device at unit 0 (should be ignored).
				// This represents the disk from spec.volumes that's already attached.
				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      controllerType,
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

				// Add new volume without unit number.
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "new-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "new-pvc",
							},
							ControllerType:      controllerType,
							ControllerBusNumber: ptr.To(int32(0)),
						},
					},
				})

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// New volume should get unit 0. Unit 0 in status devices is
				// ignored because it is not the source of truth.
				pvc := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(pvc.UnitNumber).ToNot(BeNil())
				Expect(*pvc.UnitNumber).To(Equal(int32(0)))
			},
			Entry("SCSI ignores disk devices in status", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA ignores disk devices in status", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME ignores disk devices in status", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE ignores disk devices in status", vmopv1.VirtualControllerTypeIDE),
		)

		DescribeTable("should not change existing unit number",
			func(controllerType vmopv1.VirtualControllerType) {
				params := controllerTestParamsForType(controllerType)

				// Create existing controller.
				if ctx.vm.Spec.Hardware == nil {
					ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
				}
				params.setControllers(ctx.vm.Spec.Hardware, []vmopv1util.ControllerSpec{
					params.createController(0, vmopv1.VirtualControllerSharingModeNone),
				})

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "test-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "test-pvc",
								},
								ControllerType:      controllerType,
								ControllerBusNumber: ptr.To(int32(0)),
								UnitNumber:          ptr.To(int32(5)),
							},
						},
					},
				}

				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())

				// Unit number should remain unchanged.
				pvc := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(*pvc.UnitNumber).To(Equal(int32(5)))
			},
			Entry("SCSI existing unit number", vmopv1.VirtualControllerTypeSCSI),
			Entry("SATA existing unit number", vmopv1.VirtualControllerTypeSATA),
			Entry("NVME existing unit number", vmopv1.VirtualControllerTypeNVME),
			Entry("IDE existing unit number", vmopv1.VirtualControllerTypeIDE),
		)
	})
}

func testSCSISharingMode(getCtx func() *unitMutationWebhookContext) {
	// SCSI-specific tests for sharing modes.
	Context("SCSI Sharing Mode Tests", func() {
		var ctx *unitMutationWebhookContext

		BeforeEach(func() {
			ctx = getCtx()
			ctx.vm.Status.UniqueID = dummyVMName
		})

		When("volume with OracleRAC application type", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "oracle-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "oracle-pvc",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
							},
						},
					},
				}
			})

			It("should add controller with sharingMode=None", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.SCSIControllers[0].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
			})
		})

		When("volume with MicrosoftWSFC application type", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "wsfc-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "wsfc-pvc",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeMicrosoftWSFC,
							},
						},
					},
				}
			})

			It("should add controller with sharingMode=Physical", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.SCSIControllers[0].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
			})
		})

		When("volume with no application type", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "multiwriter-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "multiwriter-pvc",
								},
								SharingMode: vmopv1.VolumeSharingModeMultiWriter,
							},
						},
					},
				}
			})

			It("should add controller with sharingMode=None", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.SCSIControllers[0].SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
			})
		})

		When("multiple volumes with different sharing requirements", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "oracle-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "oracle-pvc",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
							},
						},
					},
					{
						Name: "wsfc-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "wsfc-pvc",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeMicrosoftWSFC,
							},
						},
					},
				}
			})

			It("should add separate controllers for different sharing modes", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should create 2 controllers with different sharing modes.
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(2))

				// Find controllers by sharing mode.
				var noneController, physicalController *vmopv1.SCSIControllerSpec
				for i := range ctx.vm.Spec.Hardware.SCSIControllers {
					controller := &ctx.vm.Spec.Hardware.SCSIControllers[i]
					switch controller.SharingMode {
					case vmopv1.VirtualControllerSharingModeNone:
						noneController = controller
					case vmopv1.VirtualControllerSharingModePhysical:
						physicalController = controller
					}
				}

				Expect(noneController).ToNot(BeNil())
				Expect(physicalController).ToNot(BeNil())

				// Oracle volume should be on None controller.
				oracleVol := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(*oracleVol.ControllerBusNumber).To(Equal(noneController.BusNumber))

				// WSFC volume should be on Physical controller.
				wsfcVol := ctx.vm.Spec.Volumes[1].PersistentVolumeClaim
				Expect(*wsfcVol.ControllerBusNumber).To(Equal(physicalController.BusNumber))
			})
		})

		When("two volumes with same application type", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "oracle-vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
							},
						},
					},
					{
						Name: "oracle-vol2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc2",
								},
								ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
							},
						},
					},
				}
			})

			It("should share one controller for both volumes", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should create only one controller.
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))

				// Both volumes should be on the same controller.
				vol1 := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				vol2 := ctx.vm.Spec.Volumes[1].PersistentVolumeClaim
				Expect(*vol1.ControllerBusNumber).To(Equal(*vol2.ControllerBusNumber))
				Expect(*vol1.UnitNumber).To(Equal(int32(0)))
				Expect(*vol2.UnitNumber).To(Equal(int32(1)))
			})
		})
	})
}

func testMultipleControllerTypes(getCtx func() *unitMutationWebhookContext) {
	// Combination tests with multiple controller types.
	Context("Multiple Controller Types", func() {
		var ctx *unitMutationWebhookContext

		BeforeEach(func() {
			ctx = getCtx()
			ctx.vm.Status.UniqueID = dummyVMName
		})

		When("volumes require different controller types", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "scsi-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "scsi-pvc",
								},
								ControllerType: vmopv1.VirtualControllerTypeSCSI,
							},
						},
					},
					{
						Name: "sata-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "sata-pvc",
								},
								ControllerType: vmopv1.VirtualControllerTypeSATA,
							},
						},
					},
					{
						Name: "nvme-vol",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "nvme-pvc",
								},
								ControllerType: vmopv1.VirtualControllerTypeNVME,
							},
						},
					},
				}
			})

			It("should create appropriate controller for each type", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have one controller of each type.
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.SATAControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.NVMEControllers).To(HaveLen(1))

				// Each volume should be assigned to its respective controller.
				scsiVol := ctx.vm.Spec.Volumes[0].PersistentVolumeClaim
				Expect(scsiVol.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
				Expect(*scsiVol.ControllerBusNumber).To(Equal(int32(1)))

				sataVol := ctx.vm.Spec.Volumes[1].PersistentVolumeClaim
				Expect(sataVol.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSATA))
				Expect(*sataVol.ControllerBusNumber).To(Equal(int32(1)))

				nvmeVol := ctx.vm.Spec.Volumes[2].PersistentVolumeClaim
				Expect(nvmeVol.ControllerType).To(Equal(vmopv1.VirtualControllerTypeNVME))
				Expect(*nvmeVol.ControllerBusNumber).To(Equal(int32(1)))
			})
		})

		When("multiple volumes of each controller type", func() {
			BeforeEach(func() {
				// Add 2 volumes for each controller type.
				for i := 0; i < 2; i++ {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
						vmopv1.VirtualMachineVolume{
							Name: "scsi-vol-" + string(rune('1'+i)),
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "scsi-pvc-" + string(rune('1'+i)),
									},
									ControllerType: vmopv1.VirtualControllerTypeSCSI,
								},
							},
						},
						vmopv1.VirtualMachineVolume{
							Name: "sata-vol-" + string(rune('1'+i)),
							VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
								PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
									PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: "sata-pvc-" + string(rune('1'+i)),
									},
									ControllerType: vmopv1.VirtualControllerTypeSATA,
								},
							},
						},
					)
				}
			})

			It("should create one controller per type and assign all volumes", func() {
				mutated, err := mutation.AddControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have one controller of each type.
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.SATAControllers).To(HaveLen(1))

				// All volumes should have placement assigned.
				for _, vol := range ctx.vm.Spec.Volumes {
					pvc := vol.PersistentVolumeClaim
					Expect(pvc.ControllerBusNumber).ToNot(BeNil())
					Expect(pvc.UnitNumber).ToNot(BeNil())
				}
			})
		})

		When("VM has controllers of different types", func() {
			BeforeEach(func() {
				// Setup VM with 3 SATA controllers and 1 IDE controller.
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SATAControllers: []vmopv1.SATAControllerSpec{
						{BusNumber: 0},
						{BusNumber: 1},
						{BusNumber: 2},
					},
					IDEControllers: []vmopv1.IDEControllerSpec{
						{BusNumber: 0},
					},
				}

				// Add volumes that use the existing SATA controllers.
				for i := int32(0); i < 3; i++ {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("sata-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("sata-pvc-%d", i),
								},
								ControllerType:      vmopv1.VirtualControllerTypeSATA,
								ControllerBusNumber: ptr.To(i),
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					})
				}

				// Add a new SCSI volume that needs a controller.
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "scsi-vol",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "scsi-pvc",
							},
							ControllerType: vmopv1.VirtualControllerTypeSCSI,
						},
					},
				})
			})

			It("should create SCSI controller despite having 4 controllers of other types", func() {
				mutated, err := mutation.AddControllersForVolumes(
					&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should have created one SCSI controller.
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
				Expect(ctx.vm.Spec.Hardware.SCSIControllers[0].BusNumber).To(Equal(int32(1)))

				// SCSI volume should have placement assigned.
				scsiVol := ctx.vm.Spec.Volumes[len(ctx.vm.Spec.Volumes)-1]
				Expect(scsiVol.Name).To(Equal("scsi-vol"))
				Expect(scsiVol.PersistentVolumeClaim.ControllerType).To(Equal(vmopv1.VirtualControllerTypeSCSI))
				Expect(scsiVol.PersistentVolumeClaim.ControllerBusNumber).ToNot(BeNil())
				Expect(*scsiVol.PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(1)))
				Expect(scsiVol.PersistentVolumeClaim.UnitNumber).ToNot(BeNil())
				// Unit number should be assigned.
				Expect(*scsiVol.PersistentVolumeClaim.UnitNumber).To(BeNumerically(">=", 0))

				// Should still have the original 3 SATA and 1 IDE controllers.
				Expect(ctx.vm.Spec.Hardware.SATAControllers).To(HaveLen(3))
				Expect(ctx.vm.Spec.Hardware.IDEControllers).To(HaveLen(1))
			})
		})
	})
}
