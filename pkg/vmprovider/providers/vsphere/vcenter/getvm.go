// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"
)

// GetVirtualMachine gets the VM from VC, either by the MoID, UUID, or the inventory path.
func GetVirtualMachine(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	datacenter *object.Datacenter,
	finder *find.Finder) (*object.VirtualMachine, error) {

	if uniqueID := vmCtx.VM.Status.UniqueID; uniqueID != "" {
		if vm, err := findVMByMoID(vmCtx, finder, uniqueID); err == nil {
			return vm, nil
		}
	}

	// For when we start to use the k8s VM.UID for the VC VM's InstanceUUID or UUID (aka BiosUUID):
	/*
		if instanceUUID := vmCtx.VM.UID; instanceUUID != "" {
			if vm, err := findVMByUUID(vmCtx, vimClient, datacenter, string(instanceUUID), true); err == nil {
				return vm, nil
			}
		}
	*/

	return findVMByInventory(vmCtx, k8sClient, vimClient, finder)
}

func findVMByMoID(
	vmCtx context.VirtualMachineContext,
	finder *find.Finder,
	moID string) (*object.VirtualMachine, error) {

	ref, err := finder.ObjectReference(vmCtx, types.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
	if err != nil {
		return nil, err
	}

	vm, ok := ref.(*object.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("found VM reference was not a VM but a %T", ref)
	}

	vmCtx.Logger.V(4).Info("Found VM via MoID", "path", vm.InventoryPath, "moID", moID)
	return vm, nil
}

//nolint:deadcode
func findVMByUUID(
	vmCtx context.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter,
	uuid string,
	isInstanceUUID bool) (*object.VirtualMachine, error) {

	ref, err := object.NewSearchIndex(vimClient).FindByUuid(vmCtx, datacenter, uuid, true, &isInstanceUUID)
	if err != nil {
		return nil, fmt.Errorf("error finding object by UUID %q: %w", uuid, err)
	} else if ref == nil {
		return nil, fmt.Errorf("no VM found for UUID %q (instanceUUID: %t)", uuid, isInstanceUUID)
	}

	vm, ok := ref.(*object.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("found VM reference was not a VirtualMachine but a %T", ref)
	}

	vmCtx.Logger.V(4).Info("Found VM via UUID", "uuid", uuid, "isInstanceUUID", isInstanceUUID)
	return vm, nil
}

func findVMByInventory(
	vmCtx context.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	finder *find.Finder) (*object.VirtualMachine, error) {

	// Note that we'll usually only get here to find the VM via its inventory path when we're first
	// creating the VM. To determine the path, we need the NS Folder MoID and the VM's ResourcePolicy,
	// if set, and we'll fetch these again as a part of createVirtualMachine(). For now, just re-fetch
	// but we could pass the Folder MoID and ResourcePolicy to save a bit of duplicated work.

	folderMoID, err := topology.GetNamespaceFolderMoID(vmCtx, k8sClient, vmCtx.VM.Namespace)
	if err != nil {
		return nil, err
	}

	// While we strictly only need the Folder's ManagedObjectReference below, use the Finder
	// here to check if it actually exists.
	folder, err := GetFolderByMoID(vmCtx, finder, folderMoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace Folder: %w", err)
	}

	// When the VM has a ResourcePolicy, the VM is placed in a child folder under the namespace's folder.
	if policyName := vmCtx.VM.Spec.ResourcePolicyName; policyName != "" {
		resourcePolicy := &v1alpha1.VirtualMachineSetResourcePolicy{}

		key := ctrlclient.ObjectKey{Name: policyName, Namespace: vmCtx.VM.Namespace}
		if err := k8sClient.Get(vmCtx, key, resourcePolicy); err != nil {
			// Note that if VM does not exist and we're about to create it, the ResourcePolicy is
			// also a part of the VirtualMachinePrereqReadyCondition.
			return nil, fmt.Errorf("failed to get VirtualMachineSetResourcePolicy: %w", err)
		}

		if folderName := resourcePolicy.Spec.Folder.Name; folderName != "" {
			childFolder, err := GetChildFolder(vmCtx, folder, resourcePolicy.Spec.Folder.Name)
			if err != nil {
				vmCtx.Logger.Error(err, "Failed to get VirtualMachineSetResourcePolicy child Folder",
					"parentPath", folder.InventoryPath, "folderName", folderName, "policyName", policyName)
				return nil, err
			}

			folder = childFolder
		}
	}

	ref, err := object.NewSearchIndex(vimClient).FindChild(vmCtx, folder.Reference(), vmCtx.VM.Name)
	if err != nil {
		return nil, err
	} else if ref == nil {
		// VM does not exist.
		return nil, nil
	}

	vm, ok := ref.(*object.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("found VM reference was not a VM but a %T", ref)
	}

	vmCtx.Logger.V(4).Info("Found VM via inventory",
		"parentFolderMoID", folder.Reference().Value, "moID", vm.Reference().Value)
	return vm, nil
}
