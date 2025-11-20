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
		ctx.vm.Spec.Volumes = nil
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}

				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 10)
				unitNum := int32(0)
				for i := range 10 {
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
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(unitNum),
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
						},
					},
					ControllerBusNumber: ptr.To(int32(0)),
					ControllerType:      vmopv1.VirtualControllerTypeSCSI,
					UnitNumber:          ptr.To(unitNum),
				})
			})

			It("should allow the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("ParaVirtual SCSI controller at capacity", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}

				// Controller has 63 usable devices at max capacity (64 slots - 1 reserved), with
				// unit number 7 being reserved.
				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 63)
				unitNum := int32(0)
				for i := range 62 {
					if i == 7 {
						unitNum++
					}
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(unitNum),
					}
					unitNum++
				}

				// Try to add one more volume - controller is already at max capacity.
				// Since all slots are occupied, any unit we try will be "already in use".
				ctx.vm.Spec.Volumes[62] = vmopv1.VirtualMachineVolume{
					Name: "vol-extra",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-extra",
							},
						},
					},
					ControllerBusNumber: ptr.To(int32(0)),
					ControllerType:      vmopv1.VirtualControllerTypeSCSI,
					UnitNumber:          ptr.To(int32(42)),
				}
			})

			It("should reject due to controller at capacity", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[62].unitNumber: Invalid value: 42: controller unit number SCSI:0:42 is already in use"))
			})
		})

		When("BusLogic SCSI controller at capacity", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeBusLogic,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}

				// Controller has 14 usable devices at max capacity (15 slots - 1 reserved),
				// unit number 7 being reserved.
				ctx.vm.Spec.Volumes = make([]vmopv1.VirtualMachineVolume, 14)
				unitNum := int32(0)
				for i := range 13 {
					if unitNum == 7 {
						unitNum++
					}
					ctx.vm.Spec.Volumes[i] = vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("existing-vol-%d", i),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("existing-pvc-%d", i),
								},
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(unitNum),
					}
					unitNum++
				}

				// Try to add one more volume - controller is already at max capacity.
				// Since all slots are occupied, any unit we try will be "already in use".
				ctx.vm.Spec.Volumes[13] = vmopv1.VirtualMachineVolume{
					Name: "vol-extra",
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "pvc-extra",
							},
						},
					},
					ControllerBusNumber: ptr.To(int32(0)),
					ControllerType:      vmopv1.VirtualControllerTypeSCSI,
					UnitNumber:          ptr.To(int32(10)),
				}
			})

			It("should reject due to unit number already in use", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[13].unitNumber: Invalid value: 10: controller unit number SCSI:0:10 is already in use"))
			})
		})
	})

	Context("Controller bus number 0 validation on CREATE", func() {
		BeforeEach(func() {
			ctx.oldVM = nil

			// TODO: Fix test setup so it doesn't always assume update.
			ctx.vm.Spec.Hardware.SCSIControllers = nil
			ctx.vm.Spec.Hardware.SATAControllers = nil
			ctx.vm.Spec.Hardware.NVMEControllers = nil
		})

		When("creating VM with SCSI controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.hardware.scsiControllers[0].busNumber: Invalid value: 0: bus number 0 is reserved for the default controller"))
			})
		})

		When("creating VM with SATA controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
					{
						BusNumber: 0,
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.hardware.sataControllers[0].busNumber: Invalid value: 0: bus number 0 is reserved for the default controller"))
			})
		})

		When("creating VM with NVME controller at bus 0", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
					{
						BusNumber: 0,
					},
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.hardware.nvmeControllers[0].busNumber: Invalid value: 0: bus number 0 is reserved for the default controller"))
			})
		})

		When("creating VM with SCSI controller at bus 1", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   1,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
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
				}
			})

			It("should reject due to bus 0 being reserved", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.hardware.scsiControllers[0].busNumber: Invalid value: 0: bus number 0 is reserved for the default controller"))
			})
		})

		When("AllDisksArePVCs is enabled", func() {
			BeforeEach(func() {
				pkgcfg.SetContext(&ctx.WebhookRequestContext, func(config *pkgcfg.Config) {
					config.Features.AllDisksArePVCs = true
				})
			})
			When("creating VM with SCSI controller at bus 0", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					}
				})

				It("should accept", func() {
					response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
					Expect(response.Allowed).To(BeTrue())
				})
			})
		})
	})

	Context("Controller bus number validation on UPDATE", func() {
		When("volume specifies valid controller bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
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
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
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
							},
						},
						ControllerBusNumber: ptr.To(int32(2)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[0].controllerBusNumber: Invalid value: 2: controller SCSI:2 does not exist"))
			})
		})

		When("volume specifies out-of-range bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
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
							},
						},
						ControllerBusNumber: ptr.To(int32(5)), // Out of range (max is 3)
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[0].controllerBusNumber: Invalid value: 5: must be between 0 and 3"))
			})
		})
	})

	Context("Application type validation", func() {
		When("OracleRAC volume with mutation-added controller", func() {
			BeforeEach(func() {
				// Mutation webhook would have added a None controller for OracleRAC.
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
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
							},
						},
						ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModePhysical,
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
							},
						},
						ApplicationType:     vmopv1.VolumeApplicationTypeMicrosoftWSFC,
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
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
							},
						},
						ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}
				ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:                "cdrom1",
						Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso", Kind: "VirtualMachineImage"},
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(5)),
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
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(5)),
					},
				}
			})

			It("should reject due to unit number conflict with CD-ROM", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[0].unitNumber: Invalid value: 5: controller unit number SCSI:0:5 is already in use"))
			})
		})

		When("volume uses different slot than CD-ROM", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}
				ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
					{
						Name:                "cdrom1",
						Image:               vmopv1.VirtualMachineImageRef{Name: "test-iso", Kind: "VirtualMachineImage"},
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(5)),
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
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(6)),
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
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{
						BusNumber:   0,
						Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
						SharingMode: vmopv1.VirtualControllerSharingModeNone,
					},
				}
				ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
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
				}

				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
					},
					{
						Name: "vol2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc2",
								},
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(3)), // Conflicts with cdrom1.
					},
				}
			})

			It("should reject volume that conflicts with CD-ROM", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[1].unitNumber: Invalid value: 3: controller unit number SCSI:0:3 is already in use"))
			})
		})
	})

	Context("Mixed scenarios", func() {
		When("some volumes fit, some don't", func() {
			BeforeEach(func() {
				// Two controllers with different capacities.
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
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
							},
						},
						ControllerBusNumber: ptr.To(int32(1)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(unitNum),
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
							},
						},
						ControllerBusNumber: ptr.To(int32(0)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(0)),
					},
					vmopv1.VirtualMachineVolume{
						Name: "vol2",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc2",
								},
							},
						},
						ControllerBusNumber: ptr.To(int32(1)),
						ControllerType:      vmopv1.VirtualControllerTypeSCSI,
						UnitNumber:          ptr.To(int32(1)),
					},
				)
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(Equal("spec.volumes[15].unitNumber: Invalid value: 1: controller unit number SCSI:1:1 is already in use"))
			})
		})
	})

	Context("Nil Hardware validation", func() {
		When("VM has nil Hardware with VMSharedDisks enabled", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware = nil
				ctx.vm.Spec.Volumes = nil
			})

			It("should allow creation without panic", func() {
				ctx.oldVM = nil
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})

			It("should allow update without panic", func() {
				ctx.oldVM.Spec.Hardware = nil
				ctx.oldVM.Spec.Volumes = nil
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	validateUpdateTest := func(args testParams) {

		GinkgoHelper()

		args.setup(ctx)

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(args.expectAllowed))

		if args.validate != nil {
			args.validate(response)
		}
	}

	Context("Controller settings update", func() {
		BeforeEach(func() {
			// Remove volumes.
			ctx.oldVM.Spec.Volumes = nil
			ctx.vm.Spec.Volumes = nil
			// Remove controllers.
			ctx.oldVM.Spec.Hardware.SCSIControllers = nil
			ctx.vm.Spec.Hardware.SCSIControllers = nil
			ctx.oldVM.Spec.Hardware.SATAControllers = nil
			ctx.vm.Spec.Hardware.SATAControllers = nil
			ctx.oldVM.Spec.Hardware.NVMEControllers = nil
			ctx.vm.Spec.Hardware.NVMEControllers = nil

			// Add upgradedToSchemaVersion annotation so mutating is allowed.
			bypassUpgradeCheck(&ctx.Context, ctx.vm, ctx.oldVM)

		})

		Context("oldVM is poweredOn", func() {
			BeforeEach(func() {
				ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})

			DescribeTable("update", validateUpdateTest,
				Entry("should deny when scsiController pciSlotNumber is updated",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
								},
							}
							ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(1)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.hardware.scsiControllers[0].pciSlotNumber: Forbidden: updates to this field is not allowed when VM power is on"),
					},
				),
				Entry("should deny when scsiController sharingMode is updated",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
								},
							}
							ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
									SharingMode:   vmopv1.VirtualControllerSharingModePhysical,
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.hardware.scsiControllers[0].sharingMode: Forbidden: updates to this field is not allowed when VM power is on"),
					},
				),
				Entry("should deny when scsiController type is updated",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
									Type:          vmopv1.SCSIControllerTypeParaVirtualSCSI,
								},
							}
							ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
									Type:          vmopv1.SCSIControllerTypeBusLogic,
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.hardware.scsiControllers[0].type: Forbidden: updates to this field is not allowed when VM power is on"),
					},
				),
				Entry("should deny when sataController pciSlotNumber is updated",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
								},
							}
							ctx.vm.Spec.Hardware.SATAControllers = []vmopv1.SATAControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(1)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.hardware.sataControllers[0].pciSlotNumber: Forbidden: updates to this field is not allowed when VM power is on"),
					},
				),
				Entry("should deny when nvmeControllers pciSlotNumber is updated",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
								},
							}
							ctx.vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(1)),
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.hardware.nvmeControllers[0].pciSlotNumber: Forbidden: updates to this field is not allowed when VM power is on"),
					},
				),
				Entry("should deny when nvmeControllers pciSlotNumber is updated",
					testParams{
						setup: func(ctx *unitValidatingWebhookContext) {
							ctx.oldVM.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
									SharingMode:   vmopv1.VirtualControllerSharingModeNone,
								},
							}
							ctx.vm.Spec.Hardware.NVMEControllers = []vmopv1.NVMEControllerSpec{
								{
									BusNumber:     1,
									PCISlotNumber: ptr.To(int32(0)),
									SharingMode:   vmopv1.VirtualControllerSharingModePhysical,
								},
							}
						},
						expectAllowed: false,
						validate:      doValidateWithMsg("spec.hardware.nvmeControllers[0].sharingMode: Forbidden: updates to this field is not allowed when VM power is on"),
					},
				),
			)

			When("create", func() {
				It("should allow", func() {
					var err error
					ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
					Expect(err).ToNot(HaveOccurred())
					ctx.WebhookRequestContext.OldObj = nil

					response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
					Expect(response.Allowed).To(Equal(true))
				})
			})
		})

		Context("oldVM is poweredOff", func() {
			BeforeEach(func() {
				ctx.oldVM.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})

			When("create", func() {
				It("should allow", func() {
					var err error
					ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
					Expect(err).ToNot(HaveOccurred())
					ctx.WebhookRequestContext.OldObj = nil

					response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
					Expect(response.Allowed).To(Equal(true))
				})
			})

			When("update", func() {
				It("should allow", func() {
					ctx.oldVM.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:     1,
							PCISlotNumber: ptr.To(int32(0)),
						},
					}
					ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
						{
							BusNumber:     1,
							PCISlotNumber: ptr.To(int32(1)),
						},
					}

					var err error
					ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
					Expect(err).ToNot(HaveOccurred())
					ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
					Expect(err).ToNot(HaveOccurred())

					response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
					Expect(response.Allowed).To(Equal(true))
				})
			})
		})
	})
}
