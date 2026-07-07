// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

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

	configTarget *vimv1.ConfigTarget
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.configTarget = dummyConfigTarget()

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
	)

	type createArgs struct {
		invalidName bool
		emptyID     bool
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		configTarget := ctx.configTarget.DeepCopy()

		if args.invalidName {
			configTarget.Name = "not-a-cluster-moid"
		}

		if args.emptyID {
			configTarget.Spec.ID.ID = ""
		}

		err := ctx.Client.Create(ctx, configTarget)
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
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.configTarget))).To(Succeed())
		ctx = nil
	})

	namePath := field.NewPath("metadata", "name")
	idPath := field.NewPath("spec", "id", "id")
	DescribeTable("create table", validateCreate,
		Entry("should work", createArgs{}, true, ""),
		Entry("should not work for an invalid cluster moid name", createArgs{invalidName: true}, false,
			field.Invalid(namePath, "not-a-cluster-moid", "must be a valid vSphere cluster managed object ID, e.g. domain-c21").Error()),
		Entry("should not work for an empty spec.id", createArgs{emptyID: true}, false,
			field.Required(idPath, "").Error()),
	)
}

func intgTestsValidateUpdate() {
	var (
		err error
		ctx *intgValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		err = ctx.Client.Create(ctx, ctx.configTarget)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.configTarget)
	})
	AfterEach(func() {
		Expect(ctrlclient.IgnoreNotFound(ctx.Client.Delete(ctx, ctx.configTarget))).To(Succeed())

		err = nil
		ctx = nil
	})

	When("update is performed without changing spec.id", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("update is performed with a changed spec.id", func() {
		BeforeEach(func() {
			ctx.configTarget.Spec.ID = vimv1.ManagedObjectID{ID: "domain-c22"}
		})
		It("should deny the request", func() {
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("field is immutable"))
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
		err = ctx.Client.Create(ctx, ctx.configTarget)
		Expect(err).ToNot(HaveOccurred())
	})
	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.configTarget)
	})
	AfterEach(func() {
		err = nil
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
