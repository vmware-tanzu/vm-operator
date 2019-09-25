/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package integration

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	vmoperator "github.com/vmware-tanzu/vm-operator"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/soap"

	"k8s.io/klog"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
	"k8s.io/client-go/kubernetes"
)

const DefaultNamespace = "default"
const SecretName = "vmop-test-integration-auth" // nolint:gosec
const ContentSourceName = "vmop-test-integration-cl"

type VSphereVmProviderTestConfig struct {
	VcCredsSecretName string
	*vsphere.VSphereVmProviderConfig
}

// Support for bootstrapping VM operator resource requirements in Kubernetes.
// Generate a fake vsphere provider config that is suitable for the integration test environment.
// Post the resultant config map to the API Master for consumption by the VM operator
func InstallVmOperatorConfig(clientSet *kubernetes.Clientset, vcAddress string, vcPort int) error {
	klog.Infof("Installing a bootstrap config map for use in integration tests.")
	return vsphere.InstallVSphereVmProviderConfig(clientSet, DefaultNamespace, NewIntegrationVmOperatorConfig(vcAddress, vcPort), SecretName)
}

func NewIntegrationVmOperatorConfig(vcAddress string, vcPort int) *vsphere.VSphereVmProviderConfig {
	return &vsphere.VSphereVmProviderConfig{
		VcPNID:        vcAddress,
		VcPort:        strconv.Itoa(vcPort),
		VcCreds:       NewIntegrationVmOperatorCredentials(),
		Datacenter:    "/DC0",
		ResourcePool:  "/DC0/host/DC0_C0/Resources",
		Folder:        "/DC0/vm",
		Datastore:     "/DC0/datastore/LocalDS_0",
		ContentSource: ContentSourceName,
	}
}

func NewIntegrationVmOperatorCredentials() *vsphere.VSphereVmProviderCredentials {
	// User and password can be anything for vcSim
	return &vsphere.VSphereVmProviderCredentials{
		Username: "Administrator@vsphere.local",
		Password: "Admin!23",
	}
}

func SetupEnv(vcSim *VcSimInstance) (*vsphere.VSphereVmProviderConfig, error) {
	address, port := vcSim.Start()
	config := NewIntegrationVmOperatorConfig(address, port)
	err := setupVcSimContent(vcSim, config)
	if err != nil {
		return nil, fmt.Errorf("failed to setup content library: %v", err)
	}
	provider, err := vsphere.NewVSphereVmProviderFromConfig(DefaultNamespace, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create vSphere provider: %v", err)
	}

	vmprovider.RegisterVmProvider(provider)

	return config, nil
}

func CleanupEnv(vcSim *VcSimInstance) {
	if vcSim != nil {
		vcSim.Stop()
	}
}

func setupVcSimContent(vcSim *VcSimInstance, config *vsphere.VSphereVmProviderConfig) (err error) {
	ctx := context.TODO()
	// The 'Rootpath'/images directory is created and populated with ova content for CL related integration tests
	// and cleaned up right after.
	imagesDir := "images/"
	ovf := "ttylinux-pc_i486-16.1.ovf"
	fmt.Printf("Setting up Content Library: %+v\n", *config)
	c, err := vcSim.NewClient(ctx)
	if err != nil {
		return
	}

	rClient := rest.NewClient(c.Client)
	userInfo := url.UserPassword(config.VcCreds.Username, config.VcCreds.Password)

	err = rClient.Login(ctx, userInfo)
	if err != nil {
		return
	}
	defer func() {
		err = rClient.Logout(ctx)
	}()

	return SetupContentLibraryForTest(ctx, ContentSourceName, c, config, rClient, imagesDir, ovf)
}

func SetupContentLibraryForTest(ctx context.Context, sourceName string, c *govmomi.Client,
	config *vsphere.VSphereVmProviderConfig, rClient *rest.Client, imagesDir string, ovf string) error {
	finder := find.NewFinder(c.Client, false)
	dc, err := finder.Datacenter(ctx, config.Datacenter)
	if err != nil {
		return err
	}
	finder.SetDatacenter(dc)
	ds, err := finder.Datastore(ctx, config.Datastore)
	if err != nil {
		return err
	}
	lib := library.Library{
		Name: ContentSourceName,
		Type: "LOCAL",
		Storage: []library.StorageBackings{
			{
				DatastoreID: ds.Reference().Value,
				Type:        "DATASTORE",
			},
		},
	}
	//Create Library in vcsim for integration tests
	libID, err := library.NewManager(rClient).CreateLibrary(ctx, lib)
	if err != nil {
		return err
	}
	item := library.Item{
		Name:      "test-item",
		Type:      "ovf",
		LibraryID: libID,
	}
	//Create Library item in vcsim for integration tests
	itemID, err := library.NewManager(rClient).CreateLibraryItem(ctx, item)

	if err != nil {
		return err
	}
	session, err := library.NewManager(rClient).CreateLibraryItemUpdateSession(ctx, library.Session{
		LibraryItemID: itemID,
	})

	if err != nil {
		return err
	}
	//Update Library item with library file "ovf"
	upload := func(path string) error {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return err
		}

		name := filepath.Base(path)

		size := fi.Size()
		info := library.UpdateFile{
			Name:       name,
			SourceType: "PUSH",
			Size:       size,
		}

		update, err := library.NewManager(rClient).AddLibraryItemFile(ctx, session, info)
		if err != nil {
			return err
		}

		p := soap.DefaultUpload
		p.Headers = map[string]string{
			"vmware-api-session-id": session,
		}
		p.ContentLength = size
		u, err := url.Parse(update.UploadEndpoint.URI)
		if err != nil {
			return err
		}

		return c.Upload(ctx, f, u, &p)
	}
	path := string(vmoperator.Rootpath + "/" + imagesDir + ovf)
	if err = upload(path); err != nil {
		return err
	}
	return library.NewManager(rClient).CompleteLibraryItemUpdateSession(ctx, session)
}
