// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func hostTests() {
	Describe("GetESXHostFQDN", hostFQDN)
	Describe("GetHostMaintenanceState", hostMaintenanceState)
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

func hostMaintenanceState() {
	var (
		ctx       *builder.TestContextForVCSim
		hostMoRef vimtypes.ManagedObjectReference
	)

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		hosts, err := ctx.Finder.HostSystemList(ctx, "*")
		Expect(err).ToNot(HaveOccurred())
		Expect(hosts).ToNot(BeEmpty())
		hostMoRef = hosts[0].Reference()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	When("host is not in maintenance mode", func() {
		It("returns InMaintenanceMode=false", func() {
			state, err := vcenter.GetHostMaintenanceState(ctx, ctx.VCClient.Client, hostMoRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.InMaintenanceMode).To(BeFalse())
			Expect(state.TransitioningMaintenanceMode).To(BeFalse())
		})
	})

	When("host is in maintenance mode", func() {
		JustBeforeEach(func() {
			task, err := object.NewHostSystem(ctx.VCClient.Client, hostMoRef).
				EnterMaintenanceMode(ctx, 0, false, nil)
			Expect(err).ToNot(HaveOccurred())
			Expect(task.Wait(ctx)).To(Succeed())
		})

		It("returns InMaintenanceMode=true", func() {
			state, err := vcenter.GetHostMaintenanceState(ctx, ctx.VCClient.Client, hostMoRef)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.InMaintenanceMode).To(BeTrue())
		})
	})
}
