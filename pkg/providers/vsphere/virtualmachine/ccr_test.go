// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func ccrTests() {

	var (
		ctx  *builder.TestContextForVCSim
		vcVM *object.VirtualMachine
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})

		var err error
		vcVM, err = ctx.Finder.VirtualMachine(ctx, "DC0_C0_RP0_VM0")
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("Returns VM ClusterComputeResource", func() {
		ccr, err := virtualmachine.GetVMClusterComputeResource(ctx, vcVM)
		Expect(err).ToNot(HaveOccurred())
		Expect(ccr).ToNot(BeNil())
		Expect(ccr.Reference()).To(Equal(ctx.GetFirstClusterFromFirstZone().Reference()))
	})
}
