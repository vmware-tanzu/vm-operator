// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {

	pkgcfg.SetContext(suite, func(config *pkgcfg.Config) {
		config.Features.VMSharedDisks = true
	})

	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateUpdate,
	)
	Describe(
		"Update CD-ROM Controller",
		Serial,
		Label(
			testlabels.Update,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateCdromController,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateDelete,
	)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vm *vmopv1.VirtualMachine
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vm = builder.DummyVirtualMachine()
	ctx.vm.Namespace = ctx.Namespace

	return ctx
}

func intgTestsValidateCreate() {

	var (
		ctx *intgValidatingWebhookContext
	)

	type createArgs struct {
		emptyImage       bool
		emptyImageKind   bool
		invalidImageKind bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.emptyImage {
			ctx.vm.Spec.Image = nil
		}
		if args.emptyImageKind {
			ctx.vm.Spec.Image.Kind = ""
		}
		if args.invalidImageKind {
			ctx.vm.Spec.Image.Kind = invalidKind
		}

		err := ctx.Client.Create(ctx, ctx.vm)
		if expectedAllowed {
			Expect(err).ToNot(HaveOccurred())
		} else {
			Expect(err).To(HaveOccurred())
		}
		if expectedReason != "" {
			Expect(err.Error()).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	specPath := field.NewPath("spec")

	DescribeTable("create table", validateCreate,
		Entry("should work", createArgs{}, true, "", nil),
		Entry("should not work for empty image", createArgs{emptyImage: true}, false,
			field.Required(specPath.Child("image"), "").Error(), nil),
		Entry("should not work for empty image kind", createArgs{emptyImageKind: true}, false,
			field.Required(specPath.Child("image").Child("kind"), invalidImageKindMsg).Error(), nil),
		Entry("should not work for invalid image kind", createArgs{invalidImageKind: true}, false,
			field.Invalid(specPath.Child("image").Child("kind"), invalidKind, invalidImageKindMsg).Error(), nil),
	)
}

func intgTestsValidateUpdate() {
	const (
		immutableFieldMsg = "field is immutable"
		unsupportedValMsg = "Unsupported value"
		duplicateValMsg   = "Duplicate value"
	)

	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vm)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("update is performed with changed image", func() {
		BeforeEach(func() {
			ctx.vm.Spec.Image.Kind += "-2"
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			expectedPathStr := field.NewPath("spec", "image").String()
			Expect(err.Error()).To(ContainSubstring(expectedPathStr))
			Expect(err.Error()).To(ContainSubstring(immutableFieldMsg))
		})
	})
	When("update is performed with changed image name", func() {
		BeforeEach(func() {
			ctx.vm.Spec.ImageName += "-2"
		})

		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			expectedPathStr := field.NewPath("spec", "imageName").String()
			Expect(err.Error()).To(ContainSubstring(expectedPathStr))
			Expect(err.Error()).To(ContainSubstring(immutableFieldMsg))
		})
	})

	When("update is performed with changed storageClass name", func() {
		BeforeEach(func() {
			ctx.vm.Spec.StorageClass += "-2"
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			expectedPath := field.NewPath("spec", "storageClass")
			Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			Expect(err.Error()).To(ContainSubstring(immutableFieldMsg))
		})
	})

	When("update is performed with changed minimum hardware version", func() {
		BeforeEach(func() {
			ctx.vm.Spec.MinHardwareVersion += 2
		})
		When("vm is powered off", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
			})
			It("should allow the request", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
		When("vm is powered on", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "minHardwareVersion")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("cannot upgrade hardware version unless powered off"))
			})
		})
		When("vm is suspended", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateSuspended
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "minHardwareVersion")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("cannot upgrade hardware version unless powered off"))
			})
		})
	})

	When("update is performed with changed cd-rom", func() {
		When("cd-rom image kind is invalid", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].Image.Kind = invalidKind
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "image", "kind")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring(unsupportedValMsg))
			})
		})
		When("cd-rom image name is duplicate with others", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].Image.Name = ctx.vm.Spec.Hardware.Cdrom[1].Image.Name
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[1]", "image", "name")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring(duplicateValMsg))
			})
		})
	})

	When("currentSnapshot is updated", func() {
		When("snapshot ref is valid", func() {
			BeforeEach(func() {
				vmSnapshot := builder.DummyVirtualMachineSnapshot(ctx.vm.Namespace, "dummy-vm-snapshot", ctx.vm.Name)

				ctx.vm.Spec.CurrentSnapshotName = vmSnapshot.Name
			})

			It("should allow the request", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
		When("currentSnapshotName is empty", func() {
			BeforeEach(func() {
				ctx.vm.Spec.CurrentSnapshotName = ""
			})

			It("should allow the request", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Context("VirtualMachine update while VM is powered on", func() {
		BeforeEach(func() {
			ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		})

		When("Volume for PVC is added", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
					vmopv1.VirtualMachineVolume{
						Name: "dummy-new-pv-volume",
						VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "dummy-new-claim-name",
								},
							},
						},
					})
			})

			It("does not reject the request", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("GuestID is updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.GuestID = "vmwarePhoton64Guest"
			})

			It("rejects the request", func() {
				expectedReason := field.Forbidden(field.NewPath("spec", "guestID"),
					"updates to this field is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("CD-ROM name is updated", func() {
			BeforeEach(func() {
				// Name can only consist of lowercase letters and digits.
				ctx.vm.Spec.Hardware.Cdrom[0].Name = "dummy2"
			})

			It("rejects the request", func() {
				expectedReason := field.Forbidden(field.NewPath("spec", "hardware", "cdrom[0]", "name"),
					"adding new CD-ROMs is not allowed when VM is powered on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("CD-ROM image ref is updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].Image.Name = "dummy-new-image-name"
			})

			It("rejects the request", func() {
				expectedReason := field.Forbidden(field.NewPath("spec", "hardware", "cdrom[0]", "image"),
					"updates to this field is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})
	})
}

func intgTestsValidateDelete() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()

		err := ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vm)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(ctx.Namespace).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
	})
}

func intgTestsValidateCdromController() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
		ctx.vm.Spec.Hardware.Cdrom = ctx.vm.Spec.Hardware.Cdrom[:1]
		Expect(ctx.Client.Create(ctx, ctx.vm)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vm)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Context("CD-ROM controller validation with real webhook environment", func() {
		When("CD-ROM has missing required controller fields", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = ""
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = nil
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = nil
			})

			It("should deny the request with proper field paths", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "controllerType").String()))
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber").String()))
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber").String()))
				Expect(err.Error()).To(ContainSubstring("Required value"))
			})
		})

		When("CD-ROM has invalid IDE controller bus number and unit number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(10))
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(10))
			})

			It("should deny the request with proper error messages", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber").String()))
				Expect(err.Error()).To(ContainSubstring("IDE controllerBusNumber must be in the range of 0 to 1"))
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber").String()))
				Expect(err.Error()).To(ContainSubstring("IDE unitNumber must be in the range of 0 to 1"))
			})
		})

		When("CD-ROM has SATA controller with invalid bus number and unit number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(5))
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(30))
			})

			It("should deny the request with proper error messages", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber").String()))
				Expect(err.Error()).To(ContainSubstring("SATA controllerBusNumber must be in the range of 0 to 3"))
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber").String()))
				Expect(err.Error()).To(ContainSubstring("SATA unitNumber must be in the range of 0 to 29"))
			})
		})

		When("two CD-ROMs have duplicate unit numbers on same controller", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom = append(ctx.vm.Spec.Hardware.Cdrom,
					vmopv1.VirtualMachineCdromSpec{
						Name:                "cdrom2",
						ControllerType:      vmopv1.VirtualControllerTypeIDE,
						ControllerBusNumber: ptr.To(int32(0)),
						UnitNumber:          ptr.To(int32(0)),
						Image: vmopv1.VirtualMachineImageRef{
							Kind: "VirtualMachineImage",
							Name: "test-image-2",
						},
					})
			})

			It("should deny the request with duplicate error", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[1]", "unitNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("Duplicate value"))
			})
		})

		When("CD-ROM controller fields change when VM is powered on", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				Expect(ctx.Client.Update(ctx, ctx.vm)).To(Succeed())
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(1))
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(1))
			})

			It("should deny the request with power state errors", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "controllerType").String()))
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber").String()))
				Expect(err.Error()).To(ContainSubstring(field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber").String()))
				Expect(err.Error()).To(ContainSubstring("updates to this field is not allowed when VM power is on"))
			})
		})

		When("valid CD-ROM controller configuration", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(0))
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(0))
			})

			It("should allow the request", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
