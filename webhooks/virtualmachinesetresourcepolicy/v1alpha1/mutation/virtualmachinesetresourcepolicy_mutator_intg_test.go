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
	vmRP *vmopv1.VirtualMachineSetResourcePolicy
}

func newIntgMutatingWebhookContext() *intgMutatingWebhookContext {
	ctx := &intgMutatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmRP = builder.DummyVirtualMachineSetResourcePolicy()
	ctx.vmRP.Namespace = ctx.Namespace

	return ctx
}

func intgTestsMutating() {
	var (
		ctx  *intgMutatingWebhookContext
		vmRP *vmopv1.VirtualMachineSetResourcePolicy
	)

	BeforeEach(func() {
		ctx = newIntgMutatingWebhookContext()
		vmRP = ctx.vmRP.DeepCopy()
	})
	AfterEach(func() {
		ctx = nil
	})

	Describe("mutate", func() {
		Context("placeholder", func() {
			BeforeEach(func() {
			})

			It("should work", func() {
				err := ctx.Client.Create(ctx, vmRP)
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
