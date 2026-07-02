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

	vmco *vimv1.VirtualMachineConfigOptions
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmco = builder.DummyVirtualMachineConfigOptions("vmx-21", "vmx-21")

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
		_ = ctx.Client.Delete(ctx, ctx.vmco)
		ctx = nil
	})

	When("the hardwareVersion is valid", func() {
		It("should allow the request", func() {
			Expect(ctx.Client.Create(ctx, ctx.vmco)).To(Succeed())
		})
	})

	When("the hardwareVersion is empty", func() {
		BeforeEach(func() {
			ctx.vmco.Spec.HardwareVersion = ""
		})

		It("should deny the request", func() {
			err := ctx.Client.Create(ctx, ctx.vmco)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("hardwareVersion"))
		})
	})

	When("the hardwareVersion does not match the required pattern", func() {
		BeforeEach(func() {
			ctx.vmco.Name = "not-valid"
			ctx.vmco.Spec.HardwareVersion = "not-valid"
		})

		It("should deny the request", func() {
			err := ctx.Client.Create(ctx, ctx.vmco)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("hardwareVersion"))
		})
	})

	When("metadata.name does not equal spec.hardwareVersion", func() {
		BeforeEach(func() {
			ctx.vmco.Name = "vmx-22"
		})

		It("should deny the request", func() {
			err := ctx.Client.Create(ctx, ctx.vmco)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.name must equal spec.hardwareVersion"))
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmco)).To(Succeed())
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmco)).To(Succeed())
		ctx = nil
	})

	When("the hardwareVersion is unchanged", func() {
		It("should allow the update", func() {
			Expect(ctx.Client.Update(ctx, ctx.vmco)).To(Succeed())
		})
	})

	When("the hardwareVersion is changed", func() {
		BeforeEach(func() {
			ctx.vmco.Spec.HardwareVersion = "vmx-22"
		})

		It("should deny the update", func() {
			err := ctx.Client.Update(ctx, ctx.vmco)
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
			Expect(ctx.Client.Create(ctx, ctx.vmco)).To(Succeed())
		})

		It("should allow the request", func() {
			Expect(ctx.Client.Delete(ctx, ctx.vmco)).To(Succeed())
		})
	})
}
