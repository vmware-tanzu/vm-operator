// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func clusterTests() {
	Describe("ClusterMinCPUFreq", minFreq)
}

func minFreq() {
	// Hardcoded value in govmomi simulator/esx/host_system.go
	const expectedCPUFreq = 2294

	var (
		ctx        *builder.TestContextForVCSim
		testConfig builder.VCSimTestConfig
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{WithV1A2: true}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	Describe("ClusterMinCPUFreq", func() {
		It("returns min freq of hosts in cluster", func() {
			cpuFreq, err := vcenter.ClusterMinCPUFreq(ctx, ctx.GetFirstClusterFromFirstZone())
			Expect(err).ToNot(HaveOccurred())
			Expect(cpuFreq).Should(BeEquivalentTo(expectedCPUFreq))
		})
	})
}
