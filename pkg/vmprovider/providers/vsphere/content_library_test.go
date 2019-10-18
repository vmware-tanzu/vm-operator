// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// +build !integration

package vsphere_test

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/vmware/govmomi/simulator/vpx"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25"
	vmoperator "github.com/vmware-tanzu/vm-operator"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/mocks"

	"net/url"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vapi/rest"
	vapi "github.com/vmware/govmomi/vapi/simulator"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"github.com/vmware-tanzu/vm-operator/test/integration"
)

var (
	err error
	ctx context.Context

	sess        *vsphere.Session
	namespace   = "base-namespace"
	testOvfName = "photon-ova.ovf"

	restClient        *rest.Client
	config            *vsphere.VSphereVmProviderConfig
	fileUriToDownload string
	downloadSessionId string
)

func newVsphereConfig(vcAddress string, vcPort int) *vsphere.VSphereVmProviderConfig {
	return &vsphere.VSphereVmProviderConfig{
		VcPNID:        vcAddress,
		VcPort:        strconv.Itoa(vcPort),
		VcCreds:       vSphereCredentials(),
		Datacenter:    "/DC0",
		ResourcePool:  "/DC0/host/DC0_C0/Resources",
		Folder:        "/DC0/vm",
		Datastore:     "/DC0/datastore/LocalDS_0",
		ContentSource: "",
	}
}

func vSphereCredentials() *vsphere.VSphereVmProviderCredentials {
	return &vsphere.VSphereVmProviderCredentials{
		Username: "some-user",
		Password: "some-pass",
	}
}

func setUpContentLibrary(c *govmomi.Client, config *vsphere.VSphereVmProviderConfig) error {
	restClient = rest.NewClient(c.Client)
	if err != nil {
		return err
	}

	userInfo := url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)
	err = restClient.Login(ctx, userInfo)
	if err != nil {
		return err
	}

	return integration.SetupContentLibraryForTest(ctx, c, config, restClient, "test/resource/",
		"photon-ova.ovf")
}

func setupTest() {
	ctx = context.TODO()
	m := simulator.VPX()

	err := m.Create()
	Expect(err).To(BeNil())

	m.Service.TLS = new(tls.Config)
	server := m.Service.NewServer()
	host := server.URL.Hostname()

	port, err := strconv.Atoi(server.URL.Port())
	if err != nil {
		server.Close()
	}

	config = newVsphereConfig(host, port)

	path, handler := vapi.New(server.URL, vpx.Setting)
	m.Service.Handle(path, handler)

	client, err := govmomi.NewClient(ctx, server.URL, true)
	Expect(err).To(BeNil())

	err = setUpContentLibrary(client, config)
	Expect(err).To(BeNil())

	provider, err := vsphere.NewVSphereVmProviderFromConfig(namespace, config)
	Expect(err).ShouldNot(HaveOccurred())

	vmprovider.RegisterVmProvider(provider)

	sess, err = provider.GetSession(ctx, namespace)
	Expect(err).To(BeNil())
}

var _ = Describe("list files in content library", func() {

	var mockController *gomock.Controller
	var mockContentProvider *mocks.MockContentDownloadHandler

	BeforeEach(func() {
		mockController = gomock.NewController(GinkgoT())
		mockContentProvider = mocks.NewMockContentDownloadHandler(mockController)
	})

	AfterEach(func() {
		mockController.Finish()
	})

	Context("when items are present in library", func() {
		It("lists the ovf and downloads the ovf", func() {
			simulator.Test(func(ctx context.Context, client *vim25.Client) {
				var libID string
				var libItem *library.Item

				setupTest()

				libraries, err := library.NewManager(restClient).ListLibraries(ctx)
				Expect(err).To(BeNil())

				libID = libraries[0]
				item := library.Item{
					Name:      "test-item",
					Type:      "ovf",
					LibraryID: libID,
				}

				itemIDs, err := library.NewManager(restClient).FindLibraryItems(ctx,
					library.FindItem{LibraryID: libID, Name: item.Name})
				Expect(err).To(BeNil())
				Expect(itemIDs).Should(HaveLen(1))

				libItem, err = library.NewManager(restClient).GetLibraryItem(ctx, itemIDs[0])
				Expect(err).To(BeNil())

				testProvider := vsphere.ContentDownloadProvider{ApiWaitTimeSecs: 1}
				_ = sess.WithRestClient(ctx, func(c *rest.Client) error {
					downloadResponse, err := testProvider.GenerateDownloadUriForLibraryItem(ctx, c, libItem, sess)
					Expect(err).To(BeNil())
					Expect(downloadResponse).ShouldNot(BeEquivalentTo(vsphere.DownloadUriResponse{}))
					Expect(downloadResponse.FileUri).ShouldNot(BeEmpty())
					Expect(downloadResponse.DownloadSessionId).NotTo(BeNil())
					fileUriToDownload = downloadResponse.FileUri
					downloadSessionId = downloadResponse.DownloadSessionId
					return err
				})
			})
		})

	})

	Context("when invalid item id is passed", func() {
		It("returns an error creating a download session", func() {
			item := library.Item{
				Name:      "fakeItem",
				Type:      "ovf",
				LibraryID: "fakeID",
			}

			testContentStruct := vsphere.ContentDownloadProvider{ApiWaitTimeSecs: 1}
			err = sess.WithRestClient(ctx, func(c *rest.Client) error {
				_, err := testContentStruct.GenerateDownloadUriForLibraryItem(ctx, c, &item, sess)
				return err
			})
			Expect(err.Error()).Should(ContainSubstring("404 Not Found"))
		})
	})

	Context("when ovf file is present", func() {
		It("parses the ovf", func() {
			ovfPath := vmoperator.Rootpath + "/test/resource/" + testOvfName
			file, err := os.Open(ovfPath)
			Expect(err).To(BeNil())

			var readerStream io.ReadCloser = file
			props, err := vsphere.ParseOvfAndFetchProperties(readerStream)
			Expect(err).To(BeNil())
			Expect(props).NotTo(BeNil())
			Expect(len(props)).NotTo(BeZero())
			Expect(props).Should(HaveKeyWithValue("vmware-system.compatibilityoffering.offers.kube-apiserver."+
				"version", "1.14"))
		})
	})

	Context("when ovf file is absent", func() {
		It("returns a path error", func() {
			file, err := ioutil.TempFile("", "*.ovf")
			Expect(err).To(BeNil())

			var readerStream io.ReadCloser = file
			defer readerStream.Close()

			_, err = vsphere.ParseOvfAndFetchProperties(readerStream)
			Expect(err).Should(MatchError(errors.New("EOF")))
		})
	})

	Context("when session object is present", func() {
		It("should be possible to generate a content library session", func() {
			sessRef, err := vsphere.NewSessionAndConfigure(context.TODO(), config, nil)
			Expect(err).To(BeNil())

			clProvider := vsphere.NewContentLibraryProvider(sessRef)
			Expect(clProvider).NotTo(BeNil())
		})
	})

	Context("when url is invalid", func() {
		It("should return a Err", func() {
			ctx := context.TODO()
			sessRef, err := vsphere.NewSessionAndConfigure(ctx, config, nil)
			Expect(err).To(BeNil())
			err = sessRef.WithRestClient(ctx, func(c *rest.Client) error {
				_, err = vsphere.ReadFileFromUrl(ctx, c, sessRef, "test.com/link")
				return err
			})
			Expect(err).NotTo(BeNil())
		})
	})

	Context("when url is valid", func() {
		It("should download the file", func() {
			ctx := context.TODO()
			sessRef, err := vsphere.NewSessionAndConfigure(ctx, config, nil)
			Expect(err).To(BeNil())
			err = sessRef.WithRestClient(ctx, func(c *rest.Client) error {
				_, err = vsphere.ReadFileFromUrl(ctx, c, sessRef, fileUriToDownload)
				return err
			})
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
				GenerateDownloadUriForLibraryItem(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(vsphere.DownloadUriResponse{}, nil).
				Times(1)
			//s := &vsphere.Session{}
			clProvider := vsphere.NewContentLibraryProvider(sess)
			ovfProperties, err := clProvider.ParseAndRetrievePropsFromLibraryItem(context.TODO(), &item, mockContentProvider)
			Expect(err).Should(MatchError("error occurred downloading item fakeItem"))
			Expect(ovfProperties).To(BeEmpty())
		})
	})

	Context("when a work function is given as input", func() {
		It("should return the expected response", func() {
			doneChannel := make(chan vsphere.TimerTaskResponse)
			ticker := time.NewTicker(time.Duration(2) * time.Second)
			work := func(t *time.Time) (vsphere.TimerTaskResponse, error) {
				for i := 1; i < 10; i++ {
					fmt.Printf("performing fake task for iteration - %v \n", i)

				}
				return vsphere.TimerTaskResponse{TaskDone: true}, nil
			}
			go vsphere.RunTaskAtInterval(doneChannel, ticker, work)
			var finalResponse = <-doneChannel
			Expect(finalResponse).NotTo(BeEquivalentTo(vsphere.TimerTaskResponse{}))
			Expect(finalResponse.TaskDone).To(BeTrue())
		})
	})

	Context("when a work function is given as input", func() {
		It("should return the expected response", func() {
			doneChannel := make(chan vsphere.TimerTaskResponse)
			ticker := time.NewTicker(time.Duration(2) * time.Second)
			work := func(t *time.Time) (vsphere.TimerTaskResponse, error) {
				return vsphere.TimerTaskResponse{TaskDone: true}, errors.New("an error occurred when performing work")
			}
			go vsphere.RunTaskAtInterval(doneChannel, ticker, work)
			var finalResponse = <-doneChannel
			Expect(finalResponse).NotTo(BeEquivalentTo(vsphere.TimerTaskResponse{}))
			Expect(finalResponse.TaskDone).To(BeTrue())
			Expect(finalResponse.Err.Error()).To(BeEquivalentTo("an error occurred when performing work"))
		})
	})
})
