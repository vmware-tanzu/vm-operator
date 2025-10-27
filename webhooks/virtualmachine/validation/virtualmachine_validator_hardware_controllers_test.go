// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func controllerTests() {
	Describe(
		"Controller Validation",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Validation,
			testlabels.Webhook,
		),
		controllerValidationTests,
	)
}

func controllerValidationTests() {
	var (
		ctx *unitValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
		ctx.vm.Status.UniqueID = "vm-123"

		// Enable VMSharedDisks feature flag for consistency.
		pkgcfg.SetContext(&ctx.WebhookRequestContext, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
		})
	})

	// Update ctx.Obj and ctx.OldObj from ctx.vm and ctx.oldVM before each test.
	JustBeforeEach(func() {
		// Sync hardware and volumes from vm to oldVM to avoid false hardware
		// change detection. Tests modify ctx.vm but oldVM is created
		// before BeforeEach runs.
		if ctx.oldVM != nil {
			ctx.oldVM.Spec.Hardware = ctx.vm.Spec.Hardware.DeepCopy()
			ctx.oldVM.Spec.Volumes = append(
				ctx.oldVM.Spec.Volumes,
				ctx.vm.Spec.Volumes...,
			)
			ctx.oldVM.Status.Hardware = ctx.vm.Status.Hardware.DeepCopy()
		}

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
		if ctx.oldVM != nil {
			ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
			Expect(err).ToNot(HaveOccurred())
		} else {
			ctx.WebhookRequestContext.OldObj = nil
		}
	})

	Context("Controller capacity validation", func() {
		When("volume fits on existing controller", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 10)
				unitNum := int32(0)
				for i := 0; i < 10; i++ {
					if unitNum == 7 {
						unitNum++ // Skip reserved unit 7.
					}
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(unitNum),
							},
						},
					}
					unitNum++
				}

				// Add new volume at next available unit.
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "vol1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc1",
							},
							ControllerBusNumber: ptr.To(int32(0)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							UnitNumber:          ptr.To(unitNum),
						},
					},
				})
			})

			It("should allow the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("ParaVirtual SCSI controller at capacity", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				// Controller has 63 devices at max capacity.
				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 63)
				for i := 0; i < 63; i++ {
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(i)),
							},
						},
					}
				}

				// Try to add one more volume - controller is already at max capacity.
				// Since all slots are occupied, any unit we try will be "already in use".
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "vol1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc1",
							},
							ControllerBusNumber: ptr.To(int32(0)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							UnitNumber:          ptr.To(int32(0)),
						},
					},
				})
			})

			It("should reject due to controller at capacity", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("already in use"))
			})
		})

		When("BusLogic SCSI controller at capacity", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeBusLogic,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				// Controller has 15 devices at max capacity.
				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 15)
				for i := 0; i < 15; i++ {
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(i)),
							},
						},
					}
				}

				// Try to add one more volume - controller is already at max capacity.
				// Since all slots are occupied, any unit we try will be "already in use".
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "vol1",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc1",
							},
							ControllerBusNumber: ptr.To(int32(0)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							UnitNumber:          ptr.To(int32(0)), // Reuse slot 0
						},
					},
				})
			})

			It("should reject due to controller at capacity", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("already in use"))
			})
		})

		When("multiple volumes exceed controller capacity", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				// Controller has 62 devices. This fills all usable slots.
				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 62)
				unitNum := int32(0)
				for i := 0; i < 62; i++ {
					if unitNum == 7 {
						unitNum++ // Skip reserved unit 7.
					}
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(unitNum),
							},
						},
					}
					unitNum++
				}

				// Try to add one more volume using an already-occupied slot.
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
					Name: "vol-extra",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-extra",
							},
							ControllerBusNumber: ptr.To(int32(0)),
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							UnitNumber:          ptr.To(int32(0)), // Reuse slot 0.
						},
					},
				})
			})

			It("should reject due to unit number already in use", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("already in use"))
			})
		})
	})

	Context("Controller bus number 0 validation on CREATE", func() {
		BeforeEach(func() {
			ctx.oldVM = nil
		})

		When("creating VM with SCSI controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("bus number 0 is reserved"))
			})
		})

		When("creating VM with SATA controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SATAControllers: []vmopv1.SATAControllerSpec{
						{
							BusNumber: 0,
						},
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("bus number 0 is reserved"))
			})
		})

		When("creating VM with NVME controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					NVMEControllers: []vmopv1.NVMEControllerSpec{
						{
							BusNumber: 0,
						},
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("bus number 0 is reserved"))
			})
		})

		When("creating VM with IDE controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					IDEControllers: []vmopv1.IDEControllerSpec{
						{
							BusNumber: 0,
						},
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("bus number 0 is reserved"))
			})
		})

		When("creating VM with SCSI controller at bus 1", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}
				// Clear volumes to avoid validation errors for non-existent controllers.
				ctx.vm.Spec.Volumes = nil
			})

			It("should allow controller at bus 1", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("updating VM with SCSI controller at bus 0", func() {
			BeforeEach(func() {
				// Create a new oldVM to simulate an update.
				ctx.oldVM = ctx.vm.DeepCopy()
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}
			})

			It("should allow controller at bus 0 on update", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("creating VM with multiple controllers, one at bus 0", func() {
			BeforeEach(func() {
				ctx.oldVM = nil
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("bus number 0 is reserved"))
			})
		})
	})

	Context("Controller bus number validation on UPDATE", func() {
		When("volume specifies valid controller bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
				}
			})

			It("should allow the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("volume specifies non-existent controller bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: ptr.To(int32(2)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("controller SCSI:2 does not exist"))
			})
		})

		When("volume specifies out-of-range bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				busNum := int32(5) // Out of range (max is 3).
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: &busNum,
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("must be between 0 and 3"))
			})
		})
	})

	Context("Application type validation", func() {
		When("OracleRAC volume with mutation-added controller", func() {
			BeforeEach(func() {
				// Mutation webhook would have added a None controller for OracleRAC.
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
				}
			})

			It("should allow (mutation webhook added controller)", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("MicrosoftWSFC volume with mutation-added controller", func() {
			BeforeEach(func() {
				// Mutation webhook would have added a Physical controller for WSFC.
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ApplicationType:     vmopv1.VolumeApplicationTypeMicrosoftWSFC,
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
				}
			})

			It("should allow (mutation webhook added controller)", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("OracleRAC volume with appropriate controller", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
				}
			})

			It("should allow the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Context("CD-ROM conflict validation", func() {
		When("volume tries to use slot occupied by CD-ROM", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
					Cdrom: []vmopv1.VirtualMachineCdromSpec{
						{
							Name:                "cdrom1",
							Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso", Kind: "VirtualMachineImage"},
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(5)),
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(5)),
							},
						},
					},
				}
			})

			It("should reject due to unit number conflict with CD-ROM", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("unit number"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("already in use"))
			})
		})

		When("volume uses different slot than CD-ROM", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
					Cdrom: []vmopv1.VirtualMachineCdromSpec{
						{
							Name:                "cdrom1",
							Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso", Kind: "VirtualMachineImage"},
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(5)),
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(6)),
							},
						},
					},
				}
			})

			It("should allow the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("multiple CD-ROMs occupy slots", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
					Cdrom: []vmopv1.VirtualMachineCdromSpec{
						{
							Name:                "cdrom1",
							Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso-1", Kind: "VirtualMachineImage"},
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(3)),
						},
						{
							Name:                "cdrom2",
							Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso-2", Kind: "VirtualMachineImage"},
							ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							ControllerBusNumber: ptr.To(int32(0)),
							UnitNumber:          ptr.To(int32(5)),
						},
					},
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
					{
						Name: "vol2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc2",
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(3)), // Conflicts with cdrom1.
							},
						},
					},
				}
			})

			It("should reject volume that conflicts with CD-ROM", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("unit number"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("already in use"))
			})
		})
	})

	Context("Mixed scenarios", func() {
		When("some volumes fit, some don't", func() {
			BeforeEach(func() {
				// Two controllers with different capacities.
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
						{
							BusNumber:   1,
							Type:        vmopv1.SCSIControllerTypeBusLogic,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				// Controller 1 has 14 devices.
				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 14)
				unitNum := int32(0)
				for i := 0; i < 14; i++ {
					if unitNum == 7 {
						unitNum++ // Skip reserved unit 7.
					}
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
								ControllerBusNumber: ptr.To(int32(1)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(unitNum),
							},
						},
					}
					unitNum++
				}

				// Add volumes: one for controller 0 (should work), one for
				// controller 1 (should fail - reusing reserved slot).
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
					vmopv1.VirtualMachineVolume{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(0)),
							},
						},
					},
					vmopv1.VirtualMachineVolume{
						Name: "vol2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc2",
								},
								ControllerBusNumber: ptr.To(int32(1)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
								UnitNumber:          ptr.To(int32(1)),
							},
						},
					},
				)
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("already in use"))
			})
		})
	})
}
