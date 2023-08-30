// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func ccrTests() {

	var (
		ctx  *builder.TestContextForVCSim
		vcVM *object.VirtualMachine
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{WithV1A2: true})

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
		Expect(ccr.Reference()).To(Equal(ctx.GetSingleClusterCompute().Reference()))
	})
}
