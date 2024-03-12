// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

var _ = Describe(
	"ResolveBackingPostPlacement",
	Label(testlabels.NSXT, testlabels.VCSim),
	func() {

		var (
			testConfig builder.VCSimTestConfig
			ctx        *builder.TestContextForVCSim

			results *network.NetworkInterfaceResults
			fixedUp bool
			err     error
		)

		BeforeEach(func() {
			testConfig = builder.VCSimTestConfig{}
		})

		JustBeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(testConfig)

			fixedUp, err = network.ResolveBackingPostPlacement(
				ctx,
				ctx.VCClient.Client,
				ctx.GetFirstClusterFromFirstZone().Reference(),
				results)
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
		})

		Context("NSX-T returns success", func() {

			BeforeEach(func() {
				testConfig.WithNetworkEnv = builder.NetworkEnvNSXT

				results = &network.NetworkInterfaceResults{
					Results: []network.NetworkInterfaceResult{
						{
							NetworkID: builder.NsxTLogicalSwitchUUID,
							Backing:   nil,
						},
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(fixedUp).To(BeTrue())

				Expect(results.Results).To(HaveLen(1))
				By("should populate the backing", func() {
					backing := results.Results[0].Backing
					Expect(backing).ToNot(BeNil())
					Expect(backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
				})
			})
		})

		Context("VPC returns success", func() {

			BeforeEach(func() {
				testConfig.WithNetworkEnv = builder.NetworkEnvVPC

				results = &network.NetworkInterfaceResults{
					Results: []network.NetworkInterfaceResult{
						{
							NetworkID: builder.VPCLogicalSwitchUUID,
							Backing:   nil,
						},
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(fixedUp).To(BeTrue())

				Expect(results.Results).To(HaveLen(1))
				By("should populate the backing", func() {
					backing := results.Results[0].Backing
					Expect(backing).ToNot(BeNil())
					Expect(backing.Reference()).To(Equal(ctx.NetworkRef.Reference()))
				})
			})
		})
	})
