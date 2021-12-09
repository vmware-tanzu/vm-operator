// Copyright (c) 2019-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking Create", intgTestsValidateCreate)
	Describe("Invoking Update", intgTestsValidateUpdate)
	Describe("Invoking Delete", intgTestsValidateDelete)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vm      *vmopv1.VirtualMachine
	vmImage *vmopv1.VirtualMachineImage
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vm = builder.DummyVirtualMachine()
	ctx.vmImage = builder.DummyVirtualMachineImage(ctx.vm.Spec.ImageName)
	ctx.vm.Namespace = ctx.Namespace

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	type createArgs struct {
		invalidImageName bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		if args.invalidImageName {
			ctx.vm.Spec.ImageName = ""
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
		// Setting up a VirtualMachineImage for the VM
		err := ctx.Client.Create(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		err = ctx.Client.Status().Update(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		_ = ctx.Client.Delete(ctx, ctx.vmImage)
		ctx = nil
	})

	specPath := field.NewPath("spec")
	DescribeTable("create table", validateCreate,
		Entry("should work", createArgs{}, true, "", nil),
		Entry("should not work for invalid image name", createArgs{invalidImageName: true}, false,
			field.Required(specPath.Child("imageName"), "").Error(), nil),
	)
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext

		immutableFieldMsg = "field is immutable"
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		// Setting up a VirtualMachineImage for the VM
		err := ctx.Client.Create(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		err = ctx.Client.Status().Update(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		// Create the VM
		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vm)
	})
	AfterEach(func() {
		err := ctx.Client.Delete(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		ctx = nil
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

	Context("VirtualMachine update while VM is powered on", func() {
		BeforeEach(func() {
			ctx.vm.Spec.PowerState = "poweredOn"
		})

		When("Ports are updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Ports = []vmopv1.VirtualMachinePort{{
					Name: "updated-port",
				}}
			})
			It("rejects the request", func() {
				portPath := field.NewPath("spec", "ports")
				expectedReason := field.Forbidden(portPath, "updates to this filed is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("VmMetadata is updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.VmMetadata = &vmopv1.VirtualMachineMetadata{
					ConfigMapName: "updated-configmap",
				}
			})

			It("rejects the request", func() {
				metadataPath := field.NewPath("spec", "vmMetadata")
				expectedReason := field.Forbidden(metadataPath, "updates to this filed is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("NetworkInterfaces are updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.NetworkInterfaces = []vmopv1.VirtualMachineNetworkInterface{{
					NetworkName: "updated-network",
				}}
			})

			It("rejects the request", func() {
				networkPath := field.NewPath("spec", "networkInterfaces")
				expectedReason := field.Forbidden(networkPath, "updates to this filed is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("Volumes are updated", func() {
			When("a vSphere volume is added", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
						vmopv1.VirtualMachineVolume{
							Name:          "updated-vsphere-volume",
							VsphereVolume: &vmopv1.VsphereVolumeSource{},
						},
					)
				})

				It("rejects the request", func() {
					vSphereVolumePath := field.NewPath("spec", "volumes").Key("VsphereVolume")
					expectedReason := field.Forbidden(vSphereVolumePath, "updates to this filed is not allowed when VM power is on").Error()
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedReason))
				})
			})

			When("a PV is added", func() {
				BeforeEach(func() {
					ctx.vm.Spec.Volumes = append(ctx.vm.Spec.Volumes,
						vmopv1.VirtualMachineVolume{
							Name: "dummy-new-pv-volume",
							PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
								PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "dummy-new-claim-name",
								},
							},
						},
					)
				})

				It("does not reject the request", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		When("AdvancedOptions VolumeProvisioningOptions are updated", func() {
			BeforeEach(func() {
				thinProvisioning := true
				ctx.vm.Spec.AdvancedOptions = &vmopv1.VirtualMachineAdvancedOptions{
					DefaultVolumeProvisioningOptions: &vmopv1.VirtualMachineVolumeProvisioningOptions{
						ThinProvisioned: &thinProvisioning,
					},
				}
			})

			It("rejects the request", func() {
				fieldPath := field.NewPath("spec", "advancedOptions", "defaultVolumeProvisioningOptions")
				expectedReason := field.Forbidden(fieldPath, "updates to this filed is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})
	})
}

func intgTestsValidateDelete() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		// Setting up a VirtualMachineImage for the VM
		err := ctx.Client.Create(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		err = ctx.Client.Status().Update(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		// Create the VM
		err = ctx.Client.Create(ctx, ctx.vm)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vm)
	})
	AfterEach(func() {
		err := ctx.Client.Delete(ctx, ctx.vmImage)
		Expect(err).ToNot(HaveOccurred())
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(ctx.Namespace).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
