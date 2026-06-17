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
	vmConfigOptions *vimv1.VirtualMachineConfigOptions
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	obj := &vimv1.VirtualMachineConfigOptions{}
	obj.Name = "dummy-vmconfigoptions"
	obj.Spec.HardwareVersion = "vmx-19"
	ctx.vmConfigOptions = obj

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
		ctx = nil
	})

	When("the VirtualMachineConfigOptions is created with a valid hardwareVersion", func() {
		It("should allow the request", func() {
			Expect(ctx.Client.Create(ctx, ctx.vmConfigOptions)).To(Succeed())
		})
	})
}

func intgTestsValidateUpdate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		Expect(ctx.Client.Create(ctx, ctx.vmConfigOptions)).To(Succeed())
	})

	AfterEach(func() {
		Expect(ctx.Client.Delete(ctx, ctx.vmConfigOptions)).To(Succeed())
		ctx = nil
	})

	When("the hardwareVersion is unchanged", func() {
		It("should allow the update", func() {
			Expect(ctx.Client.Update(ctx, ctx.vmConfigOptions)).To(Succeed())
		})
	})

	When("the hardwareVersion is changed", func() {
		It("should reject the update", func() {
			ctx.vmConfigOptions.Spec.HardwareVersion = "vmx-21"
			err := ctx.Client.Update(ctx, ctx.vmConfigOptions)
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

	When("the delete is performed", func() {
		BeforeEach(func() {
			Expect(ctx.Client.Create(ctx, ctx.vmConfigOptions)).To(Succeed())
		})

		It("should allow the request", func() {
			Expect(ctx.Client.Delete(ctx, ctx.vmConfigOptions)).To(Succeed())
		})
	})
}
