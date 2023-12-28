// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe("Invoking Create", intgTestsValidateCreate)
	Describe("Invoking Update", intgTestsValidateUpdate)
	Describe("Invoking Delete", intgTestsValidateDelete)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	pvc *corev1.PersistentVolumeClaim
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}
	ctx.pvc = builder.DummyPersistentVolumeClaim()
	ctx.pvc.Namespace = ctx.Namespace
	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	type createArgs struct {
		addNonInstanceStorageLabel bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string, expectedErr error) {
		pvc := ctx.pvc.DeepCopy()
		if args.addNonInstanceStorageLabel {
			pvc.Labels = map[string]string{
				DummyLabelKey1: DummyLabelValue1,
				DummyLabelKey2: DummyLabelValue2,
			}
		}

		err := ctx.Client.Create(ctx, pvc)
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
		Entry("should allow", createArgs{}, true, nil, nil),
		Entry("should allow with labels not specific to instance storage", createArgs{addNonInstanceStorageLabel: true}, true, nil, nil),
	)
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)
	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})
	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("adding Non Instance Storage Labels", func() {
		BeforeEach(func() {
			err = ctx.Client.Create(ctx, ctx.pvc)
			Expect(err).ToNot(HaveOccurred())
		})
		JustBeforeEach(func() {
			ctx.pvc.Labels = map[string]string{
				DummyLabelKey1: DummyLabelValue1,
			}
			err = ctx.Client.Update(suite, ctx.pvc)
		})
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
	})
	AfterEach(func() {
		err = nil
		ctx = nil
	})
	When("allow deleting PVC when instance storage labels are not present", func() {
		BeforeEach(func() {
			err = ctx.Client.Create(ctx, ctx.pvc)
			Expect(err).ToNot(HaveOccurred())
		})
		JustBeforeEach(func() {
			err = ctx.Client.Delete(suite, ctx.pvc)
		})
		It("should allow the request", func() {
			Expect(err).NotTo(HaveOccurred())
		})
	})
}
