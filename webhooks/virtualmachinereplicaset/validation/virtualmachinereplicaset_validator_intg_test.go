// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func intgTests() {
	Describe(
		"Create",
		Label(
			testlabels.Create,
			testlabels.EnvTest,
			testlabels.V1Alpha3,
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
			testlabels.V1Alpha3,
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
			testlabels.V1Alpha3,
			testlabels.Validation,
			testlabels.Webhook,
		),
		intgTestsValidateDelete,
	)
}

type intgValidatingWebhookContext struct {
	builder.IntegrationTestContext
	rs, oldRS *vmopv1.VirtualMachineReplicaSet
}

func newIntgValidatingWebhookContext() *intgValidatingWebhookContext {
	ctx := &intgValidatingWebhookContext{
		IntegrationTestContext: *suite.NewIntegrationTestContext(),
	}

	ctx.rs = builder.DummyVirtualMachineReplicaSet()
	ctx.rs.Namespace = ctx.Namespace

	return ctx
}

func intgTestsValidateCreate() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
	})

	JustBeforeEach(func() {
		err = ctx.Client.Create(suite, ctx.rs)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("selector and template labels are specified", func() {
		Context("template is missing labels", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"": "",
				}
			})

			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "template", "metadata", "label")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector is invalid", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"-123-foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"-123-foo": "bar",
				}
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "selector")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector does not match the template label", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"foo": "baz",
				}
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "template", "metadata", "label")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector only matches some the template labels", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo":   "bar",
						"hello": "world",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"foo": "bar",
				}
			})
			It("should deny the request", func() {

				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "template", "metadata", "label")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector matches the template labels", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"foo": "bar",
				}
			})
			It("should allow the request", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})

	})
}

func intgTestsValidateUpdate() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()
		ctx.oldRS = ctx.rs
		ctx.oldRS.Namespace = ctx.Namespace

		Expect(ctx.Client.Create(ctx, ctx.rs)).To(Succeed())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Update(suite, ctx.rs)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("selector and template labels are specified", func() {
		Context("template is missing labels", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"": "",
				}
			})

			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "template", "metadata", "label")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector is invalid", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"-123-foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"-123-foo": "bar",
				}
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "selector")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector does not match the template label", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"foo": "baz",
				}
			})
			It("should deny the request", func() {
				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "template", "metadata", "label")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector only matches some the template labels", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo":   "bar",
						"hello": "world",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"foo": "bar",
				}
			})
			It("should deny the request", func() {

				Expect(err).To(HaveOccurred())
				expectedPath := field.NewPath("spec", "template", "metadata", "label")
				Expect(err.Error()).To(ContainSubstring(expectedPath.String()))
			})
		})

		Context("selector matches the template labels", func() {
			BeforeEach(func() {
				ctx.rs.Spec.Selector = &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"foo": "bar",
					},
				}
				ctx.rs.Spec.Template.Labels = map[string]string{
					"foo": "bar",
				}
			})
			It("should allow the request", func() {
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

}

func intgTestsValidateDelete() {
	var (
		ctx *intgValidatingWebhookContext
		err error
	)

	BeforeEach(func() {
		ctx = newIntgValidatingWebhookContext()

		err := ctx.Client.Create(ctx, ctx.rs)
		Expect(err).ToNot(HaveOccurred())
	})

	JustBeforeEach(func() {
		err = ctx.Client.Delete(suite, ctx.rs)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("delete is performed", func() {
		It("should allow the request", func() {
			Expect(ctx.Namespace).ToNot(BeNil())
			Expect(err).ToNot(HaveOccurred())
		})
	})
}
