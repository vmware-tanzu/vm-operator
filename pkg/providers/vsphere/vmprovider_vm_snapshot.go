// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

// DeleteSnapshot deletes the snapshot from the VM.
// The boolean indicating if the VM associated is deleted.
func (vs *vSphereVMProvider) DeleteSnapshot(
	ctx context.Context,
	vmSnapshot *vmopv1.VirtualMachineSnapshot,
	vm *vmopv1.VirtualMachine,
	removeChildren bool,
	consolidate *bool) (bool, error) {

	logger := pkgutil.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
	ctx = logr.NewContext(ctx, logger)

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "deleteSnapshot")),
		Logger:  logger,
		VM:      vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return false, err
	}

	vcVM, err := vs.getVM(vmCtx, client, false)
	if err != nil {
		return false, fmt.Errorf("failed to get VirtualMachine %q: %w", vmCtx.VM.Name, err)
	} else if vcVM == nil {
		return true, nil
	}

	if err := virtualmachine.DeleteSnapshot(virtualmachine.SnapshotArgs{
		VMSnapshot:     *vmSnapshot,
		VcVM:           vcVM,
		VMCtx:          vmCtx,
		RemoveChildren: removeChildren,
		Consolidate:    consolidate,
	}); err != nil {
		if errors.Is(err, virtualmachine.ErrSnapshotNotFound) {
			vmCtx.Logger.V(5).Info("snapshot not found")
			return false, nil
		}
		return false, err
	}

	return false, nil
}

// GetSnapshotSize gets the size of the snapshot from the VM.
func (vs *vSphereVMProvider) GetSnapshotSize(
	ctx context.Context,
	vmSnapshotName string,
	vm *vmopv1.VirtualMachine) (int64, error) {

	logger := pkgutil.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
	ctx = logr.NewContext(ctx, logger)

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "getSnapshotSize")),
		Logger:  logger,
		VM:      vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return 0, err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return 0, fmt.Errorf("failed to get VirtualMachine %q: %w", vmCtx.VM.Name, err)
	}

	err = vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot", "layoutEx", "config.hardware.device"}, &vmCtx.MoVM)
	if err != nil {
		return 0, err
	}

	vmSnapshot, err := virtualmachine.FindSnapshot(vmCtx, vmSnapshotName)
	if err != nil {
		return 0, fmt.Errorf("failed to find snapshot %q: %w", vmSnapshotName, err)
	}

	return virtualmachine.GetSnapshotSize(vmCtx, vmSnapshot), nil
}

// SyncVMSnapshotTreeStatus syncs the VM's current and root snapshots status.
func (vs *vSphereVMProvider) SyncVMSnapshotTreeStatus(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	logger := pkgutil.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
	ctx = logr.NewContext(ctx, logger)

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "syncVMSnapshotTreeStatus")),
		Logger:  logger,
		VM:      vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return fmt.Errorf("failed to get VirtualMachine %q: %w", vmCtx.VM.Name, err)
	}

	err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"snapshot"}, &vmCtx.MoVM)
	if err != nil {
		vmCtx.Logger.Error(err, "failed to get snapshot")
		return err
	}

	// Sync current and root snapshots status of the VM
	return vmlifecycle.SyncVMSnapshotTreeStatus(vmCtx, vs.k8sClient)
}

// markSnapshotInProgress marks a snapshot as currently being processed.
func (vs *vSphereVMProvider) markSnapshotInProgress(vmCtx pkgctx.VirtualMachineContext, vmSnapshot *vmopv1.VirtualMachineSnapshot) error {
	// Create a patch to set the InProgress condition
	patch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())

	// Set the InProgress condition to prevent concurrent operations
	pkgcnd.MarkFalse(
		vmSnapshot,
		vmopv1.VirtualMachineSnapshotReadyCondition,
		vmopv1.VirtualMachineSnapshotInProgressReason,
		"snapshot in progress")

	// Use Patch instead of Update to avoid conflicts with snapshot controller
	if err := vs.k8sClient.Status().Patch(vmCtx, vmSnapshot, patch); err != nil {
		return fmt.Errorf("failed to mark snapshot as in progress: %w", err)
	}

	vmCtx.Logger.V(4).Info("Marked snapshot as in progress", "snapshotName", vmSnapshot.Name)
	return nil
}

// markSnapshotFailed marks a snapshot as failed and clears the in-progress status.
func (vs *vSphereVMProvider) markSnapshotFailed(vmCtx pkgctx.VirtualMachineContext, vmSnapshot *vmopv1.VirtualMachineSnapshot, err error) {
	// Create a patch to update the snapshot status
	patch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())

	// Mark the snapshot as failed
	pkgcnd.MarkFalse(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition, "SnapshotFailed", "%s", err.Error())

	// Use Patch instead of Update to avoid conflicts with snapshot controller (best effort)
	if updateErr := vs.k8sClient.Status().Patch(vmCtx, vmSnapshot, patch); updateErr != nil {
		vmCtx.Logger.Error(updateErr, "Failed to update snapshot status after failure", "snapshotName", vmSnapshot.Name)
	}

	vmCtx.Logger.V(4).Info("Marked snapshot as failed", "snapshotName", vmSnapshot.Name, "error", err.Error())
}

// getVirtualMachineSnapshotsForVM finds all VirtualMachineSnapshot objects that reference this VM.
func (vs *vSphereVMProvider) getVirtualMachineSnapshotsForVM(
	vmCtx pkgctx.VirtualMachineContext) ([]vmopv1.VirtualMachineSnapshot, error) {

	// List all VirtualMachineSnapshot objects in the VM's namespace
	var snapshotList vmopv1.VirtualMachineSnapshotList
	if err := vs.k8sClient.List(vmCtx, &snapshotList, ctrlclient.InNamespace(vmCtx.VM.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineSnapshot objects: %w", err)
	}

	// Filter snapshots that reference this VM. We do this by checking
	// the VMRef, and by filtering the snapshots owned by this VM.
	var vmSnapshots []vmopv1.VirtualMachineSnapshot
	for _, snapshot := range snapshotList.Items {
		if snapshot.Spec.VMRef != nil && snapshot.Spec.VMRef.Name == vmCtx.VM.Name {
			vmSnapshots = append(vmSnapshots, snapshot)
		}
	}

	vmCtx.Logger.V(4).Info("Found VirtualMachineSnapshot objects for VM",
		"count", len(vmSnapshots))

	return vmSnapshots, nil
}
