// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/mutation"
)

func scsiControllerTests() {
	Describe(
		"AddSCSIControllersForVolumes",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		scsiControllerMutationTests,
	)
}

func scsiControllerMutationTests() {
	var (
		ctx *unitMutationWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForMutatingWebhook()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
		})
	})

	Context("VM not yet created on infrastructure", func() {
		BeforeEach(func() {
			// A VM that is not yet created
			ctx.vm.Status.UniqueID = ""
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

		It("should skip mutation", func() {
			mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())

			// No SCSI controllers should be added
			if ctx.vm.Spec.Hardware != nil {
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(BeEmpty())
			}
		})
	})

	Context("VM with UniqueID and no existing controllers", func() {
		BeforeEach(func() {
			ctx.vm.Status.UniqueID = "vm-123"
		})

		When("single volume without explicit controller", func() {
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

			It("should add a default ParaVirtual SCSI controller at bus 0", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware).ToNot(BeNil())
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))

				controller := ctx.vm.Spec.Hardware.SCSIControllers[0]
				Expect(controller.BusNumber).To(Equal(int32(0)))
				Expect(controller.Type).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))
				Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))

				// Volume should have controllerBusNumber set to bus 0 to pin it to the new controller
				Expect(ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).ToNot(BeNil())
				Expect(*ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(0)))
			})
		})

		When("multiple volumes without explicit controller", func() {
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
				}
			})

			It("should add a single controller for all volumes", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware).ToNot(BeNil())
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))

				controller := ctx.vm.Spec.Hardware.SCSIControllers[0]
				Expect(controller.BusNumber).To(Equal(int32(0)))
			})
		})

		When("volume with OracleRAC application type", func() {
			BeforeEach(func() {
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

			It("should add controller with sharingMode=None", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware).ToNot(BeNil())
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))

				controller := ctx.vm.Spec.Hardware.SCSIControllers[0]
				Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
			})
		})

		When("volume with MultiWriter sharing mode", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
					{
						Name: "vol1",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "pvc1",
								},
								SharingMode: vmopv1.VolumeSharingModeMultiWriter,
							},
						},
					},
				}
			})

			It("should add controller with sharingMode=None", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware).ToNot(BeNil())
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))

				controller := ctx.vm.Spec.Hardware.SCSIControllers[0]
				Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModeNone))
			})
		})

		When("volume with MicrosoftWSFC application type", func() {
			BeforeEach(func() {
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

			It("should add controller with sharingMode=Physical", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware).ToNot(BeNil())
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))

				controller := ctx.vm.Spec.Hardware.SCSIControllers[0]
				Expect(controller.SharingMode).To(Equal(vmopv1.VirtualControllerSharingModePhysical))
			})
		})
	})

	Context("VM with existing controllers", func() {
		BeforeEach(func() {
			ctx.vm.Status.UniqueID = "vm-123"
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

		When("controller has available slots", func() {
			BeforeEach(func() {
				// VM status reports that the SCSI Controller has 0 devices
				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{
							Type:      vmopv1.VirtualControllerTypeSCSI,
							BusNumber: 0,
							Devices:   []vmopv1.VirtualDeviceStatus{}, // no devices
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

			It("should not add a new controller", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())

				// Still only one controller
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(1))
			})
		})

		When("controller is full", func() {
			BeforeEach(func() {
				// Fill controller 0 to capacity (63 devices for ParaVirtual)
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

			It("should add a new controller at bus 1", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(2))

				// New controller should be at bus 1
				newController := ctx.vm.Spec.Hardware.SCSIControllers[1]
				Expect(newController.BusNumber).To(Equal(int32(1)))
				Expect(newController.Type).To(Equal(vmopv1.SCSIControllerTypeParaVirtualSCSI))

				// Volume should have controllerBusNumber
				Expect(ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).ToNot(BeNil())
				Expect(*ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(1)))
			})
		})

		When("multiple application types requiring different controllers", func() {
			BeforeEach(func() {
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
								ApplicationType: vmopv1.VolumeApplicationTypeMicrosoftWSFC,
							},
						},
					},
				}
			})

			It("should add controller with sharingMode=Physical for WSFC", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// Should add a new controller for WSFC
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(2))

				// Find the Physical sharing mode controller
				var physicalController *vmopv1.SCSIControllerSpec
				for i := range ctx.vm.Spec.Hardware.SCSIControllers {
					if ctx.vm.Spec.Hardware.SCSIControllers[i].SharingMode == vmopv1.VirtualControllerSharingModePhysical {
						physicalController = &ctx.vm.Spec.Hardware.SCSIControllers[i]
						break
					}
				}

				Expect(physicalController).ToNot(BeNil())
				Expect(physicalController.BusNumber).To(Equal(int32(1)))

				// WSFC volume should have controllerBusNumber set
				Expect(ctx.vm.Spec.Volumes[1].PersistentVolumeClaim.ControllerBusNumber).ToNot(BeNil())
				Expect(*ctx.vm.Spec.Volumes[1].PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(1)))
			})
		})

		When("volume already has controllerBusNumber set", func() {
			BeforeEach(func() {
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

			It("should not modify the controllerBusNumber", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())

				// controllerBusNumber should remain unchanged
				Expect(ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).ToNot(BeNil())
				Expect(*ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(0)))
			})
		})

		When("maximum controllers reached", func() {
			BeforeEach(func() {
				// Add 4 controllers (maximum)
				ctx.vm.Spec.Hardware.SCSIControllers = []vmopv1.SCSIControllerSpec{
					{BusNumber: 0, Type: vmopv1.SCSIControllerTypeParaVirtualSCSI, SharingMode: vmopv1.VirtualControllerSharingModeNone},
					{BusNumber: 1, Type: vmopv1.SCSIControllerTypeParaVirtualSCSI, SharingMode: vmopv1.VirtualControllerSharingModeNone},
					{BusNumber: 2, Type: vmopv1.SCSIControllerTypeParaVirtualSCSI, SharingMode: vmopv1.VirtualControllerSharingModeNone},
					{BusNumber: 3, Type: vmopv1.SCSIControllerTypeParaVirtualSCSI, SharingMode: vmopv1.VirtualControllerSharingModeNone},
				}

				// All controllers full
				devices := make([]vmopv1.VirtualDeviceStatus, 63)
				for i := range devices {
					devices[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

				ctx.vm.Status.Hardware = &vmopv1.VirtualMachineHardwareStatus{
					Controllers: []vmopv1.VirtualControllerStatus{
						{Type: vmopv1.VirtualControllerTypeSCSI, BusNumber: 0, Devices: devices},
						{Type: vmopv1.VirtualControllerTypeSCSI, BusNumber: 1, Devices: devices},
						{Type: vmopv1.VirtualControllerTypeSCSI, BusNumber: 2, Devices: devices},
						{Type: vmopv1.VirtualControllerTypeSCSI, BusNumber: 3, Devices: devices},
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

			It("should not add more controllers", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeFalse())

				// Should still have 4 controllers
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(4))
			})
		})
	})

	// Note: Feature flag checking is done at the webhook level (virtualmachine_mutator.go),
	// not within AddSCSIControllersForVolumes itself. The webhook checks
	// pkgcfg.FromContext(ctx).Features.VMSharedDisks before calling this function.

	Context("Non-PVC volumes", func() {
		BeforeEach(func() {
			ctx.vm.Status.UniqueID = "vm-123"
			ctx.vm.Spec.Volumes = []vmopv1.VirtualMachineVolume{
				{
					Name: "vol1",
					// No PersistentVolumeClaim - some other volume type
				},
			}
		})

		It("should not add controllers for non-PVC volumes", func() {
			mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutated).To(BeFalse())

			// No SCSI controllers should be added
			if ctx.vm.Spec.Hardware != nil {
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(BeEmpty())
			}
		})
	})

	Context("Race condition prevention", func() {
		When("new controller is created but slot might open before reconciliation", func() {
			BeforeEach(func() {
				ctx.vm.Status.UniqueID = "vm-123"

				// Controller at bus 0 is full
				devices := make([]vmopv1.VirtualDeviceStatus, 63)
				for i := range devices {
					devices[i] = vmopv1.VirtualDeviceStatus{
						Type:       vmopv1.VirtualDeviceTypeDisk,
						UnitNumber: int32(i),
					}
				}

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

			It("should set controllerBusNumber to pin volume to new controller", func() {
				mutated, err := mutation.AddSCSIControllersForVolumes(&ctx.WebhookRequestContext, ctx.Client, ctx.vm)
				Expect(err).ToNot(HaveOccurred())
				Expect(mutated).To(BeTrue())

				// New controller should be added
				Expect(ctx.vm.Spec.Hardware.SCSIControllers).To(HaveLen(2))

				// Volume should have controllerBusNumber set to new controller
				Expect(ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).ToNot(BeNil())
				Expect(*ctx.vm.Spec.Volumes[0].PersistentVolumeClaim.ControllerBusNumber).To(Equal(int32(1)))
			})
		})
	})
}
