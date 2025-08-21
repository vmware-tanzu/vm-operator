// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
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

	var moVM mo.VirtualMachine

	if err = vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot", "layoutEx", "config.hardware.device"}, &moVM); err != nil {
		return 0, err
	}

	vmCtx.MoVM = moVM

	vmNapshot, err := virtualmachine.FindSnapshot(vmCtx, vmSnapshotName)
	if err != nil {
		return 0, fmt.Errorf("failed to find snapshot %q: %w", vmSnapshotName, err)
	}

	size := virtualmachine.GetSnapshotSize(
		vmCtx, vmNapshot)

	return size, nil
}

func (vs *vSphereVMProvider) GetParentSnapshot(
	ctx context.Context,
	vmSnapshotName string,
	vm *vmopv1.VirtualMachine) (*vimtypes.VirtualMachineSnapshotTree, error) {

	logger := pkgutil.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
	ctx = logr.NewContext(ctx, logger)

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "getParentSnapshot")),
		Logger:  logger,
		VM:      vm,
	}

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get VirtualMachine %q: %w", vmCtx.VM.Name, err)
	}

	return virtualmachine.GetParentSnapshot(vmCtx, vcVM, vmSnapshotName)
}

// SyncVMSnapshotTreeStatus syncs the VM's current and root snapshots status.
func (vs *vSphereVMProvider) SyncVMSnapshotTreeStatus(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	logger := pkgutil.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
	ctx = logr.NewContext(ctx, logger)

	vmCtx := pkgctx.VirtualMachineContext{
		Context: context.WithValue(ctx, vimtypes.ID{}, vs.getOpID(ctx, vm, "getParentSnapshot")),
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

	var o mo.VirtualMachine
	err = vcVM.Properties(vmCtx, vcVM.Reference(), []string{"snapshot"}, &o)
	if err != nil {
		vmCtx.Logger.Error(err, "failed to get snapshot")
		return err
	}

	vmCtx.MoVM = o

	// sync current and root snapshots status of the VM
	return vmlifecycle.SyncVMSnapshotTreeStatus(vmCtx, vs.k8sClient)
}

// PatchSnapshotSuccessStatus patches the snapshot status to reflect the success of the snapshot operation.
func PatchSnapshotSuccessStatus(vmCtx pkgctx.VirtualMachineContext, k8sClient ctrlclient.Client,
	vmSnapshot *vmopv1.VirtualMachineSnapshot, snapMoRef *vimtypes.ManagedObjectReference) error {
	snapPatch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())
	vmSnapshot.Status.UniqueID = snapMoRef.Reference().Value
	vmSnapshot.Status.Quiesced = vmSnapshot.Spec.Quiesce != nil
	vmSnapshot.Status.PowerState = vmCtx.VM.Status.PowerState

	pkgcnd.MarkTrue(vmSnapshot, vmopv1.VirtualMachineSnapshotReadyCondition)

	if err := k8sClient.Status().Patch(vmCtx, vmSnapshot, snapPatch); err != nil {
		return fmt.Errorf(
			"failed to patch snapshot status resource %s/%s: err: %s",
			vmSnapshot.Name, vmSnapshot.Namespace, err.Error())
	}

	return nil
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

// patchChildrenSnapshotStatus find the parent snapshot of current snapshot
// and patches the children snapshot status of the parent snapshot by adding
// the current snapshot to the children list of the parent snapshot.
func (vs *vSphereVMProvider) patchChildrenSnapshotStatus(snapArgs virtualmachine.SnapshotArgs) error {
	// Use vsphereprovider's GetParentSnapshot since it will fetch latest
	// version of moVM from vsphere, after a new snapshot was taken above.
	vmCtx := snapArgs.VMCtx
	vmSnapshot := snapArgs.VMSnapshot

	vmCtx.Logger.V(4).Info("Patching children snapshot status of parent snapshot", "child SnapshotName", vmSnapshot.Name)

	parentSnapshot, err := virtualmachine.GetParentSnapshot(vmCtx, snapArgs.VcVM, vmSnapshot.Name)
	if err != nil {
		vmCtx.Logger.Error(err, "failed to get parent snapshot", "child SnapshotName", vmSnapshot.Name)
		return err
	}

	if parentSnapshot == nil {
		vmCtx.Logger.V(4).Info("No parent snapshot found for snapshot, skipping children snapshot status update",
			"snapshotName", vmSnapshot.Name)
		return nil
	}

	parentCR := &vmopv1.VirtualMachineSnapshot{}
	if err := vs.k8sClient.Get(vmCtx,
		ctrlclient.ObjectKey{Name: parentSnapshot.Name, Namespace: vmSnapshot.Namespace}, parentCR); err != nil {
		if apierrors.IsNotFound(err) {
			// TODO (lubron): The parent snapshot might be externally created (only exist in vSphere)
			// In this case, we should just skip the update.
			// For now let's return an error.
			return fmt.Errorf("parent snapshot %s/%s not found", parentSnapshot.Name, vmSnapshot.Namespace)
		}
		return fmt.Errorf("failed to get parent snapshot %s/%s: err: %s", parentSnapshot.Name, vmSnapshot.Namespace, err.Error())
	}

	if slices.ContainsFunc(parentCR.Status.Children, func(c common.LocalObjectRef) bool {
		return c.Name == vmSnapshot.Name
	}) {
		vmCtx.Logger.V(4).Info("Snapshot already in children list of parent, skipping",
			"parentSnapshotName", parentSnapshot.Name,
			"snapshotName", vmSnapshot.Name)
		return nil
	}

	objPatch := ctrlclient.MergeFrom(parentCR.DeepCopy())

	parentCR.Status.Children = append(parentCR.Status.Children, common.LocalObjectRef{
		Name:       vmSnapshot.Name,
		APIVersion: vmopv1.GroupVersion.String(),
		Kind:       "VirtualMachineSnapshot",
	})

	return vs.k8sClient.Status().Patch(vmCtx, parentCR, objPatch)
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

func (vs *vSphereVMProvider) updateVMStatus(
	vmCtx pkgctx.VirtualMachineContext,
	snapArgs virtualmachine.SnapshotArgs,
	snapshotToProcess vmopv1.VirtualMachineSnapshot) error {
	vmCtx.Logger.V(4).Info("Updating VM status", "snapshotName", snapshotToProcess.Name)

	// Update the children snapshot status of parent snapshot
	if err := vs.patchChildrenSnapshotStatus(snapArgs); err != nil {
		vmCtx.Logger.Error(err, "Failed to update children snapshot status in its parent", "childrenSnapshotName", snapshotToProcess.Name)
		return err
	}

	// Sync the snapshot tree status, including the current and root snapshots status.
	return vmlifecycle.SyncVMSnapshotTreeStatus(vmCtx, vs.k8sClient)
}
