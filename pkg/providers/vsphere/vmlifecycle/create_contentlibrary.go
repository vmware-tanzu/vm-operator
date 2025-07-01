// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"encoding/base64"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	"github.com/vmware/govmomi/vapi/vcenter"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

var _ = deployOVF

func deployOVF(
	vmCtx pkgctx.VirtualMachineContext,
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

	configSpecXML, err := util.MarshalConfigSpecToXML(createArgs.ConfigSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ConfigSpec to XML: %w", err)
	}

	deploymentSpec.VmConfigSpec = &vcenter.VmConfigSpec{
		Provider: constants.ConfigSpecProviderXML,
		XML:      base64.StdEncoding.EncodeToString(configSpecXML),
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

	return vcenter.NewManager(restClient).DeployLibraryItem(
		util.WithVAPIActivationID(vmCtx, restClient, vmCtx.VM.Spec.InstanceUUID),
		item.ID,
		deploy)
}

func createVM(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	createConfigSpec := createArgs.ConfigSpec
	createConfigSpec.GuestId = vmCtx.VM.Spec.GuestID

	vmFolder := object.NewFolder(vimClient, vimtypes.ManagedObjectReference{Type: "Folder", Value: createArgs.FolderMoID})
	resourcePool := object.NewResourcePool(vimClient, vimtypes.ManagedObjectReference{Type: "ResourcePool", Value: createArgs.ResourcePoolMoID})

	vmCtx.Logger.Info("Creating VM with params", "vmFolder", vmFolder.Reference(), "resourcePool", resourcePool.Reference(), "createConfigSpec", createConfigSpec)
	task, err := vmFolder.CreateVM(vmCtx, createConfigSpec, resourcePool, nil)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed to create VM")
		return nil, err
	}
	taskInfo, err := task.WaitForResultEx(vmCtx)
	if err != nil {
		vmCtx.Logger.Error(err, "Task failed to create VM")
		return nil, err
	}

	vmCtx.Logger.Info("Successfully Created the VM")
	vmRef := taskInfo.Result.(vimtypes.ManagedObjectReference)
	return &vmRef, nil
}

func deployVMTX(
	vmCtx pkgctx.VirtualMachineContext,
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
	vmCtx pkgctx.VirtualMachineContext,
	restClient *rest.Client,
	vimClient *vim25.Client,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	// This call is needed to get the item type. We could avoid going to CL here, and
	// instead get the item type via the {Cluster}ContentLibrary CR for the image.
	contentLibraryProvider := contentlibrary.NewProvider(vmCtx, restClient)
	item, err := contentLibraryProvider.GetLibraryItemID(vmCtx, createArgs.ProviderItemID)
	if err != nil {
		return nil, err
	}

	switch item.Type {
	case library.ItemTypeOVF:
		if pkgcfg.FromContext(vmCtx).Features.FastDeploy {
			return fastDeploy(vmCtx, vimClient, createArgs)
		}
		return deployOVF(vmCtx, restClient, item, createArgs)
	case library.ItemTypeVMTX:
		return deployVMTX(vmCtx, restClient, item, createArgs)
	case library.ItemTypeISO:
		return createVM(vmCtx, vimClient, createArgs)
	default:
		return nil, fmt.Errorf("item %s not a supported type: %s", item.Name, item.Type)
	}
}
