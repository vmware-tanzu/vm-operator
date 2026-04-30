// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
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
	vmService *vmopv1.VirtualMachineService
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmService = builder.DummyVirtualMachineService()
	ctx.vmService.Namespace = ctx.Namespace

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	validateCreate := func(expectedAllowed bool, expectedReason string, expectedErr error) {
		vmService := ctx.vmService.DeepCopy()

		if !expectedAllowed {
			vmService.Spec.Type = ""
		}

		err := ctx.Client.Create(ctx, vmService)
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
		ctx = nil
	})

	DescribeTable("create table", validateCreate,
		Entry("should work", true, "", nil),
		Entry("should not work for invalid", false, "spec.type: Required value", nil),
	)

	Describe("CRD schema and CEL (ipFamilies and ipFamilyPolicy)", func() {
		It("allows LoadBalancer with dual-stack ipFamilies and PreferDualStack policy", func() {
			vmSvc := ctx.vmService.DeepCopy()
			vmSvc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}
			p := corev1.IPFamilyPolicyPreferDualStack
			vmSvc.Spec.IPFamilyPolicy = &p
			Expect(ctx.Client.Create(ctx, vmSvc)).To(Succeed())
		})

		It("rejects ExternalName when ipFamilies is set", func() {
			vmSvc := ctx.vmService.DeepCopy()
			vmSvc.Spec.Type = vmopv1.VirtualMachineServiceTypeExternalName
			vmSvc.Spec.ExternalName = "backend.example.com."
			vmSvc.Spec.Ports = nil
			vmSvc.Spec.Selector = nil
			vmSvc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol}
			err := ctx.Client.Create(ctx, vmSvc)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("ipFamilies and ipFamilyPolicy may not be set when type is ExternalName"))
		})

		It("rejects ExternalName when ipFamilyPolicy is set", func() {
			vmSvc := ctx.vmService.DeepCopy()
			vmSvc.Spec.Type = vmopv1.VirtualMachineServiceTypeExternalName
			vmSvc.Spec.ExternalName = "backend.example.com."
			vmSvc.Spec.Ports = nil
			vmSvc.Spec.Selector = nil
			p := corev1.IPFamilyPolicySingleStack
			vmSvc.Spec.IPFamilyPolicy = &p
			err := ctx.Client.Create(ctx, vmSvc)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("ipFamilies and ipFamilyPolicy may not be set when type is ExternalName"))
		})

		It("rejects duplicate ipFamilies entries", func() {
			vmSvc := ctx.vmService.DeepCopy()
			vmSvc.Spec.IPFamilies = []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv4Protocol}
			err := ctx.Client.Create(ctx, vmSvc)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("ipFamilies must not contain duplicate entries"))
		})

		It("rejects ipFamilies values outside IPv4 and IPv6", func() {
			vmSvc := ctx.vmService.DeepCopy()
			vmSvc.Spec.IPFamilies = []corev1.IPFamily{"bogus"}
			err := ctx.Client.Create(ctx, vmSvc)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			// OpenAPI items.enum may reject before CEL; either error is acceptable.
			Expect(err.Error()).To(Or(
				ContainSubstring("each ipFamilies entry must be IPv4 or IPv6"),
				ContainSubstring("bogus"),
				ContainSubstring("supported values"),
			))
		})

		It("rejects more than two ipFamilies entries", func() {
			vmSvc := ctx.vmService.DeepCopy()
			vmSvc.Spec.IPFamilies = []corev1.IPFamily{
				corev1.IPv4Protocol,
				corev1.IPv6Protocol,
				corev1.IPv4Protocol,
			}
			err := ctx.Client.Create(ctx, vmSvc)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			// OpenAPI maxItems vs CEL duplicate rule: message text differs by apiserver version.
			Expect(err.Error()).To(ContainSubstring("ipFamilies"))
		})

		It("rejects unknown ipFamilyPolicy value", func() {
			vmSvc := ctx.vmService.DeepCopy()
			bad := corev1.IPFamilyPolicy("bogus")
			vmSvc.Spec.IPFamilyPolicy = &bad
			err := ctx.Client.Create(ctx, vmSvc)
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
			Expect(err.Error()).To(Or(
				ContainSubstring("ipFamilyPolicy"),
				ContainSubstring("Supported values"),
				ContainSubstring("enum"),
			))
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		err = ctx.Client.Create(ctx, ctx.vmService)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.vmService)
	})
	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("update is performed without spec changes", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
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
		err = ctx.Client.Create(ctx, ctx.vmService)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.vmService)
	})
	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(ctx.Namespace).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
