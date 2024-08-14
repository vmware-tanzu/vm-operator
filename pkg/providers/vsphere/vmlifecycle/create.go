// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	ctxop "github.com/vmware-tanzu/vm-operator/pkg/context/operation"
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
	restClient *rest.Client,
	vimClient *vim25.Client,
	finder *find.Finder,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	ctxop.MarkCreate(vmCtx)

	if createArgs.UseContentLibrary {
		return deployFromContentLibrary(vmCtx, restClient, vimClient, createArgs)
	}

	return cloneVMFromInventory(vmCtx, finder, createArgs)
}
