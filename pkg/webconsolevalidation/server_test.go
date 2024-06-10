// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webconsolevalidation_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/webconsolevalidation"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func serverUnitTests() {

	Context("InitServer", func() {

		var (
			originalInClusterConfig func() (*rest.Config, error)
			originalNewClient       func(config *rest.Config, options ctrlclient.Options) (ctrlclient.Client, error)
			originalAddToScheme     func(*runtime.Scheme) error
		)

		BeforeEach(func() {
			originalInClusterConfig = webconsolevalidation.InClusterConfig
			originalNewClient = webconsolevalidation.NewClient
			originalAddToScheme = webconsolevalidation.AddToScheme

			// Init all the function variables to return without any error.
			webconsolevalidation.InClusterConfig = func() (*rest.Config, error) {
				return &rest.Config{}, nil
			}
			webconsolevalidation.NewClient = func(*rest.Config, ctrlclient.Options) (ctrlclient.Client, error) {
				return builder.NewFakeClient(), nil
			}
			webconsolevalidation.AddToScheme = func(*runtime.Scheme) error {
				return nil
			}
		})

		AfterEach(func() {
			webconsolevalidation.InClusterConfig = originalInClusterConfig
			webconsolevalidation.NewClient = originalNewClient
			webconsolevalidation.AddToScheme = originalAddToScheme
		})

		When("InClusterConfig returns an error", func() {

			It("should return the error", func() {
				webconsolevalidation.InClusterConfig = func() (*rest.Config, error) {
					return nil, errors.New("in-cluster config error")
				}

				err := webconsolevalidation.InitServer()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("in-cluster config error"))
			})

		})

		When("AddToScheme returns an error", func() {

			It("should return the error", func() {
				webconsolevalidation.AddToScheme = func(*runtime.Scheme) error {
					return errors.New("add to scheme error")
				}

				err := webconsolevalidation.InitServer()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("add to scheme error"))
			})

		})

		When("NewClient returns an error", func() {

			It("should return the error", func() {
				webconsolevalidation.NewClient = func(config *rest.Config, options ctrlclient.Options) (ctrlclient.Client, error) {
					return nil, errors.New("new client error")
				}

				err := webconsolevalidation.InitServer()
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("new client error"))
			})

		})

		When("Everything works as expected", func() {

			It("should initialize a K8sClient successfully", func() {
				Expect(webconsolevalidation.InitServer()).To(Succeed())
				Expect(webconsolevalidation.K8sClient).NotTo(BeNil())
			})

		})
	})

	Context("RunServer", func() {

		It("should start the server at the given address and path", func(done Done) {
			addr := "localhost:8080"
			path := "/validate"

			go func() {
				Expect(webconsolevalidation.RunServer(addr, path)).To(Succeed())
			}()

			// Wait for the server to start.
			time.Sleep(100 * time.Millisecond)

			resp, err := http.Get("http://" + addr + path)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
			// Verify the server path is correct.
			Expect(resp.StatusCode).NotTo(Equal(http.StatusNotFound))
			Expect(resp.Body).NotTo(BeNil())
			Expect(resp.Body.Close()).To(Succeed())

			close(done)
		}, 1.0) // Time out this after 1 second.

	})

	Context("HandleWebConsoleValidation", func() {

		var (
			initObjects []ctrlclient.Object
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

		Context("requests with all params set", func() {

			BeforeEach(func() {
				wcr := &vmopv1a1.WebConsoleRequest{}
				wcr.Namespace = "test-namespace"
				wcr.Labels = map[string]string{
					v1alpha1.UUIDLabelKey: "test-uuid-1234",
				}
				initObjects = append(initObjects, wcr)
			})

			When("an error occurs while getting the WebConsoleRequest resource", func() {

				It("should return http.StatusInternalServerError (500)", func() {
					// Use the default fake client to cause an error when getting the CR.
					webconsolevalidation.K8sClient = fake.NewFakeClient()
					url := "/?uuid=test-uuid-1234&namespace=test-namespace"
					responseCode := fakeValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusInternalServerError))
				})

			})

			When("UUID matches an existing WebConsoleRequest resource", func() {

				It("should return http.StatusOK (200)", func() {
					url := "/?uuid=test-uuid-1234&namespace=test-namespace"
					responseCode := fakeValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusOK))
				})

			})

			When("Namespace doesn't match any WebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := "/?uuid=test-uuid-1234&namespace=non-existent-namespace"
					responseCode := fakeValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				})

			})

			When("UUID doesn't match any WebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := "/?uuid=non-existent-uuid&namespace=test-namespace"
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
	testRequest, err := http.NewRequest("GET", url, nil)
	Expect(err).NotTo(HaveOccurred())
	handler.ServeHTTP(responseRecorder, testRequest)
	response := responseRecorder.Result()
	Expect(response).NotTo(BeNil())
	Expect(response.Body).NotTo(BeNil())
	Expect(response.Body.Close()).To(Succeed())

	return response.StatusCode
}
