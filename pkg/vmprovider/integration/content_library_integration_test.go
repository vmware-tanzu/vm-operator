// +build integration

// Copyright (c) 2019-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi/vapi/library"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/mocks"
	"github.com/vmware-tanzu/vm-operator/test/integration"
	"github.com/vmware-tanzu/vm-operator/test/testutil"
)

var _ = Describe("content library", func() {

	var mockController *gomock.Controller
	var mockContentProvider *mocks.MockContentDownloadHandler
	var fileUriToDownload string

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		mockContentProvider = mocks.NewMockContentDownloadHandler(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	Context("when items are present in library", func() {

		It("lists the ovf and downloads the ovf", func() {
			ctx := context.Background()

			restClient := session.Client.RestClient()
			mgr := library.NewManager(restClient)
			libraries, err := mgr.ListLibraries(ctx)
			Expect(err).To(BeNil())

			libID := libraries[0]
			item := library.Item{
				Name:      integration.IntegrationContentLibraryItemName,
				Type:      "ovf",
				LibraryID: libID,
			}

			itemIDs, err := mgr.FindLibraryItems(ctx, library.FindItem{LibraryID: libID, Name: item.Name})
			Expect(err).To(BeNil())
			Expect(itemIDs).Should(HaveLen(1))

			libItem, err := mgr.GetLibraryItem(ctx, itemIDs[0])
			Expect(err).To(BeNil())

			testProvider := vsphere.ContentDownloadProvider{ApiWaitTimeSecs: 1}

			downloadResponse, err := testProvider.GenerateDownloadUriForLibraryItem(ctx, restClient, libItem)
			Expect(err).To(BeNil())
			Expect(downloadResponse).ShouldNot(BeEquivalentTo(vsphere.DownloadUriResponse{}))
			Expect(downloadResponse.FileUri).ShouldNot(BeEmpty())
			Expect(downloadResponse.DownloadSessionId).NotTo(BeNil())
			fileUriToDownload = downloadResponse.FileUri
		})
	})

	Context("when invalid item id is passed", func() {

		It("returns an error creating a download session", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}

			ctx := context.Background()
			testContentStruct := vsphere.ContentDownloadProvider{ApiWaitTimeSecs: 1}

			restClient := session.Client.RestClient()
			_, err := testContentStruct.GenerateDownloadUriForLibraryItem(ctx, restClient, &item)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).Should(ContainSubstring("404 Not Found"))
		})
	})

	Context("when ovf file is present", func() {

		It("parses the ovf", func() {
			rootDir, err := testutil.GetRootDir()
			Expect(err).ToNot(HaveOccurred())
			ovfPath := rootDir + "/test/resource/photon-ova.ovf"
			file, err := os.Open(ovfPath)
			Expect(err).ToNot(HaveOccurred())
			defer file.Close()

			ovfEnvelope, err := vsphere.ParseOvf(file)
			Expect(err).To(BeNil())
			Expect(ovfEnvelope).NotTo(BeNil())

			props := vsphere.GetVmwareSystemPropertiesFromOvf(ovfEnvelope)
			Expect(len(props)).NotTo(BeZero())
			Expect(props).Should(HaveKeyWithValue(
				"vmware-system.compatibilityoffering.offers.kube-apiserver.version", "1.14"))
		})
	})

	Context("when ovf file is absent", func() {

		It("returns a path error", func() {
			file, err := ioutil.TempFile("", "*.ovf")
			Expect(err).To(BeNil())
			defer file.Close()

			_, err = vsphere.ParseOvf(file)
			Expect(err).Should(MatchError(errors.New("EOF")))
		})
	})

	Context("when session object is present", func() {

		It("should be possible to generate a content library session", func() {
			clProvider := vsphere.NewContentLibraryProvider(session)
			Expect(clProvider).NotTo(BeNil())
		})
	})

	Context("when url is invalid", func() {

		It("should return an error", func() {
			ctx := context.TODO()

			restClient := session.Client.RestClient()
			file, err := vsphere.ReadFileFromUrl(ctx, restClient, "test.com/link")
			if file != nil {
				_ = file.Close()
			}
			Expect(err).NotTo(BeNil())
		})
	})

	Context("when url is valid", func() {

		It("should download the file", func() {
			ctx := context.TODO()

			restClient := session.Client.RestClient()
			file, err := vsphere.ReadFileFromUrl(ctx, restClient, fileUriToDownload)
			if file != nil {
				_ = file.Close()
			}

			Expect(err).To(BeNil())
		})
	})

	Context("when download of file in content library api fails", func() {

		It("should return an error", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}
			mockContentProvider.EXPECT().
				GenerateDownloadUriForLibraryItem(gomock.Any(), gomock.Any(), gomock.Any()).
				Return(vsphere.DownloadUriResponse{}, nil).
				Times(1)
			clProvider := vsphere.NewContentLibraryProvider(session)
			_, err := clProvider.RetrieveOvfEnvelopeFromLibraryItem(context.TODO(), &item, mockContentProvider)
			Expect(err).Should(MatchError("error occurred downloading item fakeItem"))
		})
	})

	Context("when a work function is given as input", func() {

		It("should return the expected response", func() {
			apiDelay := time.Duration(2) * time.Second
			work := func(ctx context.Context) (vsphere.TimerTaskResponse, error) {
				return vsphere.TimerTaskResponse{TaskDone: true}, nil
			}
			err := vsphere.RunTaskAtInterval(context.TODO(), apiDelay, work)
			Expect(err).To(BeNil())
		})
	})

	Context("when a given work function results in error", func() {

		It("should return the error as expected", func() {
			apiDelay := time.Duration(2) * time.Second
			work := func(ctx context.Context) (vsphere.TimerTaskResponse, error) {
				return vsphere.TimerTaskResponse{TaskDone: true}, errors.New("an error occurred when performing work")
			}
			err := vsphere.RunTaskAtInterval(context.TODO(), apiDelay, work)
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(BeEquivalentTo("an error occurred when performing work"))
		})
	})
})
