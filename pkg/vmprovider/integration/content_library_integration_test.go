// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"io/ioutil"
	"os"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/library"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var _ = Describe("Content Library", func() {
	var (
		libMgr    *library.Manager
		libraryID string
		clClient  contentlibrary.Provider
	)

	BeforeEach(func() {
		libMgr = library.NewManager(vcClient.RestClient())

		libraries, err := libMgr.ListLibraries(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(libraries).ToNot(BeEmpty())
		libraryID = libraries[0]

		clClient = vcClient.ContentLibClient()
	})

	Context("when items are present in library", func() {

		It("lists items", func() {
			items, err := clClient.GetLibraryItems(ctx, libraryID)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).ToNot(BeEmpty())
		})

		It("gets and downloads the ovf", func() {
			libItem, err := clClient.GetLibraryItem(ctx, libraryID, integration.IntegrationContentLibraryItemName)
			Expect(err).ToNot(HaveOccurred())
			Expect(libItem).ToNot(BeNil())

			ovfEnvelope, err := vcClient.ContentLibClient().RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
			Expect(err).ToNot(HaveOccurred())
			Expect(ovfEnvelope).ToNot(BeNil())
		})
	})

	Context("when invalid item id is passed", func() {
		It("returns an error creating a download session", func() {
			item := &library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}

			ovf, err := clClient.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("404 Not Found"))
			Expect(ovf).To(BeNil())
		})
	})

	Context("called with an OVF that is invalid", func() {
		var (
			ovf         *os.File
			libItem     *library.Item
			libItemName string
		)

		BeforeEach(func() {
			// Upload an invalid OVF (0B in size) to the integration test CL
			ovf, err = ioutil.TempFile("", "fake-*.ovf")
			Expect(err).NotTo(HaveOccurred())
			ovfInfo, err := ovf.Stat()
			Expect(err).NotTo(HaveOccurred())

			libItemName = strings.Split(ovfInfo.Name(), ".ovf")[0]
			libItem = &library.Item{
				Name:      libItemName,
				Type:      "ovf",
				LibraryID: libraryID,
			}

			err = clClient.CreateLibraryItem(ctx, *libItem, ovf.Name())
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Clean up the invalid library item from the CL
			Expect(clClient.DeleteLibraryItem(ctx, libItem)).To(Succeed())
			Expect(os.Remove(ovf.Name())).To(Succeed())
		})

		It("does not return error", func() {
			var err error

			libItem, err = clClient.GetLibraryItem(ctx, libraryID, libItemName)
			Expect(err).ToNot(HaveOccurred())
			Expect(libItem).ToNot(BeNil())

			ovfEnvelope, err := clClient.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
			Expect(err).ToNot(HaveOccurred())
			Expect(ovfEnvelope).To(BeNil())
		})
	})
})
