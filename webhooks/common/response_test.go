// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package common_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/common"
)

var _ = Describe(
	"Validation Response",
	Label(
		testlabels.EnvTest,
		testlabels.V1Alpha2,
		testlabels.Webhook,
	),
	func() {

		var (
			gr  = schema.GroupResource{Group: vmopv1.SchemeGroupVersion.Group, Resource: "VirtualMachine"}
			ctx *context.WebhookRequestContext
		)

		BeforeEach(func() {
			ctx = &context.WebhookRequestContext{
				Logger: ctrllog.Log.WithName("validate-response"),
			}
		})

		When("No errors occur", func() {
			It("Returns allowed", func() {
				response := common.BuildValidationResponse(ctx, nil, nil, nil)
				Expect(response.Allowed).To(BeTrue())
			})
		})

		When("Validation errors occur", func() {
			It("Returns denied", func() {
				validationErrs := []string{"this is required"}
				response := common.BuildValidationResponse(ctx, nil, validationErrs, nil)
				Expect(response.Allowed).To(BeFalse())
				Expect(response.Result).ToNot(BeNil())
				Expect(response.Result.Code).To(Equal(int32(http.StatusUnprocessableEntity)))
				Expect(string(response.Result.Reason)).To(ContainSubstring(validationErrs[0]))
			})
		})

		When("Validation has warnings", func() {
			It("Returns allowed, with warnings", func() {
				validationWarnings := []string{"this is deprecated"}
				response := common.BuildValidationResponse(ctx, validationWarnings, nil, nil)
				Expect(response.Allowed).To(BeTrue())
				Expect(response.Warnings).To(Equal(validationWarnings))
				Expect(response.Result).ToNot(BeNil())
				Expect(response.Result.Code).To(Equal(int32(http.StatusOK)))
			})
		})

		Context("Returns denied for expected well-known errors", func() {
			wellKnownError := func(err error, expectedCode int) {
				response := common.BuildValidationResponse(ctx, nil, nil, err)
				Expect(response.Allowed).To(BeFalse())
				Expect(response.Result).ToNot(BeNil())
				Expect(response.Result.Code).To(Equal(int32(expectedCode)))
			}

			DescribeTable("", wellKnownError,
				Entry("NotFound", apierrors.NewNotFound(gr, ""), http.StatusNotFound),
				Entry("Gone", apierrors.NewGone("gone"), http.StatusGone),
				Entry("ResourceExpired", apierrors.NewResourceExpired("expired"), http.StatusGone),
				Entry("ServiceUnavailable", apierrors.NewServiceUnavailable("unavailable"), http.StatusServiceUnavailable),
				Entry("ServiceUnavailable", apierrors.NewServiceUnavailable("unavailable"), http.StatusServiceUnavailable),
				Entry("Timeout", apierrors.NewTimeoutError("timeout", 42), http.StatusGatewayTimeout),
				Entry("Server Timeout", apierrors.NewServerTimeout(gr, "op", 42), http.StatusGatewayTimeout),
				Entry("Generic", apierrors.NewMethodNotSupported(gr, "op"), http.StatusInternalServerError),
			)
		})
	})
