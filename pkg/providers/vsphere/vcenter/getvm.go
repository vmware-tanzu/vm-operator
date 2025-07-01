// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vcenter

import (
	"errors"
	"fmt"

	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

var errVMNotFound = errors.New("vm not found")

// GetVirtualMachine gets the VM from VC, either by the Instance UUID, BIOS UUID, or MoID.
func GetVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter) (*object.VirtualMachine, error) {

	// Find by Instance UUID.
	if id := vmCtx.VM.UID; id != "" {
		if vm, err := findVMByUUID(vmCtx, vimClient, datacenter, string(id), true); err == nil {
			return vm, nil
		} else if !errors.Is(err, errVMNotFound) {
			return nil, err
		}
	}

	// Find by BIOS UUID.
	if id := vmCtx.VM.Spec.BiosUUID; id != "" {
		if vm, err := findVMByUUID(vmCtx, vimClient, datacenter, id, false); err == nil {
			return vm, nil
		} else if !errors.Is(err, errVMNotFound) {
			return nil, err
		}
	}

	// Find by MoRef.
	if id := vmCtx.VM.Status.UniqueID; id != "" {
		if vm, err := findVMByMoID(vmCtx, vimClient, id); err == nil {
			return vm, nil
		} else if !errors.Is(err, errVMNotFound) {
			return nil, err
		}
	}

	return nil, nil
}

func findVMByMoID(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	moID string) (*object.VirtualMachine, error) {

	moRef := vimtypes.ManagedObjectReference{
		Type:  "VirtualMachine",
		Value: moID,
	}

	vm := mo.VirtualMachine{}
	if err := property.DefaultCollector(vimClient).RetrieveOne(vmCtx, moRef, []string{"name"}, &vm); err != nil {
		var f *vimtypes.ManagedObjectNotFound
		if _, ok := fault.As(err, &f); ok {
			return nil, errVMNotFound
		}
		return nil, fmt.Errorf("error retreiving VM via MoID: %w", err)
	}

	vmCtx.Logger.V(4).Info("Found VM via MoID", "moID", moID)
	return object.NewVirtualMachine(vimClient, moRef), nil
}

func findVMByUUID(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter,
	uuid string,
	isInstanceUUID bool) (*object.VirtualMachine, error) {

	ref, err := object.NewSearchIndex(vimClient).FindByUuid(vmCtx, datacenter, uuid, true, &isInstanceUUID)
	if err != nil {
		return nil, fmt.Errorf("error finding VM by UUID %q: %w", uuid, err)
	} else if ref == nil {
		return nil, errVMNotFound
	}

	vm, ok := ref.(*object.VirtualMachine)
	if !ok {
		return nil, fmt.Errorf("found VM reference was not a VirtualMachine but a %T", ref)
	}

	vmCtx.Logger.V(4).Info("Found VM via UUID", "uuid", uuid, "isInstanceUUID", isInstanceUUID)
	return vm, nil
}
