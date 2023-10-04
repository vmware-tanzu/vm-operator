// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/library"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/test/builder"
)

func clTests() {
	Describe("Content Library", func() {

		var (
			initObjects []client.Object
			ctx         *builder.TestContextForVCSim
			testConfig  builder.VCSimTestConfig

			clProvider contentlibrary.Provider
		)

		BeforeEach(func() {
			testConfig = builder.VCSimTestConfig{WithV1A2: true}
			testConfig.WithContentLibrary = true
		})

		JustBeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
			clProvider = contentlibrary.NewProvider(ctx.RestClient)
		})

		AfterEach(func() {
			ctx.AfterEach()
			ctx = nil
			initObjects = nil
		})

		Context("when items are present in library", func() {

			It("List items id in library", func() {
				items, err := clProvider.GetLibraryItems(ctx, ctx.ContentLibraryID)
				Expect(err).ToNot(HaveOccurred())
				Expect(items).ToNot(BeEmpty())
			})

			It("Does not return error when library does not exist", func() {
				items, err := clProvider.GetLibraryItems(ctx, "dummy-cl")
				Expect(err).ToNot(HaveOccurred())
				Expect(items).To(BeEmpty())
			})

			It("Does not return error when item name is invalid when notFoundReturnErr is set to false", func() {
				item, err := clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, "dummy-name", true)
				Expect(err).To(HaveOccurred())
				Expect(item).To(BeNil())

				item, err = clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, "dummy-name", false)
				Expect(err).NotTo(HaveOccurred())
				Expect(item).To(BeNil())
			})

			It("Gets items and returns OVF", func() {
				item, err := clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, ctx.ContentLibraryImageName, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(item).ToNot(BeNil())

				ovfEnvelope, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
				Expect(err).ToNot(HaveOccurred())
				Expect(ovfEnvelope).ToNot(BeNil())
			})
		})

		Context("when items are not present in library", func() {

			Context("when invalid item id is passed", func() {

				It("returns an error creating a download session", func() {
					libItem := &library.Item{
						Name:      "fakeItem",
						Type:      "ovf",
						LibraryID: "fakeID",
					}

					ovf, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("404 Not Found"))
					Expect(ovf).To(BeNil())
				})
			})
		})

		Context("called with an OVF that is invalid because of network connectivity issue", func() {
			var ovfPath string

			AfterEach(func() {
				if ovfPath != "" {
					Expect(os.Remove(ovfPath)).To(Succeed())
				}
			})

			It("returns error", func() {
				ovf, err := os.CreateTemp("", "fake-*.ovf")
				Expect(err).NotTo(HaveOccurred())
				ovfPath = ovf.Name()

				ovfInfo, err := ovf.Stat()
				Expect(err).NotTo(HaveOccurred())

				libItemName := strings.Split(ovfInfo.Name(), ".ovf")[0]
				libItem := library.Item{
					Name:      libItemName,
					Type:      "ovf",
					LibraryID: ctx.ContentLibraryID,
				}

				err = clProvider.CreateLibraryItem(ctx, libItem, ovfPath)
				Expect(err).NotTo(HaveOccurred())

				libItem2, err := clProvider.GetLibraryItem(ctx, ctx.ContentLibraryID, libItemName, true)
				Expect(err).ToNot(HaveOccurred())
				Expect(libItem2).ToNot(BeNil())
				Expect(libItem2.Name).To(Equal(libItem.Name))

				ovfEnvelope, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem2)
				Expect(err).To(HaveOccurred())
				Expect(ovfEnvelope).To(BeNil())
			})
		})
	})
}
