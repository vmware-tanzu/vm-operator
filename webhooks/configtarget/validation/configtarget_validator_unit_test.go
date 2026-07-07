// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

// configTargetCounter ensures each dummyConfigTarget has a unique name, since
// ConfigTarget is cluster-scoped and the integration tests share a single
// envtest API server across parallel specs.
var configTargetCounter atomic.Int64

func unitTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateCreate,
	)
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.API,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateDelete,
	)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook

	configTarget    *vimv1.ConfigTarget
	oldConfigTarget *vimv1.ConfigTarget
}

func dummyConfigTarget() *vimv1.ConfigTarget {
	name := fmt.Sprintf("domain-c%d", configTargetCounter.Add(1))

	return &vimv1.ConfigTarget{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: vimv1.ConfigTargetSpec{
			ID: vimv1.ManagedObjectID{ID: name},
		},
	}
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	configTarget := dummyConfigTarget()
	obj, err := builder.ToUnstructured(configTarget)
	Expect(err).ToNot(HaveOccurred())

	var (
		oldConfigTarget *vimv1.ConfigTarget
		oldObj          *unstructured.Unstructured
	)

	if isUpdate {
		oldConfigTarget = configTarget.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldConfigTarget)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj),
		configTarget:                        configTarget,
		oldConfigTarget:                     oldConfigTarget,
	}
}

func unitTestsValidateCreate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type createArgs struct {
		id *string
	}

	validateCreate := func(args createArgs, expectedAllowed bool, expectedReason string) {
		var err error

		if args.id != nil {
			ctx.configTarget.Spec.ID.ID = *args.id
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.configTarget)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(Equal(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	idPath := field.NewPath("spec", "id", "id")
	DescribeTable("create table", validateCreate,
		Entry("should allow valid", createArgs{}, true, ""),
		Entry("should deny an empty spec.id", createArgs{id: new(string)}, false,
			field.Required(idPath, "").Error()),
	)
}

func unitTestsValidateUpdate() {
	var (
		ctx *unitValidatingWebhookContext
	)

	type updateArgs struct {
		id vimv1.ManagedObjectID
	}

	validateUpdate := func(args updateArgs, expectedAllowed bool, expectedReason string) {
		var err error

		if args.id != (vimv1.ManagedObjectID{}) {
			ctx.configTarget.Spec.ID = args.id
		}

		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.configTarget)
		Expect(err).ToNot(HaveOccurred())

		response := ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(expectedAllowed))

		if expectedReason != "" {
			Expect(string(response.Result.Reason)).To(ContainSubstring(expectedReason))
		}
	}

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})
	AfterEach(func() {
		ctx = nil
	})

	immutableFieldMsg := "field is immutable"
	DescribeTable("update table", validateUpdate,
		Entry("should allow when spec.id is unchanged", updateArgs{}, true, ""),
		Entry("should deny when spec.id changes to domain-c22",
			updateArgs{id: vimv1.ManagedObjectID{ID: "domain-c22"}}, false, immutableFieldMsg),
	)

	When("the update is performed while object deletion", func() {
		var response admission.Response

		JustBeforeEach(func() {
			t := metav1.Now()
			ctx.WebhookRequestContext.Obj.SetDeletionTimestamp(&t)
			response = ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}

func unitTestsValidateDelete() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	When("the delete is performed", func() {
		JustBeforeEach(func() {
			response = ctx.ValidateDelete(&ctx.WebhookRequestContext)
		})

		It("should allow the request", func() {
			Expect(response.Allowed).To(BeTrue())
			Expect(response.Result).ToNot(BeNil())
		})
	})
}
