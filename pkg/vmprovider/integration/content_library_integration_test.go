// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vapi/library"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var _ = Describe("Content Library", func() {

	Context("when items are present in library", func() {
		It("lists the ovf and downloads the ovf", func() {
			ctx := context.Background()
			restClient := session.Client.RestClient()

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

			testProvider := vsphere.NewContentLibraryProviderWithWaitSec(restClient, 1)

			ovfEnvelope, err := testProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, libItem)
			Expect(err).ToNot(HaveOccurred())
			Expect(ovfEnvelope).ToNot(BeNil())
		})
	})

	Context("when invalid item id is passed", func() {
		It("returns an error creating a download session", func() {
			ctx := context.Background()
			restClient := session.Client.RestClient()

			item := &library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}

			testProvider := vsphere.NewContentLibraryProviderWithWaitSec(restClient, 1)

			_, err := testProvider.RetrieveOvfEnvelopeFromLibraryItem(ctx, item)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("404 Not Found"))
		})
	})
})
