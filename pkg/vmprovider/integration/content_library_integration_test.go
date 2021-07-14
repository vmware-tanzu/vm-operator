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

	Context("when items are present in library", func() {
		It("lists the ovf and downloads the ovf", func() {
			restClient := c.RestClient()

			mgr := library.NewManager(restClient)
			libraries, err := mgr.ListLibraries(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(libraries).ToNot(BeEmpty())

			libID := libraries[0]
			item := library.Item{
				Name:      integration.IntegrationContentLibraryItemName,
				Type:      "ovf",
				LibraryID: libID,
			}

			itemIDs, err := mgr.FindLibraryItems(ctx, library.FindItem{LibraryID: libID, Name: item.Name})
			Expect(err).ToNot(HaveOccurred())
			Expect(itemIDs).Should(HaveLen(1))

			libItem, err := mgr.GetLibraryItem(ctx, itemIDs[0])
			Expect(err).ToNot(HaveOccurred())

			testProvider := contentlibrary.NewProviderWithWaitSec(restClient, 1)

			ovfEnvelope, err := testProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
			Expect(err).ToNot(HaveOccurred())
			Expect(ovfEnvelope).ToNot(BeNil())
		})
	})

	Context("when invalid item id is passed", func() {
		It("returns an error creating a download session", func() {
			restClient := vmClient.RestClient()

			item := &library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}

			testProvider := contentlibrary.NewProviderWithWaitSec(restClient, 1)

			_, err := testProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("404 Not Found"))
		})
	})
})

var _ = Describe("RetrieveOvfEAnvelopeFromLibraryItems", func() {
	When("called with an OVF that is invalid", func() {
		var (
			ovf         *os.File
			mgr         *library.Manager
			libItem     *library.Item
			libItemName string
		)

		BeforeEach(func() {
			mgr = library.NewManager(vmClient.RestClient())

			// Upload an invalid OVF (0B in size) to the integration test CL
			ovf, err = ioutil.TempFile(os.TempDir(), "fake-*.ovf")
			Expect(err).NotTo(HaveOccurred())
			ovfInfo, err := ovf.Stat()
			Expect(err).NotTo(HaveOccurred())

			libItemName = strings.Split(ovfInfo.Name(), ".ovf")[0]
			err = integration.CreateLibraryItem(ctx, vmClient, libItemName, "ovf", integration.GetContentSourceID(), ovf.Name())
			Expect(err).NotTo(HaveOccurred())

		})

		AfterEach(func() {
			// Clean up the invalid library item from the CL
			Expect(vmClient.ContentLibClient().DeleteLibraryItem(ctx, libItem)).To(Succeed())
			Expect(os.Remove(ovf.Name())).To(Succeed())
		})

		It("does not return error", func() {
			itemIDs, err := mgr.FindLibraryItems(ctx, library.FindItem{LibraryID: integration.GetContentSourceID(), Name: libItemName})
			Expect(err).ToNot(HaveOccurred())
			Expect(itemIDs).Should(HaveLen(1))

			libItem, err = mgr.GetLibraryItem(ctx, itemIDs[0])
			Expect(err).ToNot(HaveOccurred())

			testProvider := contentlibrary.NewProviderWithWaitSec(vmClient.RestClient(), 1)
			ovfEnvelope, err := testProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
			Expect(err).ToNot(HaveOccurred())
			Expect(ovfEnvelope).To(BeNil())
		})
	})
})
