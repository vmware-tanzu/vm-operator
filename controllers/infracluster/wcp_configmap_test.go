// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infracluster_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/controllers/infracluster"
)

func unitTestsWcpConfig() {
	Describe("ParseWcpClusterConfig", func() {
		var (
			err    error
			config *infracluster.WcpClusterConfig
			data   map[string]string
		)

		JustBeforeEach(func() {
			config, err = infracluster.ParseWcpClusterConfig(data)
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
					infracluster.WcpClusterConfigFileName: "not-valid-yaml",
				}
			})

			It("returns an error", func() {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unmarshal errors"))
			})
		})

		Context("valid data", func() {
			pnid := "foo-pnid"
			port := "foo-port"

			BeforeEach(func() {
				cm, err := infracluster.NewWcpClusterConfigMap(infracluster.WcpClusterConfig{
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
	})
}
