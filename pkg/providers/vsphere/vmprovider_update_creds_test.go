// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe("UpdateVcCreds", func() {
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

	When("Invalid Credentials", func() {

		It("returns error", func() {
			data := map[string][]byte{}
			Expect(vmProvider.UpdateVcCreds(ctx, data)).To(MatchError("vCenter username and password are missing"))
		})
	})

	When("New Credentials", func() {

		It("VC Client is logged out", func() {
			vcClient, err := vmProvider.VSphereClient(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vcClient).NotTo(BeNil())
			session, err := vcClient.RestClient().Session(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).ToNot(BeNil())

			data := map[string][]byte{
				"username": []byte("newUser"),
				"password": []byte("newPassword"),
			}
			Expect(vmProvider.UpdateVcCreds(ctx, data)).To(Succeed())
			By("Client is logged out", func() {
				session, err := vcClient.RestClient().Session(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(session).To(BeNil())
			})
		})
	})

	When("Same Credentials", func() {

		It("VC Client is not logged out", func() {
			vcClient, err := vmProvider.VSphereClient(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(vcClient).NotTo(BeNil())
			session, err := vcClient.RestClient().Session(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(session).ToNot(BeNil())

			data := map[string][]byte{
				"username": []byte(ctx.VCClientConfig.Username),
				"password": []byte(ctx.VCClientConfig.Password),
			}
			Expect(vmProvider.UpdateVcCreds(ctx, data)).To(Succeed())
			By("VC Client is the same", func() {
				vcClient2, err := vmProvider.VSphereClient(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(vcClient2).To(BeIdenticalTo(vcClient))

				session, err := vcClient.RestClient().Session(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(session).ToNot(BeNil())
			})
		})
	})
})
