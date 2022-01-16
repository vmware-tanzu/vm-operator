// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package hostvalidation_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"

	"github.com/vmware-tanzu/vm-operator/pkg/hostvalidation"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func hostValidationTests() {

	Describe("Host validation server tests", func() {

		var (
			ctx        *builder.TestContextForVCSim
			testVM     *object.VirtualMachine
			testVMMoID string
		)

		BeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
			hostvalidation.K8sClient = ctx.Client

			list, err := ctx.Finder.VirtualMachineList(ctx.Context, "*")
			Expect(err).ToNot(HaveOccurred())
			Expect(list).ToNot(BeEmpty())
			testVM = list[0]
			testVMMoID = testVM.Reference().Value
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			testVM = nil
			testVMMoID = ""
		})

		Context("When making the requests with missing params", func() {
			It("should return http.StatusBadRequest (400)", func() {
				var responseCode int

				responseCode = makeHostValidationRequest("/")
				Expect(responseCode).To(Equal(http.StatusBadRequest))

				responseCode = makeHostValidationRequest("/?host=1.2.3.4")
				Expect(responseCode).To(Equal(http.StatusBadRequest))

				responseCode = makeHostValidationRequest("/?vm_moid=vm-123")
				Expect(responseCode).To(Equal(http.StatusBadRequest))
			})
		})

		Context("When making the requests with required params", func() {
			It("should return http.StatusOK (200) for a valid host", func() {
				// Collect the testVM's ESXi hostname and IPs.
				testCtx := ctx.Context
				var validHosts []string
				objHostSystem, err := testVM.HostSystem(testCtx)
				Expect(err).ToNot(HaveOccurred())
				esxiHostname, err := objHostSystem.ObjectName(testCtx)
				Expect(err).ToNot(HaveOccurred())
				validHosts = append(validHosts, esxiHostname)

				esxiHostIPs, err := objHostSystem.ManagementIPs(testCtx)
				Expect(err).ToNot(HaveOccurred())
				for _, ip := range esxiHostIPs {
					validHosts = append(validHosts, ip.String())
				}

				// Send out requests to the server with valid hosts.
				// We don't test against a VC IP here as the simulator doesn't have it setup.
				for _, host := range validHosts {
					url := fmt.Sprintf("/?vm_moid=%s&host=%s", testVMMoID, host)
					responseCode := makeHostValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusOK))
				}
			})

			It("should return http.StatusForbidden (403) for an invalid host", func() {
				// Send out requests to the server with invalid hosts.
				invalidHosts := []string{"1.2.3.4", "5.6.7.8"}
				for _, host := range invalidHosts {
					url := fmt.Sprintf("/?vm_moid=%s&host=%s", testVMMoID, host)
					responseCode := makeHostValidationRequest(url)
					Expect(responseCode).To(Equal(http.StatusForbidden))
				}
			})

			It("should return http.StatusInternalServerError (500) if cannot get VM", func() {
				// Send out request to the server without setting any client.
				url := fmt.Sprintf("/?vm_moid=%s&host=%s", "vm-fake", "1.2.3.4")
				responseCode := makeHostValidationRequest(url)
				Expect(responseCode).To(Equal(http.StatusInternalServerError))
			})

		})
	})
}

// makeHostValidationRequest is a helper function to make a HTTP request to the host validation server.
func makeHostValidationRequest(url string) int {
	responseRecorder := httptest.NewRecorder()
	handler := http.HandlerFunc(hostvalidation.HandleHostValidation)
	testRequest, _ := http.NewRequest("GET", url, nil)
	handler.ServeHTTP(responseRecorder, testRequest)
	response := responseRecorder.Result()
	_ = response.Body.Close()

	return response.StatusCode
}
