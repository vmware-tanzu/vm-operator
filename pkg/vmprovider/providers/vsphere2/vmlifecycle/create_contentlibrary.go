// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/contentlibrary"
)

func deployOVF(
	vmCtx context.VirtualMachineContextA2,
	restClient *rest.Client,
	item *library.Item,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	deploymentSpec := vcenter.DeploymentSpec{
		Name:                vmCtx.VM.Name,
		StorageProfileID:    createArgs.StorageProfileID,
		StorageProvisioning: createArgs.StorageProvisioning,
		AcceptAllEULA:       true,
	}

	if deploymentSpec.StorageProfileID == "" {
		// Without a storage profile, fall back to the datastore.
		deploymentSpec.DefaultDatastoreID = createArgs.DatastoreMoID
	}

	if pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate && createArgs.ConfigSpec != nil {
		configSpecXML, err := util.MarshalConfigSpecToXML(createArgs.ConfigSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal ConfigSpec to XML: %w", err)
		}

		deploymentSpec.VmConfigSpec = &vcenter.VmConfigSpec{
			Provider: constants.ConfigSpecProviderXML,
			XML:      base64.StdEncoding.EncodeToString(configSpecXML),
		}
	}

	deploy := vcenter.Deploy{
		DeploymentSpec: deploymentSpec,
		Target: vcenter.Target{
			ResourcePoolID: createArgs.ResourcePoolMoID,
			FolderID:       createArgs.FolderMoID,
			HostID:         createArgs.HostMoID,
		},
	}

	vmCtx.Logger.Info("Deploying OVF Library Item", "itemID", item.ID, "itemName", item.Name, "deploy", deploy)

	return vcenter.NewManager(restClient).DeployLibraryItem(vmCtx, item.ID, deploy)
}

func deployVMTX(
	vmCtx context.VirtualMachineContextA2,
	restClient *rest.Client,
	item *library.Item,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	// Not yet supported. This item type needs to be deployed via DeployTemplateLibraryItem(),
	// which doesn't take a ConfigSpec so it is a heavy lift.
	// TODO: We should catch this earlier to avoid a bunch of wasted work.

	_ = vmCtx
	_ = restClient
	_ = createArgs

	return nil, fmt.Errorf("creating VM from VMTX content library type is not supported: %s", item.Name)
}

func deployFromContentLibrary(
	vmCtx context.VirtualMachineContextA2,
	clClient contentlibrary.Provider,
	restClient *rest.Client,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	// This call is needed to get the item type. We could avoid going to CL here, and
	// instead get the item type via the {Cluster}ContentLibrary CR for the image.
	item, err := clClient.GetLibraryItemID(vmCtx, createArgs.ProviderItemID)
	if err != nil {
		return nil, err
	}

	switch item.Type {
	case library.ItemTypeOVF:
		return deployOVF(vmCtx, restClient, item, createArgs)
	case library.ItemTypeVMTX:
		return deployVMTX(vmCtx, restClient, item, createArgs)
	default:
		return nil, errors.Errorf("item %s not a supported type: %s", item.Name, item.Type)
	}
}
