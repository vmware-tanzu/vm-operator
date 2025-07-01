// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configmap_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/configmap"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

func unitTests() {
	Describe("WCP Config", Label(testlabels.API), unitTestsWcpConfig)
}

func unitTestsWcpConfig() {
	Describe("ParseWcpClusterConfig", func() {
		var (
			err    error
			config *configmap.WcpClusterConfig
			data   map[string]string
		)

		JustBeforeEach(func() {
			config, err = configmap.ParseWcpClusterConfig(data)
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
					configmap.WcpClusterConfigFileName: "not-valid-yaml",
				}
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("error unmarshaling JSON"))
			})
		})

		Context("valid data", func() {
			pnid := "foo-pnid"
			port := "foo-port"

			BeforeEach(func() {
				cm, err := configmap.NewWcpClusterConfigMap(configmap.WcpClusterConfig{
					VcPNID: pnid,
					VcPort: port,
				})
				Expect(err).ToNot(HaveOccurred())

				data = cm.Data
			})

			It("returns expected config", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.VcPNID).To(Equal(pnid))
				Expect(config.VcPort).To(Equal(port))
			})
		})

		Context("valid data directly", func() {
			pnid := "foo-pnid"
			port := "foo-port"

			BeforeEach(func() {
				data = map[string]string{
					"wcp-cluster-config.yaml": fmt.Sprintf("\"vc_pnid\": %s\n\"vc_port\": %s", pnid, port),
				}
			})

			It("returns expected config", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.VcPNID).To(Equal(pnid))
				Expect(config.VcPort).To(Equal(port))
			})
		})
	})

}
