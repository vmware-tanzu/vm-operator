// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("UpdateVcPNID", func() {
	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
	})

	When("the PNID and port are unchanged", func() {

		It("keeps the VC client and does not signal a replacement", func() {
			providerConfig, err := config.GetProviderConfig(ctx, ctx.Client)
			Expect(err).NotTo(HaveOccurred())

			vcClient, err := vmProvider.VSphereClient(ctx)
			Expect(err).NotTo(HaveOccurred())
			replaced := vmProvider.VSphereClientReplaced()

			Expect(vmProvider.UpdateVcPNID(ctx, providerConfig.VcPNID, providerConfig.VcPort)).To(Succeed())
			Expect(replaced).ToNot(BeClosed())

			vcClient2, err := vmProvider.VSphereClient(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vcClient2).To(BeIdenticalTo(vcClient))
		})
	})

	When("the PNID changes", func() {

		It("logs out the old VC client and signals a replacement", func() {
			providerConfig, err := config.GetProviderConfig(ctx, ctx.Client)
			Expect(err).NotTo(HaveOccurred())

			vcClient, err := vmProvider.VSphereClient(ctx)
			Expect(err).NotTo(HaveOccurred())
			replaced := vmProvider.VSphereClientReplaced()

			Expect(vmProvider.UpdateVcPNID(ctx, "some-pnid", providerConfig.VcPort)).To(Succeed())
			Expect(replaced).To(BeClosed())

			session, err := vcClient.RestClient().Session(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).To(BeNil())
		})
	})
})
