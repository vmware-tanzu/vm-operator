// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func hostTests() {
	Describe("GetESXHostFQDN", hostFQDN)
}

func hostFQDN() {
	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig

		hostMoID string
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{WithInstanceStorage: true}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)

		hosts, err := ctx.Finder.HostSystemList(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).ToNot(BeEmpty())
		hostMoID = hosts[0].Reference().Value
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Describe("GetESXHostFQDN", func() {
		When("host does not have DNSConfig", func() {
			BeforeEach(func() {
				testConfig.WithInstanceStorage = false
			})

			It("returns expected error", func() {
				_, err := vcenter.GetESXHostFQDN(ctx, ctx.VCClient.Client, hostMoID)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(" does not have DNSConfig"))
			})
		})

		It("returns expected host name for host", func() {
			hostName, err := vcenter.GetESXHostFQDN(ctx, ctx.VCClient.Client, hostMoID)
			Expect(err).ToNot(HaveOccurred())
			Expect(hostName).Should(Equal(fmt.Sprintf("%s.vmop.vmware.com", hostMoID)))
		})
	})
}
