// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

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
	vmService *vmopv1.VirtualMachineService
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.vmService = builder.DummyVirtualMachineServiceA2()
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

	When("update is performed with changed ... placeholder", func() {
		BeforeEach(func() {
		})
		It("should deny the request", func() {
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
