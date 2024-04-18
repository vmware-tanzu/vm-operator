// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vapi/rest"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
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
}

func CreateVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	clClient contentlibrary.Provider,
	restClient *rest.Client,
	finder *find.Finder,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	if createArgs.UseContentLibrary {
		return deployFromContentLibrary(vmCtx, clClient, restClient, createArgs)
	}

	return cloneVMFromInventory(vmCtx, finder, createArgs)
}
