// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	XDescribe("Invoking Mutation", intgTestsMutating)
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	vmService *vmopv1.VirtualMachineService
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmService = builder.DummyVirtualMachineServiceA2()
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
