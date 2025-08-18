// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/test/builder"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

var _ = Describe("Validating RetrieveDefaultImagePublishContentLibrary",
	Label(
		testlabels.EnvTest,
		testlabels.API,
		testlabels.Webhook,
	),
	func() {
		var (
			ctx *builder.UnitTestContext
		)
		BeforeEach(func() {
			ctx = builder.NewUnitTestContext()
		})

		When("there is no CL", func() {
			It("should return NotFound err", func() {
				cl, err := common.RetrieveDefaultImagePublishContentLibrary(ctx, ctx.Client, "")
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
				Expect(cl).To(BeNil())
			})
		})

		When("there is no default CL", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, &imgregv1a1.ContentLibrary{
					ObjectMeta: metav1.ObjectMeta{Name: builder.DummyContentLibraryName + "-0"},
				})).To(Succeed())
			})

			It("should return NotFound err", func() {
				cl, err := common.RetrieveDefaultImagePublishContentLibrary(ctx, ctx.Client, "")
				Expect(err).To(HaveOccurred())
				Expect(apierrors.IsNotFound(err)).To(BeTrue())
				Expect(cl).To(BeNil())
			})
		})

		When("there is a default CL", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
					builder.DummyContentLibraryName+"-0", "", ""))).To(Succeed())
			})

			It("should succeed", func() {
				cl, err := common.RetrieveDefaultImagePublishContentLibrary(ctx, ctx.Client, "")
				Expect(err).ToNot(HaveOccurred())
				Expect(cl.Name).To(Equal(builder.DummyContentLibraryName + "-0"))
			})
		})

		When("there is multiple default CLs", func() {
			BeforeEach(func() {
				Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
					builder.DummyContentLibraryName+"-0", "", ""))).To(Succeed())
				Expect(ctx.Client.Create(ctx, builder.DummyDefaultContentLibrary(
					builder.DummyContentLibraryName+"-1", "", ""))).To(Succeed())
			})

			It("should return bad request error", func() {
				_, err := common.RetrieveDefaultImagePublishContentLibrary(ctx, ctx.Client, "")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("more than one default ContentLibrary found: dummy-cl-0, dummy-cl-1"))
			})
		})
	})

var _ = Describe("Validating ConvertFieldErrorsToStrings",
	Label(
		testlabels.EnvTest,
		testlabels.API,
		testlabels.Webhook,
	),
	func() {
		When("Error List is Nil", func() {
			It("should return empty string slice", func() {
				Expect(common.ConvertFieldErrorsToStrings(nil)).To(HaveLen(0))
			})
		})
		When("Error List is empty", func() {
			It("should return empty string slice", func() {
				Expect(common.ConvertFieldErrorsToStrings(field.ErrorList{})).To(HaveLen(0))
			})
		})
		When("Error List is not Nil", func() {
			It("should return a string slice with same length", func() {
				var fieldErrs field.ErrorList
				fieldErrs = append(fieldErrs, field.Invalid(field.NewPath("path1"), "", ""))
				fieldErrs = append(fieldErrs, field.Forbidden(field.NewPath("path2"), ""))
				errStrings := common.ConvertFieldErrorsToStrings(fieldErrs)
				Expect(errStrings).To(HaveLen(len(fieldErrs)))
			})
		})
		When("Error List is a list with nil errors", func() {
			It("should return a string slice with nil errors removed", func() {
				var fieldErrs field.ErrorList
				fieldErrs = append(fieldErrs, field.Invalid(field.NewPath("path1"), "", ""))
				fieldErrs = append(fieldErrs, nil)
				fieldErrs = append(fieldErrs, field.Forbidden(field.NewPath("path2"), ""))
				fieldErrs = append(fieldErrs, nil)
				errStrings := common.ConvertFieldErrorsToStrings(fieldErrs)
				Expect(errStrings).To(HaveLen(2))
			})
		})
	})
