// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

type testParams struct {
	setup         func(ctx *unitValidatingWebhookContext)
	validate      func(ctx *unitValidatingWebhookContext, response admission.Response)
	expectAllowed bool
}

func unitTests() {
	Describe(
		"Update",
		Label(
			testlabels.Update,
			testlabels.V1Alpha3,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateUpdate,
	)
	Describe(
		"Delete",
		Label(
			testlabels.Delete,
			testlabels.V1Alpha3,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestsValidateDelete,
	)
	Describe(
		"TemplateObjectMetaAndSelectorMatching",
		Label(
			testlabels.V1Alpha3,
			testlabels.Validation,
			testlabels.Webhook,
		),
		unitTestVaildateTemplateObjectMetaAndSelectorMatching,
	)
}

type unitValidatingWebhookContext struct {
	builder.UnitTestContextForValidatingWebhook
	rs, oldRS *vmopv1.VirtualMachineReplicaSet
}

func newUnitTestContextForValidatingWebhook(isUpdate bool) *unitValidatingWebhookContext {
	rs := builder.DummyVirtualMachineReplicaSet()
	rs.Name = "dummy-rs-for-webhook-validation"
	rs.Namespace = "dummy-rs-namespace-for-webhook-validation"
	obj, err := builder.ToUnstructured(rs)
	Expect(err).ToNot(HaveOccurred())

	var (
		oldRS  *vmopv1.VirtualMachineReplicaSet
		oldObj *unstructured.Unstructured
	)

	if isUpdate {
		oldRS = rs.DeepCopy()
		oldObj, err = builder.ToUnstructured(oldRS)
		Expect(err).ToNot(HaveOccurred())
	}

	return &unitValidatingWebhookContext{
		UnitTestContextForValidatingWebhook: *suite.NewUnitTestContextForValidatingWebhook(obj, oldObj, nil...),
		rs:                                  rs,
		oldRS:                               oldRS,
	}
}

func unitTestVaildateTemplateObjectMetaAndSelectorMatching() {
	var (
		ctx *unitValidatingWebhookContext
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(false)
	})
	AfterEach(func() {
		ctx = nil
	})

	doTest := func(args testParams) {
		args.setup(ctx)

		var err error
		ctx.WebhookRequestContext.Obj, err = builder.ToUnstructured(ctx.rs)
		Expect(err).ToNot(HaveOccurred())

		fmt.Printf("context is: %+v", ctx.rs)

		response := ctx.ValidateCreate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(args.expectAllowed))

		if args.validate != nil {
			args.validate(ctx, response)
		}

		// Template metadata validations, and label matching has the same vaildation for create and update.
		response = ctx.ValidateUpdate(&ctx.WebhookRequestContext)
		Expect(response.Allowed).To(Equal(args.expectAllowed))

		if args.validate != nil {
			args.validate(ctx, response)
		}
	}

	Context("Label selector", func() {
		specPath := field.NewPath("spec")

		DescribeTable("label selector matching validations", doTest,
			Entry("should return error on mismatch",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.rs.Spec.Selector = &metav1.LabelSelector{
							MatchLabels: map[string]string{"foo": "bar"},
						}
						ctx.rs.Spec.Template.Labels = map[string]string{"foo": "baz"}
					},
					validate: func(ctx *unitValidatingWebhookContext, response admission.Response) {
						selector, err := metav1.LabelSelectorAsSelector(ctx.rs.Spec.Selector)
						Expect(err).NotTo(HaveOccurred())

						err = field.Invalid(
							specPath.Child("template", "metadata", "labels"),
							ctx.rs.Spec.Template.ObjectMeta.Labels,
							fmt.Sprintf("must match spec.selector %q", selector.String()),
						)
						Expect(string(response.Result.Reason)).To(ContainSubstring(err.Error()))
					},
					expectAllowed: false,
				},
			),
			Entry("should return error on missing labels",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.rs.Spec.Selector = &metav1.LabelSelector{
							MatchLabels: map[string]string{"foo": "bar"},
						}
						ctx.rs.Spec.Template.Labels = map[string]string{"": ""}
					},
					validate: func(ctx *unitValidatingWebhookContext, response admission.Response) {
						selector, err := metav1.LabelSelectorAsSelector(ctx.rs.Spec.Selector)
						Expect(err).NotTo(HaveOccurred())

						err = field.Invalid(
							specPath.Child("template", "metadata", "labels"),
							ctx.rs.Spec.Template.ObjectMeta.Labels,
							fmt.Sprintf("must match spec.selector %q", selector.String()),
						)
						Expect(string(response.Result.Reason)).To(ContainSubstring(err.Error()))
					},
					expectAllowed: false,
				},
			),
			Entry("should return error if all selectors don't match",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.rs.Spec.Selector = &metav1.LabelSelector{
							MatchLabels: map[string]string{"foo": "bar", "hello": "world"},
						}
						ctx.rs.Spec.Template.Labels = map[string]string{"foo": "bar"}
					},
					validate: func(ctx *unitValidatingWebhookContext, response admission.Response) {
						selector, err := metav1.LabelSelectorAsSelector(ctx.rs.Spec.Selector)
						Expect(err).NotTo(HaveOccurred())

						err = field.Invalid(
							specPath.Child("template", "metadata", "labels"),
							ctx.rs.Spec.Template.ObjectMeta.Labels,
							fmt.Sprintf("must match spec.selector %q", selector.String()),
						)
						Expect(string(response.Result.Reason)).To(ContainSubstring(err.Error()))
					},
					expectAllowed: false,
				},
			),
			Entry("should not return error on match",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.rs.Spec.Selector = &metav1.LabelSelector{
							MatchLabels: map[string]string{"foo": "bar"},
						}
						ctx.rs.Spec.Template.Labels = map[string]string{"foo": "bar"}
					},
					expectAllowed: true,
				},
			),
			Entry("should not return error for invalid selector",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.rs.Spec.Selector = &metav1.LabelSelector{
							MatchLabels: map[string]string{"-123-foo": "bar"},
						}
						ctx.rs.Spec.Template.Labels = map[string]string{"-123-foo": "bar"}
					},
					validate: func(ctx *unitValidatingWebhookContext, response admission.Response) {
						selector, err := metav1.LabelSelectorAsSelector(ctx.rs.Spec.Selector)
						Expect(selector).To(BeNil())
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("name part must consist of alphanumeric characters"))
					},
					expectAllowed: false,
				},
			),
			Entry("should return error on invalid labels and annotations",
				testParams{
					setup: func(ctx *unitValidatingWebhookContext) {
						ctx.rs.Spec.Template.Labels = map[string]string{
							"foo":          "$invalid-key",
							"bar":          strings.Repeat("a", 64) + "too-long-value",
							"/invalid-key": "foo",
						}
						ctx.rs.Spec.Template.Annotations = map[string]string{
							"/invalid-key": "foo",
						}
					},
					validate: func(ctx *unitValidatingWebhookContext, response admission.Response) {
						_, err := metav1.LabelSelectorAsSelector(ctx.rs.Spec.Selector)
						Expect(err).NotTo(HaveOccurred())

						Expect(string(response.Result.Reason)).To(ContainSubstring("spec.template.metadata.labels: Invalid value"))
					},
					expectAllowed: false,
				},
			),
		)
	})
}

func unitTestsValidateUpdate() {
	var (
		ctx      *unitValidatingWebhookContext
		response admission.Response
	)

	BeforeEach(func() {
		ctx = newUnitTestContextForValidatingWebhook(true)
	})
	AfterEach(func() {
		ctx = nil
	})

	When("the update is performed while object deletion", func() {
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
