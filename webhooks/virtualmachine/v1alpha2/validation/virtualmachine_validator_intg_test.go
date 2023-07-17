// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking Create", intgTestsValidateCreate)
	Describe("Invoking Update", intgTestsValidateUpdate)
	Describe("Invoking Delete", intgTestsValidateDelete)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	vm *vmopv1.VirtualMachine
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vm = builder.DummyVirtualMachineA2()
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
	})

	AfterEach(func() {
		ctx.AfterEach()
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
	const (
		immutableFieldMsg = "field is immutable"
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
			ctx.vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
		})

		When("Bootstrap is updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Bootstrap = vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						RawCloudConfig: corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{},
							Key:                  "",
							Optional:             nil,
						},
					},
				}
			})

			It("rejects the request", func() {
				expectedReason := field.Forbidden(field.NewPath("spec", "bootstrap"),
					"updates to this field is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
		})

		When("Network is updated", func() {
			BeforeEach(func() {
				ctx.vm.Spec.Network = vmopv1.VirtualMachineNetworkSpec{
					HostName: "my-new-name",
				}
			})

			It("rejects the request", func() {
				expectedReason := field.Forbidden(field.NewPath("spec", "network"),
					"updates to this field is not allowed when VM power is on").Error()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedReason))
			})
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
