// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
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

	ConfigSpec          vimtypes.VirtualMachineConfigSpec
	StorageProvisioning string
	FolderMoID          string
	ResourcePoolMoID    string
	HostMoID            string
	StorageProfileID    string
	DatastoreMoID       string // gce2e only: used only if StorageProfileID is unset
	Datastores          []DatastoreRef
	ZoneName            string
}

type DatastoreRef struct {
	Name                             string
	MoRef                            vimtypes.ManagedObjectReference
	URL                              string
	TopLevelDirectoryCreateSupported bool

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
	datacenter *object.Datacenter,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	if createArgs.UseContentLibrary {
		return deployFromContentLibrary(
			vmCtx,
			k8sClient,
			restClient,
			vimClient,
			datacenter,
			createArgs)
	}

	return cloneVMFromInventory(vmCtx, finder, createArgs)
}
