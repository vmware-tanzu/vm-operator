// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package webconsolevalidation_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/webconsolevalidation"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func serverUnitTests() {

	const (
		serverPath = "/validate"
		serverAddr = "localhost:8080"
	)

	Context("NewServer", func() {

		When("Server addr or path is empty", func() {

			It("should return an error", func() {
				_, err := webconsolevalidation.NewServer("", serverPath, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("server addr and path cannot be empty"))

				_, err = webconsolevalidation.NewServer(serverAddr, "", nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal("server addr and path cannot be empty"))
			})

		})

		When("All Server parameters are provided", func() {

			It("should initialize a new Server successfully", func() {
				server, err := webconsolevalidation.NewServer(serverAddr, serverPath, fake.NewFakeClient())
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

			const (
				wcrUUID   = "test-uuid-wcr"
				vmwcrUUID = "test-uuid-vmwcr"
				namespace = "test-namespace"
			)

			BeforeEach(func() {
				wcr := &vmopv1a1.WebConsoleRequest{}
				wcr.Namespace = namespace
				wcr.Labels = map[string]string{
					webconsolevalidation.UUIDLabelKey: wcrUUID,
				}
				initObjects = append(initObjects, wcr)

				vmwcr := &vmopv1.VirtualMachineWebConsoleRequest{}
				vmwcr.Namespace = namespace
				vmwcr.Labels = map[string]string{
					webconsolevalidation.UUIDLabelKey: vmwcrUUID,
				}
				initObjects = append(initObjects, vmwcr)
			})

			When("an error occurs while getting the VirtualMachineWebConsoleRequest resource", func() {

				It("should return http.StatusInternalServerError (500)", func() {
					// Use a fake client with the v1alpha1 scheme only to cause an
					// error when getting the VirtualMachineWebConsoleRequestList CR.
					scheme := runtime.NewScheme()
					Expect(vmopv1a1.AddToScheme(scheme)).To(Succeed())
					server.KubeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

					url := fmt.Sprintf("/?uuid=%s&namespace=%s", vmwcrUUID, namespace)
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusInternalServerError))
				})

			})

			When("UUID matches an existing VirtualMachineWebConsoleRequest resource", func() {

				It("should return http.StatusOK (200)", func() {
					url := fmt.Sprintf("/?uuid=%s&namespace=%s", vmwcrUUID, namespace)
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusOK))
				})

			})

			When("an error occurs while getting the WebConsoleRequest resource", func() {

				It("should return http.StatusInternalServerError (500)", func() {
					// Use a fake client with the vmopv1 scheme only to cause an
					// error when getting the v1alpha1 WebConsoleRequestList CR.
					scheme := runtime.NewScheme()
					Expect(vmopv1.AddToScheme(scheme)).To(Succeed())
					server.KubeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

					url := fmt.Sprintf("/?uuid=%s&namespace=%s", wcrUUID, namespace)
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusInternalServerError))
				})

			})

			When("UUID matches an existing WebConsoleRequest resource", func() {

				It("should return http.StatusOK (200)", func() {
					url := fmt.Sprintf("/?uuid=%s&namespace=test-namespace", wcrUUID)
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusOK))
				})

			})

			When("Namespace doesn't match any WebConsoleRequest or VirtualMachineWebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					nonExistentNamespace := "non-existent-namespace"
					url := fmt.Sprintf("/?uuid=%s&namespace=%s", wcrUUID, nonExistentNamespace)
					responseCode := fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusForbidden))

					url = fmt.Sprintf("/?uuid=%s&namespace=%s", vmwcrUUID, nonExistentNamespace)
					responseCode = fakeValidationRequest(url, server)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				})

			})

			When("UUID doesn't match any WebConsoleRequest or VirtualMachineWebConsoleRequest resource", func() {

				It("should return http.StatusForbidden (403)", func() {
					url := fmt.Sprintf("/?uuid=%s&namespace=%s", "non-existent-uuid", namespace)
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
