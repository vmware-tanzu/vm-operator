// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func intgTestsValidateCdromController() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
			config.Features.VMSharedDisks = true
		})
		ctx.vm.Spec.Hardware.Cdrom = []vmopv1.VirtualMachineCdromSpec{
			{
				Name:                "cdrom1",
				ControllerType:      vmopv1.VirtualControllerTypeIDE,
				ControllerBusNumber: ptr.To(int32(0)),
				UnitNumber:          ptr.To(int32(0)),
				Image: vmopv1.VirtualMachineImageRef{
					Kind: "VirtualMachineImage",
					Name: "test-image-1",
				},
			},
		}
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
		When("CD-ROM has missing controller type", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = ""
			})

			It("should deny the request with proper field path", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "controllerType")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("Required value"))
			})
		})

		When("CD-ROM has missing controller bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = nil
			})

			It("should deny the request with proper field path", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("Required value"))
			})
		})

		When("CD-ROM has missing unit number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = nil
			})

			It("should deny the request with proper field path", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("Required value"))
			})
		})

		When("CD-ROM has invalid IDE controller bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(10))
			})

			It("should deny the request with proper error message", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("IDE controllerBusNumber must be in the range of 0 to 1"))
			})
		})

		When("CD-ROM has invalid IDE unit number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(10))
			})

			It("should deny the request with proper error message", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("IDE unitNumber must be in the range of 0 to 1"))
			})
		})

		When("CD-ROM has SATA controller with invalid bus number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(5))
			})

			It("should deny the request with proper error message", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("SATA controllerBusNumber must be in the range of 0 to 3"))
			})
		})

		When("CD-ROM has SATA controller with invalid unit number", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(30))
			})

			It("should deny the request with proper error message", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
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

		When("CD-ROM controller changes when VM is powered on", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerType = vmopv1.VirtualControllerTypeSATA
			})

			It("should deny the request with power state error", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "controllerType")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("updates to this field is not allowed when VM power is on"))
			})
		})

		When("CD-ROM controller bus number changes when VM is powered on", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.vm.Spec.Hardware.Cdrom[0].ControllerBusNumber = ptr.To(int32(1))
			})

			It("should deny the request with power state error", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "controllerBusNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
				Expect(err.Error()).To(ContainSubstring("updates to this field is not allowed when VM power is on"))
			})
		})

		When("CD-ROM unit number changes when VM is powered on", func() {
			BeforeEach(func() {
				ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
				ctx.vm.Spec.Hardware.Cdrom[0].UnitNumber = ptr.To(int32(1))
			})

			It("should deny the request with power state error", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "hardware", "cdrom[0]", "unitNumber")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
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
