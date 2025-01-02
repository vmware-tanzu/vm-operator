// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vcenter"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func folderTests() {
	Describe("GetFolderByMoID", getFolderByMoID)
	Describe("CreateDeleteExistsFolder", createDeleteExistsFolder)
}

func getFolderByMoID() {

	var (
		ctx        *builder.TestContextForVCSim
		nsInfo     builder.WorkloadNamespaceInfo
		testConfig builder.VCSimTestConfig
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			WithWorkloadIsolation: true,
		}
		ctx = suite.NewTestContextForVCSim(testConfig)
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

func createDeleteExistsFolder() {

	var (
		ctx        *builder.TestContextForVCSim
		nsInfo     builder.WorkloadNamespaceInfo
		testConfig builder.VCSimTestConfig

		parentFolderMoID string
	)

	BeforeEach(func() {
		testConfig = builder.VCSimTestConfig{
			WithWorkloadIsolation: true,
		}

		ctx = suite.NewTestContextForVCSim(testConfig)
		nsInfo = ctx.CreateWorkloadNamespace()
		parentFolderMoID = nsInfo.Folder.Reference().Value
	})

	AfterEach(func() {
		ctx.AfterEach()
		ctx = nil
		parentFolderMoID = ""
	})

	Context("CreateFolder", func() {
		It("creates child Folder", func() {
			childMoID, err := vcenter.CreateFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())
			Expect(childMoID).ToNot(BeEmpty())

			By("NoOp when child Folder already exists", func() {
				moID, err := vcenter.CreateFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
				Expect(err).ToNot(HaveOccurred())
				Expect(moID).To(Equal(childMoID))
			})

			By("child Folder is found by MoID", func() {
				folder, err := vcenter.GetFolderByMoID(ctx, ctx.Finder, childMoID)
				Expect(err).ToNot(HaveOccurred())
				Expect(folder.Reference().Value).To(Equal(childMoID))
			})
		})

		It("returns error when parent Folder MoID does not exist", func() {
			childMoID, err := vcenter.CreateFolder(ctx, ctx.VCClient.Client, "bogus", "myFolder")
			Expect(err).To(HaveOccurred())
			Expect(childMoID).To(BeEmpty())
		})
	})

	Context("GetChildFolder", func() {
		It("returns success when child Folder exists", func() {
			childFolderMoID, err := vcenter.CreateFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())

			parentFolder := object.NewFolder(ctx.VCClient.Client, vimtypes.ManagedObjectReference{
				Type:  "Folder",
				Value: parentFolderMoID,
			})

			childFolder, err := vcenter.GetChildFolder(ctx, parentFolder, "myFolder")
			Expect(err).ToNot(HaveOccurred())
			Expect(childFolder).ToNot(BeNil())
			Expect(childFolder.Reference().Value).To(Equal(childFolderMoID))
		})

		It("returns error when child Folder does not exists", func() {
			parentFolder := object.NewFolder(ctx.VCClient.Client, vimtypes.ManagedObjectReference{
				Type:  "Folder",
				Value: parentFolderMoID,
			})

			childFolder, err := vcenter.GetChildFolder(ctx, parentFolder, "myFolder")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found under parent Folder"))
			Expect(childFolder).To(BeNil())
		})
	})

	Context("DoesChildFolderExist", func() {
		It("returns true when child Folder exists", func() {
			_, err := vcenter.CreateFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())

			exists, err := vcenter.DoesChildFolderExist(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeTrue())
		})

		It("returns false when child Folder does not exist", func() {
			exists, err := vcenter.DoesChildFolderExist(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())
			Expect(exists).To(BeFalse())
		})

		It("returns error when parent Folder MoID does not exist", func() {
			exists, err := vcenter.DoesChildFolderExist(ctx, ctx.VCClient.Client, "bogus", "myFolder")
			Expect(err).To(HaveOccurred())
			Expect(exists).To(BeFalse())
		})
	})

	Context("DeleteFolder", func() {
		It("deletes child Folder", func() {
			childMoID, err := vcenter.CreateFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())

			err = vcenter.DeleteChildFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
			Expect(err).ToNot(HaveOccurred())

			By("child Folder is not found by MoID", func() {
				_, err := vcenter.GetFolderByMoID(ctx, ctx.Finder, childMoID)
				Expect(err).To(HaveOccurred())
			})

			By("NoOp when child does not exist", func() {
				err := vcenter.DeleteChildFolder(ctx, ctx.VCClient.Client, parentFolderMoID, "myFolder")
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
}
