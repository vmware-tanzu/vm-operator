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

var _ = Describe("CPU Frequency", func() {
	var (
		testConfig builder.VCSimTestConfig
		ctx        *builder.TestContextForVCSim
		vmProvider providers.VirtualMachineProviderInterface
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{}
	})

	JustBeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(testConfig)
		vmProvider = vsphere.NewVSphereVMProviderFromClient(ctx, ctx.Client, ctx.Recorder)
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		vmProvider = nil
	})

	Context("ComputeCPUMinFrequency", func() {
		It("returns success", func() {
			Expect(vmProvider.ComputeCPUMinFrequency(ctx)).To(Succeed())
		})
	})
})
