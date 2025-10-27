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
		ctx = newUnitTestContextForValidatingWebhook(true)
		ctx.vm.Status.UniqueID = "vm-123"

		// Enable VMSharedDisks feature flag for consistency
		pkgcfg.SetContext(&ctx.WebhookRequestContext, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
		})
	})

	// Update ctx.Obj and ctx.OldObj from ctx.vm and ctx.oldVM before each test
	JustBeforeEach(func() {
		// Sync hardware and volumes from vm to oldVM to avoid false hardware
		// change detection. Tests modify ctx.vm but oldVM is created
		// before BeforeEach runs
		ctx.oldVM.Spec.Hardware = ctx.vm.Spec.Hardware.DeepCopy()
		ctx.oldVM.Spec.Volumes = append(
			ctx.oldVM.Spec.Volumes,
			ctx.vm.Spec.Volumes...,
		)
		ctx.oldVM.Status.Hardware = ctx.vm.Status.Hardware.DeepCopy()

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.vm)
		Expect(err).ToNot(HaveOccurred())
		ctx.WebhookRequestContext.OldObj, err = builder.ToUnstructured(ctx.oldVM)
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
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
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
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("controller SCSI:0 full"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("maxDevices: 63"))
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
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							},
						},
					},
				}
			})

			It("should reject the volume", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("controller SCSI:0 full"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("maxDevices: 15"))
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
				busNum := ptr.To(int32(0))
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{}

				for i := 0; i < 4; i++ {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes, vmopv1.VirtualMachineVolume{
						Name: fmt.Sprintf("vol-%d", i+1),
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: fmt.Sprintf("pvc-%d", i+1),
								},
								ControllerBusNumber: busNum,
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							},
						},
					})
				}
			})

			It("should reject due to insufficient capacity", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("controller SCSI:0 full"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("maxDevices: 63"))
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
								ControllerBusNumber: ptr.To(int32(2)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
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
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
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
				// Mutation webhook would have added a None controller for OracleRAC
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
								ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
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
				// Mutation webhook would have added a Physical controller for WSFC
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
								ApplicationType:     vmopv1.VolumeApplicationTypeMicrosoftWSFC,
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
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
								ApplicationType:     vmopv1.VolumeApplicationTypeOracleRAC,
								ControllerBusNumber: ptr.To(int32(0)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
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
								ControllerBusNumber: ptr.To(int32(1)),
								ControllerType:      vmopv1.VirtualControllerTypeSCSI,
							},
						},
					},
				}
			})

			It("should reject due to controller 1 being full", func() {
				response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
				Expect(response.Allowed).To(BeFalse())
				Expect(string(response.Result.Reason)).To(ContainSubstring("controller SCSI:1 full"))
				Expect(string(response.Result.Reason)).To(ContainSubstring("maxDevices: 15"))
			})
		})
	})
}
