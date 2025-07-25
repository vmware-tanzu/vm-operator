// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// CreateArgs contains the arguments needed to create a VM.
type CreateArgs struct {
	UseContentLibrary bool
	ProviderItemID    string

	ConfigSpec                vimtypes.VirtualMachineConfigSpec
	StorageProvisioning       string
	DatacenterMoID            string
	FolderMoID                string
	ResourcePoolMoID          string
	HostMoID                  string
	StorageProfileID          string
	IsEncryptedStorageProfile bool
	DatastoreMoID             string // gce2e only: used only if StorageProfileID is unset
	Datastores                []DatastoreRef
	DiskPaths                 []string
	FilePaths                 []string
	ZoneName                  string
}

type DatastoreRef struct {
	Name        string
	MoRef       vimtypes.ManagedObjectReference
	URL         string
	DiskFormats []string

	// ForDisk is false if the recommendation is for the VM's home directory and
	// true if for a disk. DiskKey is only valid if ForDisk is true.
	ForDisk bool
	DiskKey int32
}

func CreateVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	restClient *rest.Client,
	vimClient *vim25.Client,
	finder *find.Finder,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	if strings.HasPrefix(createArgs.ProviderItemID, "vm-") {
		// This is a VM-backed image, and it can only be provisioned via fast
		// deploy.
		return fastDeploy(vmCtx, vimClient, createArgs)
	}

	if createArgs.UseContentLibrary {
		return deployFromContentLibrary(vmCtx, restClient, vimClient, createArgs)
	}
	return cloneVMFromInventory(vmCtx, finder, createArgs)
}
