// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary_test

import (
	"os"
	"strings"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/library"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
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
			testConfig = builder.VCSimTestConfig{}
			testConfig.WithContentLibrary = true
		})

		JustBeforeEach(func() {
			ctx = suite.NewTestContextForVCSim(testConfig, initObjects...)
			clProvider = contentlibrary.NewProvider(ctx, ctx.RestClient)
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

			Context("VirtualMachineImageResourceForLibrary", func() {
				var itemID string
				JustBeforeEach(func() {
					items, err := clProvider.ListLibraryItems(ctx, ctx.ContentLibraryID)
					Expect(err).ShouldNot(HaveOccurred())
					Expect(items).NotTo(BeEmpty())
					itemID = items[0]
				})

				It("Get VMImage Resource from library", func() {
					image, err := clProvider.VirtualMachineImageResourceForLibrary(ctx, itemID, ctx.ContentLibraryID, nil)
					Expect(err).NotTo(HaveOccurred())

					Expect(image.Name).To(Equal(ctx.ContentLibraryImageName))
					Expect(image.Spec.Type).To(Equal("ovf"))
					Expect(image.Status.ImageName).To(Equal(ctx.ContentLibraryImageName))
					Expect(image.Status.Firmware).To(Equal("efi"))
				})

				It("Returns cached VirtualMachineImage from map", func() {
					image, err := clProvider.VirtualMachineImageResourceForLibrary(ctx, itemID, ctx.ContentLibraryID, nil)
					Expect(err).NotTo(HaveOccurred())

					Expect(image.Name).To(Equal(ctx.ContentLibraryImageName))
					image.Spec.Type = "dummy-type-to-test-cache"
					currentCLImages := map[string]vmopv1.VirtualMachineImage{
						image.Status.ImageName: *image,
					}

					cachedImage, err := clProvider.VirtualMachineImageResourceForLibrary(ctx, itemID, ctx.ContentLibraryID, currentCLImages)
					Expect(err).NotTo(HaveOccurred())
					Expect(cachedImage.Name).To(Equal(image.Name))
					Expect(cachedImage.Spec.Type).To(Equal(image.Spec.Type))
				})
			})
		})

		Context("when items are not present in library", func() {

		})

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
