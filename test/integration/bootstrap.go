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

	"github.com/pkg/errors"
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

func SetupEnv(vcSim *VcSimInstance) error {
	address, port := vcSim.Start()
	config := NewIntegrationVmOperatorConfig(address, port)
	provider, err := vsphere.NewVSphereVmProviderFromConfig(DefaultNamespace, config)
	if err != nil {
		return fmt.Errorf("failed to create vSphere provider: %v", err)
	}
	vmprovider.RegisterVmProvider(provider)
	return nil
}

func CleanupEnv(vcSim *VcSimInstance) {
	vcSim.Stop()
}

func SetupVcSimContent(ctx context.Context, s *vsphere.Session, config *vsphere.VSphereVmProviderConfig) error {
	// The 'Rootpath'/images directory is created and populated with ova content for CL related integration tests
	// and cleaned up right after.
	imagesDir := "images/"
	ovf := "ttylinux-pc_i486-16.1.ovf"

	return s.WithRestClient(ctx, func(c *rest.Client) error {
		ds, err := s.Finder.Datastore(ctx, config.Datastore)
		if err != nil {
			return err
		}

		lib := library.Library{
			Name: config.ContentSource,
			Type: "LOCAL",
			Storage: []library.StorageBackings{
				{
					DatastoreID: ds.Reference().Value,
					Type:        "DATASTORE",
				},
			},
		}
		//Create Library in vcsim for integration tests
		libID, err := library.NewManager(c).CreateLibrary(ctx, lib)
		if err != nil {
			return errors.Wrapf(err, "failed to create content library %v for tests", config.ContentSource)
		}

		item := library.Item{
			Name:      "test-item",
			Type:      "ovf",
			LibraryID: libID,
		}
		//Create Library item in vcsim for integration tests
		itemID, err := library.NewManager(c).CreateLibraryItem(ctx, item)
		if err != nil {
			return errors.Wrapf(err, "failed to create content library item for tests ")
		}

		session, err := library.NewManager(c).CreateLibraryItemUpdateSession(ctx, library.Session{
			LibraryItemID: itemID,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create update session for library item %v", item.Name)
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

			update, err := library.NewManager(c).AddLibraryItemFile(ctx, session, info)
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

		return library.NewManager(c).CompleteLibraryItemUpdateSession(ctx, session)
	})
}
