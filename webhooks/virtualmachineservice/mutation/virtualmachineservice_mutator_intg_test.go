// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Mutate",
		Label(
			testlabels.Create,
			testlabels.Update,
			testlabels.Delete,
			testlabels.EnvTest,
			testlabels.API,
			testlabels.Mutation,
			testlabels.Webhook,
		),
		intgTestsMutating,
	)
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	vmService *vmopv1.VirtualMachineService
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmService = builder.DummyVirtualMachineService()
	ctx.vmService.Namespace = ctx.Namespace

	return ctx
}

func intgTestsMutating() {
	var (
		ctx       *intgMutatingWebhookContext
		vmService *vmopv1.VirtualMachineService
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
		vmService = ctx.vmService.DeepCopy()
	})
	AfterEach(func() {
		ctx = nil
	})

	Describe("mutate", func() {
		Context("placeholder", func() {
			BeforeEach(func() {
			})

			It("should work", func() {
				err := ctx.Client.Create(ctx, vmService)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
