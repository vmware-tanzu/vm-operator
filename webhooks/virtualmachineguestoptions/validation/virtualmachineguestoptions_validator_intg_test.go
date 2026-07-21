// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

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

	vmgo *vimv1.VirtualMachineGuestOptions
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmgo = builder.DummyVirtualMachineGuestOptions("otherlinux64guest", "otherLinux64Guest")

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	AfterEach(func() {
		_ = ctx.Client.Delete(ctx, ctx.vmgo)
		ctx = nil
	})

	When("spec.id is provided and metadata.name matches its DNS-safe transform", func() {
		It("should allow the request", func() {
			Expect(ctx.Client.Create(ctx, ctx.vmgo)).To(Succeed())
		})
	})

	When("spec.id is empty", func() {
		BeforeEach(func() {
			ctx.vmgo.Spec.ID = ""
		})

		It("should deny the request", func() {
			err := ctx.Client.Create(ctx, ctx.vmgo)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("id must be provided"))
		})
	})

	When("metadata.name does not equal the DNS-safe transform of spec.id", func() {
		BeforeEach(func() {
			ctx.vmgo.Name = "not-a-match"
		})

		It("should deny the request", func() {
			err := ctx.Client.Create(ctx, ctx.vmgo)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.name must equal the DNS-safe transform of spec.id"))
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmgo)).To(Succeed())
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmgo)).To(Succeed())
		ctx = nil
	})

	When("spec.id is unchanged", func() {
		It("should allow the update", func() {
			Expect(ctx.Client.Update(ctx, ctx.vmgo)).To(Succeed())
		})
	})

	When("spec.id is changed", func() {
		BeforeEach(func() {
			ctx.vmgo.Spec.ID = "dos"
		})

		It("should deny the update", func() {
			err := ctx.Client.Update(ctx, ctx.vmgo)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field is immutable"))
		})
	})
}

func intgTestsValidateDelete() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	AfterEach(func() {
		ctx = nil
	})

	When("delete is performed", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, ctx.vmgo)).To(Succeed())
		})

		It("should allow the request", func() {
			Expect(ctx.Client.Delete(ctx, ctx.vmgo)).To(Succeed())
		})
	})
}
