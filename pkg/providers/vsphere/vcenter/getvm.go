// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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

// GetVirtualMachine gets the VM from VC, either by the MoID or the Instance UUID.
func GetVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter) (*object.VirtualMachine, error) {

	moID := vmCtx.VM.Status.UniqueID
	if moID != "" {
		if vm, err := findVMByMoID(vmCtx, vimClient, moID); err == nil {
			return vm, nil
		} else if !errors.Is(err, errVMNotFound) {
			return nil, err
		}
	}

	instanceUUID := vmCtx.VM.Spec.InstanceUUID
	if instanceUUID != "" {
		if vm, err := findVMByInstancedUUID(vmCtx, vimClient, datacenter, instanceUUID); err == nil {
			return vm, nil
		} else if !errors.Is(err, errVMNotFound) {
			return nil, err
		}
	}

	if uid := string(vmCtx.VM.UID); uid != "" && uid != instanceUUID {
		// We used to set the VM's InstanceUUID to that of the k8s VM UID. Fallback
		// to that ID to still find VMs there were created with that.
		if vm, err := findVMByInstancedUUID(vmCtx, vimClient, datacenter, uid); err == nil {
			return vm, nil
		} else if !errors.Is(err, errVMNotFound) {
			return nil, err
		}
	}

	if moID == "" && instanceUUID == "" {
		// This shouldn't happen, but we cannot determine if this VM exists or not so
		// must return an error.
		return nil, fmt.Errorf("neither MoID or InstanceUUID set on VM")
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
		return nil, fmt.Errorf("error retreiving VM via MoID %q: %w", moID, err)
	}

	vmCtx.Logger.V(4).Info("Found VM via MoID", "moID", moID)
	return object.NewVirtualMachine(vimClient, moRef), nil
}

func findVMByInstancedUUID(
	vmCtx pkgctx.VirtualMachineContext,
	vimClient *vim25.Client,
	datacenter *object.Datacenter,
	uuid string) (*object.VirtualMachine, error) {

	refs, err := object.NewSearchIndex(vimClient).FindAllByUuid(vmCtx, datacenter, uuid, true, vimtypes.NewBool(true))
	if err != nil {
		return nil, fmt.Errorf("error finding VM by instance UUID %q: %w", uuid, err)
	}

	switch len(refs) {
	case 0:
		return nil, errVMNotFound
	case 1:
		vm, ok := refs[0].(*object.VirtualMachine)
		if !ok {
			return nil, fmt.Errorf("found VM reference was not a VirtualMachine but a %T", refs[0])
		}
		vmCtx.Logger.V(4).Info("Found VM via instance UUID", "uuid", uuid)
		return vm, nil
	default:
		return nil, fmt.Errorf("found multiple VMs for instance UUID %q (%d)", uuid, len(refs))
	}
}
