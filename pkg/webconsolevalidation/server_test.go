// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webconsolevalidation_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/webconsolevalidation"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func serverUnitTests() {

	Describe("web-console validation server unit tests", func() {

		var (
			initObjects []client.Object
		)

		JustBeforeEach(func() {
			webconsolevalidation.K8sClient = builder.NewFakeClient(initObjects...)
		})

		AfterEach(func() {
			initObjects = nil
			webconsolevalidation.K8sClient = nil
		})

		Context("requests with missing params", func() {

			It("should return http.StatusBadRequest (400)", func() {
				var responseCode int

				responseCode = fakeValidationRequest("/")
				Expect(responseCode).To(Equal(http.StatusBadRequest))

				responseCode = fakeValidationRequest("/?uuid=123")
				Expect(responseCode).To(Equal(http.StatusBadRequest))

				responseCode = fakeValidationRequest("/?namespace=dummy")
				Expect(responseCode).To(Equal(http.StatusBadRequest))
			})

		})

		Context("requests with a uuid param set", func() {

			BeforeEach(func() {
				wcr := &vmopv1.WebConsoleRequest{}
				wcr.Namespace = "dummy-namespace"
				wcr.Labels = map[string]string{
					v1alpha1.UUIDLabelKey: "dummy-uuid-1234",
				}
				initObjects = append(initObjects, wcr)
			})

			When("UUID matches an existing WebConsoleRequest resource", func() {

				It("should return http.StatusOK (200)", func() {
					url := "/?uuid=dummy-uuid-1234&namespace=dummy-namespace"
					responseCode := fakeValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusOK))
				})

			})

			When("Namespace doesn't match any WebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := "/?uuid=dummy-uuid-1234&namespace=non-existent-namespace"
					responseCode := fakeValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				})

			})

			When("UUID doesn't match any WebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := "/?uuid=non-existent-uuid&namespace=dummy-namespace"
					responseCode := fakeValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				})

			})
		})
	})
}

// fakeValidationRequest is a helper function to make a fake validation request.
// It returns the response code from the server.
func fakeValidationRequest(url string) int {
	responseRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(webconsolevalidation.HandleWebConsoleValidation)
	testRequest, _ := http.NewRequest("GET", url, nil)
	handler.ServeHTTP(responseRecorder, testRequest)
	response := responseRecorder.Result()
	_ = response.Body.Close()

	return response.StatusCode
}
