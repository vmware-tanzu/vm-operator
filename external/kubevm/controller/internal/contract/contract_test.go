// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package contract_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/vmware-tanzu/vm-operator/external/kubevm/controller/internal/contract"
)

func TestContract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contract Suite")
}

var _ = Describe("ReadStatus", func() {
	It("reads every fixed contract path", func() {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"status": map[string]any{
					"addresses": []any{
						map[string]any{
							"interface": "eth0",
							"type":      "InternalIP",
							"address":   "10.0.0.5",
						},
					},
					"powerState": "PoweredOn",
					"providerID": "vm-123",
					"providerMetadata": map[string]any{
						"uniqueID": "vm-abc",
					},
					"conditions": []any{
						map[string]any{
							"type":    "InfrastructureReady",
							"status":  "True",
							"reason":  "Reported",
							"message": "",
						},
						map[string]any{
							"type":    "UpToDate",
							"status":  "False",
							"reason":  "UnsupportedByProvider",
							"message": "cannot apply",
						},
					},
				},
			},
		}

		status, err := contract.ReadStatus(obj)
		Expect(err).ToNot(HaveOccurred())

		Expect(status.Addresses).To(ConsistOf(contract.Address{
			Interface: "eth0",
			Type:      "InternalIP",
			Address:   "10.0.0.5",
		}))
		Expect(status.PowerState).To(Equal("PoweredOn"))
		Expect(status.ProviderID).To(Equal("vm-123"))
		Expect(status.ProviderMetadata).To(HaveKeyWithValue("uniqueID", "vm-abc"))
		Expect(status.Ready).ToNot(BeNil())
		Expect(*status.Ready).To(Equal(corev1.ConditionTrue))
		Expect(status.UpToDate).ToNot(BeNil())
		Expect(*status.UpToDate).To(Equal(corev1.ConditionFalse))
		Expect(status.UpToDateReason).To(Equal("UnsupportedByProvider"))
		Expect(status.UpToDateMessage).To(Equal("cannot apply"))
	})

	It("leaves every field at its zero value when status is absent", func() {
		obj := &unstructured.Unstructured{Object: map[string]any{}}

		status, err := contract.ReadStatus(obj)
		Expect(err).ToNot(HaveOccurred())

		Expect(status.Addresses).To(BeEmpty())
		Expect(status.PowerState).To(BeEmpty())
		Expect(status.ProviderID).To(BeEmpty())
		Expect(status.Ready).To(BeNil())
		Expect(status.UpToDate).To(BeNil())
	})
})
