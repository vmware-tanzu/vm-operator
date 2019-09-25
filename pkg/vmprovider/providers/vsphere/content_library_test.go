// +build !integration

package vsphere_test

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/vmware/govmomi/simulator/vpx"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vim25"
	vmoperator "github.com/vmware-tanzu/vm-operator"

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

	sess              *vsphere.Session
	namespace         = "base-namespace"
	ContentSourceName = "cl_unit_test"
	testOvfName       = "photon-ova.ovf"

	restClient        *rest.Client
	config            *vsphere.VSphereVmProviderConfig
	fileUriToDownload string
)

type ClientMock struct {
}

func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	return &http.Response{}, nil
}

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
	Expect(err).To(BeNil())
	err = integration.SetupContentLibraryForTest(ctx, ContentSourceName, c, config, restClient, "test/resource/",
		"photon-ova.ovf")
	return err
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
	path, handler := vapi.New(server.URL, vpx.Setting)
	m.Service.Handle(path, handler)
	if err != nil {
		server.Close()
	}
	client, err := govmomi.NewClient(ctx, server.URL, true)
	Expect(err).To(BeNil())
	config = newVsphereConfig(host, port)
	err = setUpContentLibrary(client, config)
	provider, err := vsphere.NewVSphereVmProviderFromConfig(namespace, config)
	Expect(err).ShouldNot(HaveOccurred())
	vmprovider.RegisterVmProvider(provider)
	sess, err = provider.GetSession(ctx, namespace)
}

var _ = Describe("list files in content library", func() {

	Context("when items are present in library", func() {
		It("lists the ovf and downloads the ovf", func() {
			simulator.Test(func(ctx context.Context, client *vim25.Client) {
				setupTest()
				var libID string
				var libItem *library.Item
				clProvider := vsphere.NewContentLibraryProvider(sess)
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
				fileUri, sessionid, err := clProvider.GenerateDownloadUriForLibraryItem(ctx, *libItem, restClient)
				Expect(err).To(BeNil())
				Expect(fileUri).ShouldNot(BeEmpty())
				Expect(sessionid).NotTo(BeNil())
				fileUriToDownload = fileUri
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
			clProvider := vsphere.NewContentLibraryProvider(sess)
			_, _, err := clProvider.GenerateDownloadUriForLibraryItem(ctx, item, restClient)
			Expect(err.Error()).Should(ContainSubstring("404 Not Found"))
		})
	})

	Context("when ovf file is present", func() {
		It("parses the ovf", func() {
			ovfPath := string(vmoperator.Rootpath + "/test/resource/" + testOvfName)
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
		It("should be possbile to generate a content library session", func() {
			sessRef, err := vsphere.NewSessionAndConfigure(context.TODO(), config, nil)
			Expect(err).To(BeNil())
			clProvider := vsphere.NewContentLibraryProvider(sessRef)
			Expect(clProvider).NotTo(BeNil())
		})
	})

	Context("when url is invalid", func() {
		It("should return a err", func() {
			_, err := vsphere.ReadFileFromUrl(context.TODO(), restClient, "test.com/link")
			Expect(err).NotTo(BeNil())
		})
	})

	Context("when url is valid", func() {
		It("should download the file", func() {
			_, err := vsphere.ReadFileFromUrl(context.TODO(), restClient, fileUriToDownload)
			Expect(err).To(BeNil())
		})
	})

})
