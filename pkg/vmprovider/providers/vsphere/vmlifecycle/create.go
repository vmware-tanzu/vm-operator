// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/contentlibrary"
)

// CreateArgs contains the arguments needed to create a VM.
type CreateArgs struct {
	UseContentLibrary bool
	ProviderItemID    string

	ConfigSpec          types.VirtualMachineConfigSpec
	StorageProvisioning string
	FolderMoID          string
	ResourcePoolMoID    string
	HostMoID            string
	StorageProfileID    string
	DatastoreMoID       string // gce2e only: used only if StorageProfileID is unset
}

func CreateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	clClient contentlibrary.Provider,
	restClient *rest.Client,
	finder *find.Finder,
	createArgs *CreateArgs) (*types.ManagedObjectReference, error) {

	if createArgs.UseContentLibrary {
		return deployFromContentLibrary(vmCtx, clClient, restClient, createArgs)
	}

	return cloneVMFromInventory(vmCtx, finder, createArgs)
}
