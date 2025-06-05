// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// SnapshotArgs contains the options for createSnapshot.
type SnapshotArgs struct {
	VMCtx      pkgctx.VirtualMachineContext
	VcVM       *object.VirtualMachine
	VMSnapshot vmopv1.VirtualMachineSnapshot
}

func SnapshotVirtualMachine(args SnapshotArgs) (*types.ManagedObjectReference, error) {
	obj := args.VMSnapshot
	vm := args.VcVM
	// Find snapshot by name
	snapMoRef, _ := vm.FindSnapshot(args.VMCtx, obj.Name)
	if snapMoRef != nil {
		// TODO: Handle revert to snapshot. Need a way to compare currentSnapshot's moID
		// 	with spec.currentSnap
		//
		// Update vm.status with currentSnapshot
		updateVMStatusCurrentSnapshot(args.VMCtx, obj)
		// Return early, snapshot found
		return snapMoRef, nil
	}

	// If no snapshot was found, create it
	args.VMCtx.Logger.Info("Creating Snapshot of VirtualMachine", "snapshot name", obj.Name)
	snapMoRef, err := CreateSnapshot(args)
	if err != nil {
		args.VMCtx.Logger.Error(err, "failed to create snapshot for VM", "snapshot", obj.Name)
		return nil, err
	}

	// Update vm.status with currentSnapshot
	updateVMStatusCurrentSnapshot(args.VMCtx, obj)
	return snapMoRef, nil
}

func CreateSnapshot(args SnapshotArgs) (*types.ManagedObjectReference, error) {
	snapObj := args.VMSnapshot
	var quiesceSpec *types.VirtualMachineGuestQuiesceSpec
	if quiesce := snapObj.Spec.Quiesce; quiesce != nil {
		quiesceSpec = &types.VirtualMachineGuestQuiesceSpec{
			Timeout: int32(quiesce.Timeout.Round(time.Minute).Minutes()),
		}
	}

	t, err := args.VcVM.CreateSnapshotEx(args.VMCtx, snapObj.Name, snapObj.Spec.Description, snapObj.Spec.Memory, quiesceSpec)
	if err != nil {
		return nil, err
	}

	// Wait for task to finish
	taskInfo, err := t.WaitForResult(args.VMCtx)
	if err != nil {
		args.VMCtx.Logger.V(5).Error(err, "create snapshot task failed", "taskInfo", taskInfo)
		return nil, err
	}

	snapMoRef, ok := taskInfo.Result.(types.ManagedObjectReference)
	if !ok {
		return nil, fmt.Errorf("create snapshot VM task failed: %w", err)
	}

	return &snapMoRef, nil
}

func updateVMStatusCurrentSnapshot(vmCtx pkgctx.VirtualMachineContext, vmSnapshot vmopv1.VirtualMachineSnapshot) {
	vmCtx.VM.Status.CurrentSnapshot = &vmopv1common.LocalObjectRef{
		APIVersion: vmSnapshot.APIVersion,
		Kind:       vmSnapshot.Kind,
		Name:       vmSnapshot.Name,
	}
}
