// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"fmt"
	"strings"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/topology"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GetVirtualMachine gets the VM from VC, either by the MoID or by the inventory path.
// Folder is optional so this can be called from within a Session where we have already
// the folder. This will go away as we continue to hoist code out of the Session.
func GetVirtualMachine(
	vmCtx context.VirtualMachineContext,
	client ctrlclient.Client,
	finder *find.Finder,
	folder *object.Folder) (*object.VirtualMachine, error) {

	if uniqueID := vmCtx.VM.Status.UniqueID; uniqueID != "" {
		// Fast path: lookup via the MoID.
		if vm, err := findVMByMoID(vmCtx, finder, uniqueID); err == nil {
			return vm, nil
		}

		vmCtx.Logger.V(5).Info("Failed to find existing VM by MoID, falling back to inventory path",
			"moID", uniqueID)
	}

	if folder == nil {
		// Called from outside a Session: lookup the namespace's Folder.
		folderMoID, err := topology.GetNamespaceFolderMoID(vmCtx, client, vmCtx.VM.Namespace)
		if err != nil {
			return nil, err
		}

		folder, err = GetFolderByMoID(vmCtx, finder, folderMoID)
		if err != nil {
			return nil, err
		}
	}

	// When the VM has a ResourcePolicy, the VM is placed in a child folder under the namespace's folder.
	if policyName := vmCtx.VM.Spec.ResourcePolicyName; policyName != "" {
		resourcePolicy := &v1alpha1.VirtualMachineSetResourcePolicy{}

		key := ctrlclient.ObjectKey{Name: policyName, Namespace: vmCtx.VM.Namespace}
		if err := client.Get(vmCtx, key, resourcePolicy); err != nil {
			vmCtx.Logger.Error(err, "Failed to get VirtualMachineSetResourcePolicy", "name", key)
			// Don't return a wrapped error because that can cause us to later incorrectly assume
			// the VM doesn't exist. There is just a fundamental issue with potentially needing the
			// ResourcePolicy to find a VM that we need to fully sort out.
			return nil, fmt.Errorf("failed to get VirtualMachineSetResourcePolicy: %v", err)
		}

		childFolder, err := finder.Folder(vmCtx, folder.InventoryPath+"/"+resourcePolicy.Spec.Folder.Name)
		if err != nil {
			vmCtx.Logger.Error(err, "Failed to find ResourcePolicy child Folder",
				"parentPath", folder.InventoryPath,
				"folderName", resourcePolicy.Spec.Folder.Name, "resourcePolicy", key)
			return nil, err
		}

		folder = childFolder
	}

	vmPath := folder.InventoryPath + "/" + vmCtx.VM.Name
	vm, err := finder.VirtualMachine(vmCtx, vmPath)
	if err != nil {
		vmCtx.Logger.Error(err, "Failed find VM by path", "path", vmPath)
		return nil, transformVMError(vmCtx.VM.NamespacedName(), err)
	}

	vmCtx.Logger.V(4).Info("Found VM via path", "path", vmPath, "moID", vm.Reference().Value)
	return vm, nil
}

func findVMByMoID(
	vmCtx context.VirtualMachineContext,
	finder *find.Finder,
	moID string) (*object.VirtualMachine, error) {

	ref, err := finder.ObjectReference(vmCtx, types.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
	if err != nil {
		return nil, err
	}

	vm := ref.(*object.VirtualMachine)
	vmCtx.Logger.V(4).Info("Found VM via MoID", "name", vm.Name(),
		"path", vm.InventoryPath, "moID", moID)
	return vm, nil
}

// Transform Govmomi error to Kubernetes error
// TODO: Fill out with VIM fault types.
func transformError(resourceType string, resource string, err error) error {
	switch err.(type) {
	case *find.NotFoundError, *find.DefaultNotFoundError:
		return k8serrors.NewNotFound(schema.GroupResource{Group: "vmoperator.vmware.com", Resource: strings.ToLower(resourceType)}, resource)
	case *find.MultipleFoundError, *find.DefaultMultipleFoundError:
		// Transform?
		return err
	default:
		return err
	}
}

func transformVMError(resource string, err error) error {
	return transformError("VirtualMachine", resource, err)
}
