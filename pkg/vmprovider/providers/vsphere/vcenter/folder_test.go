// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func folderTests() {
	Describe("GetFolderByMoID", getFolderByMoID)
}

func getFolderByMoID() {

	var (
		ctx    *builder.TestContextForVCSim
		nsInfo builder.WorkloadNamespaceInfo
	)

	BeforeEach(func() {
		ctx = suite.NewTestContextForVCSim(builder.VCSimTestConfig{})
		nsInfo = ctx.CreateWorkloadNamespace()
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
	})

	It("returns success", func() {
		moID := nsInfo.Folder.Reference().Value

		folder, err := vcenter.GetFolderByMoID(ctx, ctx.Finder, moID)
		Expect(err).ToNot(HaveOccurred())
		Expect(folder).ToNot(BeNil())
		Expect(folder.Name()).To(Equal(nsInfo.Namespace))
	})

	It("returns error when moID does not exist", func() {
		folder, err := vcenter.GetFolderByMoID(ctx, ctx.Finder, "bogus")
		Expect(err).To(HaveOccurred())
		Expect(folder).To(BeNil())
	})
}
