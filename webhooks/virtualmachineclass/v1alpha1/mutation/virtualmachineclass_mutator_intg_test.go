// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package mutation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	XDescribe("Invoking Mutation", intgTestsMutating)
}

type intgMutatingWebhookContext struct {
	builder.IntegrationTestContext
	vmClass *vmopv1.VirtualMachineClass
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmClass = builder.DummyVirtualMachineClass()

	return ctx
}

func intgTestsMutating() {
	var (
		ctx     *intgMutatingWebhookContext
		vmClass *vmopv1.VirtualMachineClass
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
		vmClass = ctx.vmClass.DeepCopy()
	})
	AfterEach(func() {
		ctx = nil
	})

	Describe("mutate", func() {
		Context("placeholder", func() {
			BeforeEach(func() {
			})

			It("should work", func() {
				err := ctx.Client.Create(ctx, vmClass)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
