// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capability_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/capability"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

func unitTests() {
	Describe("WCP Capabilities Config", Label(testlabels.V1Alpha3), unitTestsWcpCapabilitiesConfig)
}

func unitTestsWcpCapabilitiesConfig() {
	Describe("ParseWcpClusterCapabilitiesConfig", func() {
		var (
			err                      error
			data                     map[string]string
			isTKGMultipleCLSupported bool
		)

		JustBeforeEach(func() {
			isTKGMultipleCLSupported, err = capability.IsTKGMultipleCLSupported(data)
		})

		AfterEach(func() {
			data = nil
		})

		Context("empty data", func() {
			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("required key"))
			})
		})

		Context("invalid data", func() {
			BeforeEach(func() {
				data = map[string]string{
					capability.TKGMultipleCLCapabilityKey: "not-valid",
				}
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid syntax"))
			})
		})

		Context("valid data with false", func() {
			BeforeEach(func() {
				data = map[string]string{
					capability.TKGMultipleCLCapabilityKey: "false",
				}
			})

			It("returns expected config", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(isTKGMultipleCLSupported).To(BeFalse())
			})
		})

		Context("valid data with true", func() {
			BeforeEach(func() {
				data = map[string]string{
					capability.TKGMultipleCLCapabilityKey: "true",
				}
			})

			It("returns expected config", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(isTKGMultipleCLSupported).To(BeTrue())
			})
		})
	})

}
