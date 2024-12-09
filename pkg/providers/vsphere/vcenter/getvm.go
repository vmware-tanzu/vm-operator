// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// GetVirtualMachine gets the VM from VC, either by the Instance UUID, BIOS UUID, or MoID.
func GetVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter,
	finder *find.Finder) (*object.VirtualMachine, error) {

	// Find by Instance UUID.
	if id := vmCtx.VM.UID; id != "" {
		if vm, err := findVMByUUID(vmCtx, vimClient, datacenter, string(id), true); err == nil {
			return vm, nil
		}
	}

	// Find by BIOS UUID.
	if id := vmCtx.VM.Spec.BiosUUID; id != "" {
		if vm, err := findVMByUUID(vmCtx, vimClient, datacenter, id, false); err == nil {
			return vm, nil
		}
	}

	// Find by MoRef.
	if id := vmCtx.VM.Status.UniqueID; id != "" {
		if vm, err := findVMByMoID(vmCtx, finder, id); err == nil {
			return vm, nil
		}
	}

	return nil, nil
}

func findVMByMoID(
	vmCtx pkgctx.VirtualMachineContext,
	finder *find.Finder,
	moID string) (*object.VirtualMachine, error) {

	ref, err := finder.ObjectReference(vmCtx, vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: moID})
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

func findVMByUUID(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter,
	uuid string,
	isInstanceUUID bool) (*object.VirtualMachine, error) {

	ref, err := object.NewSearchIndex(vimClient).FindByUuid(vmCtx, datacenter, uuid, true, &isInstanceUUID)
	if err != nil {
		return nil, fmt.Errorf("error finding object by UUID %q: %w", uuid, err)
	} else if ref == nil {
		return nil, fmt.Errorf("no VM found for UUID %q (instanceUUID: %v)", uuid, isInstanceUUID)
	}

	vm, ok := ref.(*object.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("found VM reference was not a VirtualMachine but a %T", ref)
	}

	vmCtx.Logger.V(4).Info("Found VM via UUID", "uuid", uuid, "isInstanceUUID", isInstanceUUID)
	return vm, nil
}
