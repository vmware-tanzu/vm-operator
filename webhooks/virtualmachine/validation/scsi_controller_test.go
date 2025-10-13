// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func scsiControllerTests() {
	Describe(
		"SCSI Controller Validation",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Validation,
			testlabels.Webhook,
		),
		scsiControllerValidationTests,
	)
}

func scsiControllerValidationTests() {
	var (
		ctx *unitValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
		ctx.vm.Status.UniqueID = "vm-123" // VM exists on infrastructure

		// Enable VMSharedDisks feature flag for consistency
		pkgcfg.SetContext(&ctx.WebhookRequestContext, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
		})
	})

	// Update ctx.Obj from ctx.vm before each test
	JustBeforeEach(func() {
		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
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

				// Controller has 10 devices
				devices := make([]vmopv1.VirtualDeviceStatus, 10)
				for i := range devices {
					devices[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   devices,
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
							},
						},
					},
				}
			})

			It("should allow the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
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

				// Controller has 63 devices (maximum for ParaVirtual)
				devices := make([]vmopv1.VirtualDeviceStatus, 63)
				for i := range devices {
					devices[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   devices,
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
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("SCSI controller 0"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("cannot accommodate"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("Maximum slots: 63"))
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

				// Controller has 15 devices (maximum for BusLogic)
				devices := make([]vmopv1.VirtualDeviceStatus, 15)
				for i := range devices {
					devices[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   devices,
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
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("Maximum slots: 15"))
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

				// Controller has 60 devices
				devices := make([]vmopv1.VirtualDeviceStatus, 60)
				for i := range devices {
					devices[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   devices,
						},
					},
				}

				// Try to add 4 more volumes (would be 64 total, exceeds 63 max)
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
					},
					{
						Name: "vol3",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc3",
								},
							},
						},
					},
					{
						Name: "vol4",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc4",
								},
							},
						},
					},
				}
			})

			It("should reject due to insufficient capacity", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("cannot accommodate 4 more volumes"))
			})
		})
	})

	Context("Controller bus number validation", func() {
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

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{},
						},
					},
				}

				busNum := int32(0)
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: &busNum,
							},
						},
					},
				}
			})

			It("should allow the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
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

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{},
						},
					},
				}

				busNum := int32(2) // Controller 2 doesn't exist
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: &busNum,
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("SCSI controller with bus number 2 does not exist"))
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

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{},
						},
					},
				}

				busNum := int32(5) // Out of range (max is 3)
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								ControllerBusNumber: &busNum,
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("must be between 0 and 3"))
			})
		})
	})

	Context("Application type validation", func() {
		When("OracleRAC volume without sharingMode=None controller", func() {
			BeforeEach(func() {
				// Only have a Physical sharing mode controller
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModePhysical,
						},
					},
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{},
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
								ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
							},
						},
					},
				}
			})

			It("should allow (mutation webhook will add controller)", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("MicrosoftWSFC volume without sharingMode=Physical controller", func() {
			BeforeEach(func() {
				// Only have a None sharing mode controller
				ctx.vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{
					SCSIControllers: []vmopv1.SCSIControllerSpec{
						{
							BusNumber:   0,
							Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
							SharingMode: vmopv1.VirtualControllerSharingModeNone,
						},
					},
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{},
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
								ApplicationType: vmopv1.VolumeApplicationTypeMicrosoftWSFC,
							},
						},
					},
				}
			})

			It("should allow (mutation webhook will add controller)", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
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

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{},
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
								ApplicationType: vmopv1.VolumeApplicationTypeOracleRAC,
							},
						},
					},
				}
			})

			It("should allow the volume", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Context("No controllers specified", func() {
		When("VM has volumes but no controllers", func() {
			BeforeEach(func() {
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
					},
				}
			})

			It("should allow (mutation webhook will add controller)", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeTrue())
			})
		})
	})

	Context("Mixed scenarios", func() {
		When("some volumes fit, some don't", func() {
			BeforeEach(func() {
				// Two controllers with different capacities
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

				// Controller 0 has space, controller 1 is full
				devices1 := make([]vmopv1.VirtualDeviceStatus, 15)
				for i := range devices1 {
					devices1[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{}, // Empty
						},
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 1,
							Devices:   devices1, // Full
						},
					},
				}

				busNum1 := int32(1)
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								// Will go to controller 0 (default)
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
								ControllerBusNumber: &busNum1, // Explicitly targets full controller
							},
						},
					},
				}
			})

			It("should reject due to controller 1 being full", func() {
				response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("SCSI controller 1"))
			})
		})
	})
}
