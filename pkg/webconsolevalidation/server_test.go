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

	const (
		serverPath = "/validate"
		serverAddr = "localhost:8080"
	)

	var (
		inClusterConfig = func() (*rest.Config, error) {
			return &rest.Config{}, nil
		}
		addToScheme = func(*runtime.Scheme) error {
			return nil
		}
		//nolint:unparam
		newClient = func(*rest.Config, ctrlclient.Options) (ctrlclient.Client, error) {
			return builder.NewFakeClient(), nil
		}
	)

	Context("NewServer", func() {

		When("Server addr or path is empty", func() {

			It("should return an error", func() {
				_, err := webconsolevalidation.NewServer("", serverPath, nil, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("server addr and path cannot be empty"))

				_, err = webconsolevalidation.NewServer(serverAddr, "", nil, nil, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("server addr and path cannot be empty"))
			})

		})

		When("InClusterConfig returns an error", func() {

			It("should return the error", func() {
				errInClusterConfig := func() (*rest.Config, error) {
					return nil, errors.New("in-cluster config error")
				}

				_, err := webconsolevalidation.NewServer(serverAddr, serverPath, errInClusterConfig, addToScheme, newClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("in-cluster config error"))
			})

		})

		When("AddToScheme returns an error", func() {

			It("should return the error", func() {
				errAddToScheme := func(*runtime.Scheme) error {
					return errors.New("add to scheme error")
				}

				_, err := webconsolevalidation.NewServer(serverAddr, serverPath, inClusterConfig, errAddToScheme, newClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("add to scheme error"))
			})

		})

		When("NewClient returns an error", func() {

			It("should return the error", func() {
				errNewClient := func(config *rest.Config, options ctrlclient.Options) (ctrlclient.Client, error) {
					return nil, errors.New("new client error")
				}

				_, err := webconsolevalidation.NewServer(serverAddr, serverPath, inClusterConfig, addToScheme, errNewClient)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("new client error"))
			})

		})

		When("Everything works as expected", func() {

			It("should initialize a new Server successfully", func() {
				server, err := webconsolevalidation.NewServer(serverAddr, serverPath, inClusterConfig, addToScheme, newClient)
				Expect(err).NotTo(HaveOccurred())
				Expect(server).NotTo(BeNil())
				Expect(server.Addr).To(Equal(serverAddr))
				Expect(server.Path).To(Equal(serverPath))
				Expect(server.KubeClient).NotTo(BeNil())
			})

		})
	})

	Context("RunServer", func() {

		It("should start the server at the given address and path", func(done Done) {

			server := &webconsolevalidation.Server{
				Addr:       serverAddr,
				Path:       serverPath,
				KubeClient: builder.NewFakeClient(),
			}

			go func() {
				Expect(server.Run()).To(Succeed())
			}()

			// Wait for the server to start.
			time.Sleep(100 * time.Millisecond)

			resp, err := http.Get("http://" + serverAddr + serverPath)
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
			server      webconsolevalidation.Server
		)

		JustBeforeEach(func() {
			server = webconsolevalidation.Server{
				KubeClient: builder.NewFakeClient(initObjects...),
			}
		})

		AfterEach(func() {
			initObjects = nil
		})

		Context("requests with missing params", func() {

			It("should return http.StatusBadRequest (400)", func() {
				var responseCode int

				responseCode = fakeValidationRequest("/", server)
				Expect(responseCode).To(Equal(http.StatusBadRequest))

				responseCode = fakeValidationRequest("/?uuid=123", server)
				Expect(responseCode).To(Equal(http.StatusBadRequest))

				responseCode = fakeValidationRequest("/?namespace=dummy", server)
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
					server.KubeClient = fake.NewFakeClient()
					url := "/?uuid=test-uuid-1234&namespace=test-namespace"
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusInternalServerError))
				})

			})

			When("UUID matches an existing WebConsoleRequest resource", func() {

				It("should return http.StatusOK (200)", func() {
					url := "/?uuid=test-uuid-1234&namespace=test-namespace"
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusOK))
				})

			})

			When("Namespace doesn't match any WebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := "/?uuid=test-uuid-1234&namespace=non-existent-namespace"
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				})

			})

			When("UUID doesn't match any WebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := "/?uuid=non-existent-uuid&namespace=test-namespace"
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				})

			})
		})
	})
}

// fakeValidationRequest is a helper function to make a fake validation request.
// It returns the response code from the server.
func fakeValidationRequest(url string, server webconsolevalidation.Server) int {
	responseRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(server.HandleWebConsoleValidation)
	testRequest, err := http.NewRequest("GET", url, nil)
	Expect(err).NotTo(HaveOccurred())
	handler.ServeHTTP(responseRecorder, testRequest)
	response := responseRecorder.Result()
	Expect(response).NotTo(BeNil())
	Expect(response.Body).NotTo(BeNil())
	Expect(response.Body.Close()).To(Succeed())

	return response.StatusCode
}
