// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	k8syaml "sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	vmconfunmanagedvolsfil "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/backfill"
	vmconfunmanagedvolsreg "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/register"

	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
)

// DeleteSnapshot deletes the snapshot from the VM.
// The boolean indicating if the VM associated is deleted.
func (vs *vSphereVMProvider) DeleteSnapshot(
	ctx context.Context,
	vmSnapshot *vmopv1.VirtualMachineSnapshot,
	vm *vmopv1.VirtualMachine,
	removeChildren bool,
	consolidate *bool) (bool, error) {

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "deleteSnapshot"),
		vm,
	)
	ctx = vmCtx.Context

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

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "getSnapshotSize"),
		vm,
	)
	ctx = vmCtx.Context

	client, err := vs.getVcClient(ctx)
	if err != nil {
		return 0, err
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return 0, fmt.Errorf("failed to get VirtualMachine %q: %w", vmCtx.VM.Name, err)
	}

	moVM := mo.VirtualMachine{}
	if err := vcVM.Properties(
		ctx, vcVM.Reference(),
		[]string{"snapshot", "layoutEx", "config.hardware.device"},
		&moVM); err != nil {
		return 0, err
	}

	vmSnapshot, err := virtualmachine.FindSnapshot(moVM, vmSnapshotName)
	if err != nil {
		return 0, fmt.Errorf("failed to find snapshot %q: %w", vmSnapshotName, err)
	}

	total, _, err := virtualmachine.GetSnapshotSize(vmCtx, moVM, &vmSnapshot.Snapshot)
	return total, err
}

// GetSnapshotDeviceConfig returns the virtual device list from a vCenter
// snapshot's saved configuration. Used to capture diskPaths for VM-owned
// volumes before the snapshot's vCenter record is deleted (D.2).
//
// Returns nil when no snapshot with the given name exists on the VM.
func (vs *vSphereVMProvider) GetSnapshotDeviceConfig(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	snapshotName string,
) ([]vimtypes.BaseVirtualDevice, error) {

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "getSnapshotDeviceConfig"),
		vm,
	)
	ctx = vmCtx.Context

	vcClient, err := vs.getVcClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vCenter client: %w", err)
	}

	vcVM, err := vs.getVM(vmCtx, vcClient, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM %q: %w", vmCtx.VM.Name, err)
	}

	moVMObj := mo.VirtualMachine{}
	if err := vcVM.Properties(ctx, vcVM.Reference(), []string{"snapshot"}, &moVMObj); err != nil {
		return nil, fmt.Errorf("failed to fetch snapshot property on VM %q: %w",
			vmCtx.VM.Name, err)
	}

	snapNode, err := virtualmachine.FindSnapshot(moVMObj, snapshotName)
	if err != nil || snapNode == nil {
		return nil, nil
	}

	// Fetch the snapshot object's device config.
	snapRef := snapNode.Snapshot
	var moSnap mo.VirtualMachineSnapshot
	if err := vcVM.Properties(ctx, snapRef, []string{"config.hardware.device"}, &moSnap); err != nil {
		return nil, fmt.Errorf("failed to fetch device config from snapshot %q: %w",
			snapshotName, err)
	}

	return moSnap.Config.Hardware.Device, nil
}

// SyncVMSnapshotTreeStatus syncs the VM's current and root snapshots status.
func (vs *vSphereVMProvider) SyncVMSnapshotTreeStatus(
	ctx context.Context,
	vm *vmopv1.VirtualMachine) error {

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "syncVMSnapshotTreeStatus"),
		vm,
	)
	ctx = vmCtx.Context

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
		return fmt.Errorf("failed to get snapshot: %w", err)
	}

	// Sync current and root snapshots status of the VM
	return vmlifecycle.SyncVMSnapshotTreeStatus(vmCtx, vs.k8sClient)
}

// markSnapshotInProgress marks a snapshot as currently being processed.
func markSnapshotInProgress(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vmSnapshot *vmopv1.VirtualMachineSnapshot) error {
	// Create a patch to set the InProgress condition
	patch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())

	// Set the InProgress condition to prevent concurrent operations
	pkgcnd.MarkFalse(
		vmSnapshot,
		vmopv1.VirtualMachineSnapshotCreatedCondition,
		vmopv1.VirtualMachineSnapshotCreationInProgressReason,
		"snapshot in progress")

	// Use Patch instead of Update to avoid conflicts with snapshot controller
	if err := k8sClient.Status().Patch(vmCtx, vmSnapshot, patch); err != nil {
		return fmt.Errorf("failed to mark snapshot as in progress: %w", err)
	}

	vmCtx.Logger.V(4).Info("Marked snapshot as in progress", "snapshotName", vmSnapshot.Name)
	return nil
}

// markSnapshotFailed marks a snapshot as failed and clears the in-progress status.
func markSnapshotFailed(vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vmSnapshot *vmopv1.VirtualMachineSnapshot, err error) error {
	// Create a patch to update the snapshot status.
	patch := ctrlclient.MergeFrom(vmSnapshot.DeepCopy())

	// Mark the snapshot as failed.
	pkgcnd.MarkError(
		vmSnapshot,
		vmopv1.VirtualMachineSnapshotCreatedCondition,
		vmopv1.VirtualMachineSnapshotCreationFailedReason,
		err)

	// Use Patch instead of Update to avoid conflicts with snapshot controller (best effort)
	if updateErr := k8sClient.Status().Patch(vmCtx, vmSnapshot, patch); updateErr != nil {
		return fmt.Errorf("failed to update snapshot status after failure: %w", updateErr)
	}

	vmCtx.Logger.V(4).Info("Marked snapshot as failed", "snapshotName", vmSnapshot.Name, "error", err.Error())
	return nil
}

// getVirtualMachineSnapshotsForVM finds all VirtualMachineSnapshot objects that reference this VM.
func getVirtualMachineSnapshotsForVM(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client) ([]vmopv1.VirtualMachineSnapshot, error) {

	// List all VirtualMachineSnapshot objects in the VM's namespace
	var snapshotList vmopv1.VirtualMachineSnapshotList
	if err := k8sClient.List(vmCtx, &snapshotList, ctrlclient.InNamespace(vmCtx.VM.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineSnapshot objects: %w", err)
	}

	// Filter snapshots that reference this VM. We do this by checking
	// the VMName, and by filtering the snapshots owned by this VM.
	var vmSnapshots []vmopv1.VirtualMachineSnapshot
	for _, snapshot := range snapshotList.Items {
		if snapshot.Spec.VMName == vmCtx.VM.Name {
			vmSnapshots = append(vmSnapshots, snapshot)
		}
	}

	vmCtx.Logger.V(4).Info("Current count of VirtualMachineSnapshot objects for VM",
		"count", len(vmSnapshots))

	return vmSnapshots, nil
}

func (vs *vSphereVMProvider) reconcileSnapshotRevert(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	if err := vs.reconcileSnapshotRevertCheckTask(vmCtx, vcVM); err != nil {
		return err
	}

	return vs.reconcileSnapshotRevertDoTask(vmCtx, vcVM)
}

func (vs *vSphereVMProvider) reconcileSnapshotRevertCheckTask(
	vmCtx pkgctx.VirtualMachineContext,
	_ *object.VirtualMachine) error {

	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.V(4).Info("Reconciling snapshot task")

	//
	// Check if there are any running snapshot-related tasks and skip
	// reconciliation if so. This includes create snapshot, delete snapshot,
	// and revert snapshot operations.
	//
	if pkgctx.HasVMRunningSnapshotTask(vmCtx) {
		return pkgerr.NoRequeueNoErr("snapshot operation in progress")
	}

	//
	// Check if a snapshot revert annotation is present but no corresponding
	// vSphere task is running. This handles the case where the vSphere
	// revert task completed but there was an issue with the VM spec
	// restoration or annotation cleanup.
	//
	_, ok := vmCtx.VM.Annotations[pkgconst.VirtualMachineSnapshotRevertInProgressAnnotationKey]
	if ok {
		return pkgerr.NoRequeueNoErr("snapshot revert in progress")
	}

	return nil
}

func (vs *vSphereVMProvider) reconcileSnapshotRevertDoTask(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.V(4).Info("Reconciling snapshot revert")

	// If no spec.currentSnapshot is specified, nothing to revert to.
	if vmCtx.VM.Spec.CurrentSnapshotName == "" {
		logger.V(4).Info("Skipping revert for empty spec.currentSnapshot")

		// Clear any existing snapshot revert condition.
		// This handles the case where a previous revert
		// failed and the user removed spec.currentSnapshot
		// to abort the revert.
		pkgcnd.Delete(vmCtx.VM, vmopv1.VirtualMachineSnapshotRevertSucceeded)

		return nil
	}

	desiredSnapshotName := vmCtx.VM.Spec.CurrentSnapshotName
	logger = logger.WithValues(
		"desiredSnapshotName",
		vmCtx.VM.Spec.CurrentSnapshotName)
	vmCtx.Context = logr.NewContext(vmCtx.Context, logger)

	// Skip snapshot revert processing for VKS/TKG nodes.
	if kubeutil.HasCAPILabels(vmCtx.VM.Labels) {
		message := "Skipping snapshot revert for VKS node"

		pkgcnd.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertSkippedReason,
			"%s",
			message,
		)

		logger.V(4).Info(message)
		return nil
	}

	// Check if the snapshot exists.
	obj := &vmopv1.VirtualMachineSnapshot{}
	if err := vs.k8sClient.Get(
		vmCtx,
		ctrlclient.ObjectKey{
			Name:      desiredSnapshotName,
			Namespace: vmCtx.VM.Namespace,
		},
		obj); err != nil {

		err := fmt.Errorf(
			"failed to get snapshot object %q: %w", desiredSnapshotName, err)

		pkgcnd.MarkError(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertFailedReason,
			err,
		)

		return err
	}

	// Check if the snapshot is ready. This is to ensure that the snapshot
	// as reconciled by VM operator and is not an out-of-band, or in-progress
	// snapshot.
	if !pkgcnd.IsTrue(obj, vmopv1.VirtualMachineSnapshotReadyCondition) {
		err := fmt.Errorf(
			"skipping revert for not-ready snapshot %q",
			desiredSnapshotName)

		pkgcnd.MarkError(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertSkippedReason,
			err,
		)

		return err
	}

	// Check if the VM has a snapshot by the desired name.
	snapNode, err := virtualmachine.FindSnapshot(vmCtx.MoVM, desiredSnapshotName)
	if err != nil || snapNode == nil {
		err := fmt.Errorf("failed to find vSphere snapshot %q: %w",
			desiredSnapshotName, err)

		pkgcnd.MarkError(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertFailedReason,
			err,
		)

		return err
	}

	ref := &snapNode.Snapshot

	logger = logger.WithValues("desiredSnapshotRef", ref.Value)
	vmCtx.Context = logr.NewContext(vmCtx.Context, logger)
	logger.V(4).Info("Found vSphere snapshot")

	var currentRef vimtypes.ManagedObjectReference
	if s := vmCtx.MoVM.Snapshot; s != nil {
		if c := s.CurrentSnapshot; c != nil {
			currentRef = ref.Reference()
			logger = logger.WithValues("currentSnapshot", currentRef.Value)
		}
	}
	logger = logger.WithValues("isCurrent", currentRef == *ref)
	vmCtx.Context = logr.NewContext(vmCtx.Context, logger)

	logger.Info("Starting snapshot revert operation")

	// Set the revert in progress annotation.
	logger.V(4).Info("Setting revert in progress annotation")
	vmCtx.VM.SetAnnotation(
		pkgconst.VirtualMachineSnapshotRevertInProgressAnnotationKey, "")

	// Set the VM snapshot revert in progress condition.
	// This condition will be cleared once the revert is successful,
	// since the Status is wiped out after a revert.
	pkgcnd.MarkFalse(vmCtx.VM,
		vmopv1.VirtualMachineSnapshotRevertSucceeded,
		vmopv1.VirtualMachineSnapshotRevertInProgressReason,
		"%s",
		"snapshot revert in progress",
	)

	// Perform the actual snapshot revert
	logger.V(4).Info("Starting vSphere snapshot revert operation")
	if err := vs.performSnapshotRevert(
		vmCtx, vcVM, ref, desiredSnapshotName); err != nil {

		return fmt.Errorf("failed to revert vSphere snapshot %q: %w",
			desiredSnapshotName, err)
	}

	// TODO (AKP): We will modify the snapshot workflow to always skip the
	// automatic registration.  Once we do that, we will have to remove that
	// ExtraConfig key.

	// TODO (AKP): Move the parsing VM spec code above this section so we can
	// bail out if it's not a VM that we can restore.

	// Capture the volumes present on the VM before restoring the snapshot
	// spec. The difference between pre- and post-revert volumes determines
	// which volumes were dropped by the revert (C.5 in the VM-owned storage
	// ownership-transfer workflow).
	preRevertVolumes := make([]vmopv1.VirtualMachineVolume, len(vmCtx.VM.Spec.Volumes))
	copy(preRevertVolumes, vmCtx.VM.Spec.Volumes)

	// Restore VM spec and metadata from the snapshot.
	logger.Info("Starting VM spec and metadata restoration from snapshot",
		"beforeRestoreAnnotations", vmCtx.VM.Annotations,
		"beforeRestoreLabels", vmCtx.VM.Labels,
		"beforeRestoreSpecCurrentSnapshot", vmCtx.VM.Spec.CurrentSnapshotName)

	if err := vs.restoreVMSpecFromSnapshot(
		vmCtx, vcVM, obj, snapNode); err != nil {

		err := fmt.Errorf(
			"failed to restore vm spec and metadata from snapshot: %w", err)

		pkgcnd.MarkError(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertFailedInvalidVMManifestReason,
			err,
		)

		return err
	}

	logger.V(4).Info("VM spec restoration completed",
		"afterRestoreAnnotations", vmCtx.VM.Annotations,
		"afterRestoreLabels", vmCtx.VM.Labels,
		"afterRestoreSpecCurrentSnapshot", vmCtx.VM.Spec.CurrentSnapshotName)

	// Signal CSI about any VM-owned volumes that were dropped by the revert.
	// This updates the BA so CSI can re-evaluate the volume ownership state.
	if err := vs.handleRevertInducedDrops(
		vmCtx, preRevertVolumes, vmCtx.VM.Spec.Volumes); err != nil {
		// Non-fatal: log and continue. The BA will be corrected on the next
		// reconcile via the volumebatch controller's re-adoption path.
		logger.Error(err, "Failed to signal revert-induced volume drops; "+
			"volumebatch controller will reconcile on next pass")
	}

	logger.Info("Successfully completed reverted snapshot")

	// Return a NoRequeue error so the updateVirtualMachine func exit
	// immediately. A successful revert and spec restoration
	// to will trigger another reconcile right after this one.
	// This ensures the next reconcile operates on
	// the updated VM spec, labels, and annotations. The VM status
	// will be updated in the subsequent reconcile loop.
	return ErrSnapshotRevert
}

// handleRevertInducedDrops signals CSI about volumes that were removed from
// the VM by a snapshot revert operation. For each dropped volume it patches
// the BA with a VolumeDetached=True/DroppedBySnapshotRevert condition and
// removes the volume from the BA spec.
//
// vSphere already removed the disks during the revert so no ReconfigVM call is
// issued. The diskPath on CVI is preserved (it holds the last-known value;
// the JIT resolution model does not require it to be updated here).
//
// This function is idempotent: volumes already absent from the BA spec or that
// already carry the DroppedBySnapshotRevert condition are skipped.
func (vs *vSphereVMProvider) handleRevertInducedDrops(
	vmCtx pkgctx.VirtualMachineContext,
	preRevertVolumes []vmopv1.VirtualMachineVolume,
	postRevertVolumes []vmopv1.VirtualMachineVolume,
) error {
	if !pkgcfg.FromContext(vmCtx).Features.VMOwnedVolumes {
		return nil
	}
	if !vmopv1util.IsVMOwnedStorageVM(vmCtx.VM) {
		return nil
	}

	// Build a set of volume names in the post-revert spec.
	postRevertNames := make(map[string]struct{}, len(postRevertVolumes))
	for _, v := range postRevertVolumes {
		postRevertNames[v.Name] = struct{}{}
	}

	// Compute the dropped set: volumes in pre but not in post.
	var droppedNames []string
	for _, v := range preRevertVolumes {
		if _, ok := postRevertNames[v.Name]; !ok {
			droppedNames = append(droppedNames, v.Name)
		}
	}

	if len(droppedNames) == 0 {
		return nil
	}

	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.Info("Revert dropped VM-owned volumes; signaling CSI via BA",
		"vmName", vmCtx.VM.Name,
		"droppedCount", len(droppedNames),
		"droppedVolumes", droppedNames)

	// Look up the BA for this VM.
	ba := &cnsv1alpha1.CnsNodeVMBatchAttachment{}
	baKey := ctrlclient.ObjectKey{
		Name:      pkgutil.CNSBatchAttachmentNameForVM(vmCtx.VM.Name),
		Namespace: vmCtx.VM.Namespace,
	}
	if err := vs.k8sClient.Get(vmCtx, baKey, ba); err != nil {
		if apierrors.IsNotFound(err) {
			// No BA yet; nothing to update.
			return nil
		}
		return fmt.Errorf("failed to get CnsNodeVMBatchAttachment %s: %w", baKey.Name, err)
	}

	// Capture the original state for both patches before making in-memory
	// changes. The status subresource and main resource patches must each
	// compute their diff from the pre-mutation state.
	baOriginal := ba.DeepCopy()

	for _, volName := range droppedNames {
		// Set condition VolumeDetached=True, reason=DroppedBySnapshotRevert in BA status.
		for i, volStatus := range ba.Status.VolumeStatus {
			if volStatus.Name != volName {
				continue
			}
			// Check idempotency.
			alreadySet := false
			for _, c := range volStatus.PersistentVolumeClaim.Conditions {
				if c.Type == cnsv1alpha1.ConditionDetached &&
					c.Reason == cnsv1alpha1.ReasonDroppedBySnapshotRevert {
					alreadySet = true
					break
				}
			}
			if !alreadySet {
				ba.Status.VolumeStatus[i].PersistentVolumeClaim.Conditions = append(
					ba.Status.VolumeStatus[i].PersistentVolumeClaim.Conditions,
					metav1.Condition{
						Type:               cnsv1alpha1.ConditionDetached,
						Status:             metav1.ConditionTrue,
						Reason:             cnsv1alpha1.ReasonDroppedBySnapshotRevert,
						LastTransitionTime: metav1.Now(),
					},
				)
			}
		}

		// Remove the volume from the BA spec.
		filtered := ba.Spec.Volumes[:0]
		for _, specVol := range ba.Spec.Volumes {
			if specVol.Name != volName {
				filtered = append(filtered, specVol)
			}
		}
		ba.Spec.Volumes = filtered

		logger.Info("Signaled revert-induced drop for volume",
			"volumeName", volName,
			"vmName", vmCtx.VM.Name)
	}

	// Patch spec first (main resource), then status (status subresource).
	// Both diffs are computed against the original pre-mutation state.
	if err := vs.k8sClient.Patch(vmCtx, ba, ctrlclient.MergeFrom(baOriginal)); err != nil {
		return fmt.Errorf("failed to patch BA spec for revert-induced drops: %w", err)
	}

	if err := vs.k8sClient.Status().Patch(vmCtx, ba, ctrlclient.MergeFrom(baOriginal)); err != nil {
		return fmt.Errorf("failed to patch BA status for revert-induced drops: %w", err)
	}

	return nil
}

// performSnapshotRevert executes the actual snapshot revert operation.
func (vs *vSphereVMProvider) performSnapshotRevert(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	snapObj *vimtypes.ManagedObjectReference,
	snapshotName string) error {

	logger := pkglog.FromContextOrDefault(vmCtx)

	// TODO (AKP): Add support for suppresspoweron option.
	task, err := vcVM.RevertToSnapshot(vmCtx, snapObj.Value, false)
	if err != nil {
		return fmt.Errorf("failed to call revert snapshot %q: %w",
			snapshotName, err)
	}

	// Wait for the revert task to complete
	taskInfo, err := task.WaitForResult(vmCtx)
	if err != nil {
		var conditionMessage string

		// taskInfo.Error is of the format:
		//
		// {
		// 	"fault": {
		//   "faultMessage": [
		//     {
		//       "key":"msg.snapshot.vigor.revert.error",
		//       "arg":[
		//         {
		//           "key":"1",
		//           "value":"msg.snapshot.error-NOTREVERTABLE"
		//         }
		//       ],
		//       "message":"An error occurred while reverting to a
		// 			snapshot: The specified snapshot is not revertable."
		//     },
		//     {
		//       "key":"msg.snapshot.vigor.revert.errorFcd",
		//       "message":"The virtual machine cannot be reverted when
		// 			crossing an Improved-Virtual-Disk snapshot."
		//      }
		//   ]
		// }
		//
		// Note: A risk is that if the key strings change in future
		// vSphere versions, the error parsing will break. However,
		// this is unlikely to happen without notice.
		revertErrorMessage := errorMessageFromTaskInfoWithKey(
			taskInfo, "msg.snapshot.vigor.revert.error",
		)
		if revertErrorMessage != "" {
			conditionMessage += revertErrorMessage
		}

		// We don't use the fcdErrorMessage directly since it has references
		// to Improved-Virtual-Disk which may not be clear to users.
		fcdErrorMessage := errorMessageFromTaskInfoWithKey(
			taskInfo, "msg.snapshot.vigor.revert.errorFcd",
		)
		if fcdErrorMessage != "" {
			conditionMessage += " The virtual machine cannot be reverted " +
				"when crossing a CSI VolumeSnapshot. " +
				"VolumeSnapshots should be deleted before retrying the revert."
		}

		// The condition reason and message are based on the error
		// encountered. It would be cleared once the revert is successful.
		pkgcnd.MarkFalse(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertTaskFailedReason,
			"%s",
			conditionMessage,
		)

		return fmt.Errorf("failed to revert snapshot %q, taskInfo=%+v: %w",
			snapshotName, taskInfo, err)
	}

	logger.Info("Successfully reverted VM to snapshot")
	return nil
}

// restoreVMSpecFromSnapshot extracts and applies the VM spec from the snapshot.
func (vs *vSphereVMProvider) restoreVMSpecFromSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	snapCR *vmopv1.VirtualMachineSnapshot,
	snapNode *vimtypes.VirtualMachineSnapshotTree) error {
	if snapNode == nil {
		return fmt.Errorf("snapshot node is nil")
	}

	vmYAML, err := vs.getVMYamlFromSnapshot(vmCtx, vcVM, snapCR, snapNode.Snapshot)
	if err != nil {
		return fmt.Errorf("failed to parse VM spec from snapshot: %w", err)
	}

	// Handle cases where the snapshot doesn't contain any VM spec and metadata.
	if vmYAML == "" {
		return fmt.Errorf("no VM YAML in snapshot config")
	}

	vmCtx.Logger.V(5).Info("Restoring VM spec from snapshot", "vmYAML", vmYAML)
	// Unmarshal and convert VM from yaml in snapshot based on API version
	vm, err := vs.unmarshalAndConvertVMFromYAML(vmYAML)
	if err != nil {
		return fmt.Errorf("failed to unmarshal and convert VM from snapshot: %w", err)
	}

	// Swap the spec, annotations and labels. We don't need to worry
	// about pruning fields here since the backup process already
	// handles that.
	vmCtx.VM.Spec = *(&vm.Spec).DeepCopy()

	// VM spec should never have spec.currentSnapshot set because the
	// currentSnapshot is only set during a revert operation. However,
	// in the off-chance that a snapshot was taken _during_ a revert,
	// clear it out.
	vmCtx.VM.Spec.CurrentSnapshotName = ""

	// TODO: AKP: We need to merge (and skip) some labels so that we
	// are not simply reverting to the previous labels. Otherwise, we
	// risk the VM being impacted after a snapshot revert.
	vmCtx.VM.Labels = maps.Clone(vm.Labels)
	vmCtx.VM.Annotations = maps.Clone(vm.Annotations)
	// Empty out the status.
	vmCtx.VM.Status = vmopv1.VirtualMachineStatus{}

	// Reconcile the power state of the VM post revert.
	// vpxd sets the power state of the snapshot to Off if a VM which is On
	// is snapshotted without the memory state. We don't need to separately
	// handle that case here.
	vmCtx.VM.Spec.PowerState = vmopv1util.ConvertPowerState(vmCtx.Logger, snapNode.State)

	// Note that since we swapped out the annotations from the
	// snapshot, it will implicitly remove the "restore-in-progress"
	// annotation. But go ahead and clean it up again in case the
	// snapshot somehow contained this annotation.
	delete(
		vmCtx.VM.Annotations,
		pkgconst.VirtualMachineSnapshotRevertInProgressAnnotationKey)

	return nil
}

// getVMYamlFromSnapshot extracts the VM YAML from snapshot's ExtraConfig.
func (vs *vSphereVMProvider) getVMYamlFromSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	snap *vmopv1.VirtualMachineSnapshot,
	snapRef vimtypes.ManagedObjectReference) (string, error) {

	var (
		moSnap mo.VirtualMachineSnapshot
		logger = pkglog.FromContextOrDefault(vmCtx)
	)

	// Fetch the config stored with the snapshot.
	if err := vcVM.Properties(
		vmCtx, snapRef, []string{"config"}, &moSnap); err != nil {

		return "", fmt.Errorf("failed to fetch snapshot config: %w", err)
	}

	logger.V(4).Info("Successfully fetched snapshot properties")

	ecList := object.OptionValueList(moSnap.Config.ExtraConfig)
	raw, _ := ecList.GetString(backupapi.VMResourceYAMLExtraConfigKey)
	if raw == "" {
		// If the Snapshot has ImportedSnapshotAnnotation, then approximate the
		// VM spec.
		return getVMYamlFromSnapshotFromSynthesizedSpec(
			vmCtx, snap, moSnap)
	}

	logger.V(4).Info("Found backup data in snapshot config",
		"key", backupapi.VMResourceYAMLExtraConfigKey,
		"dataLength", len(raw))

	// Uncompress and decode the data.
	data, err := pkgutil.TryToDecodeBase64Gzip([]byte(raw))
	if err != nil {
		return "", fmt.Errorf("failed to decode payload from snapshot: %w", err)
	}

	return data, nil
}

func getVMYamlFromSnapshotFromSynthesizedSpec(
	vmCtx pkgctx.VirtualMachineContext,
	snapCR *vmopv1.VirtualMachineSnapshot,
	moSnap mo.VirtualMachineSnapshot) (string, error) {

	logger := pkglog.FromContextOrDefault(vmCtx)

	if _, ok := snapCR.Annotations[vmopv1.ImportedSnapshotAnnotation]; !ok {
		logger.Info("Unsupported revert attempted for snapshot "+
			"sans ExtraConfig and annotation",
			"annotationKey", vmopv1.ImportedSnapshotAnnotation)
		return "", nil
	}

	logger.Info(
		"Approximating VM spec when reverting snapshot sans ExtraConfig",
		"extraConfigKey", backupapi.VMResourceYAMLExtraConfigKey)

	// For VMs that are imported, we do not have the
	// ExtraConfig set in the snapshot. In this case, we
	// will approximate the spec from the VirtualMachineConfigInfo.
	// This is a best-effort attempt to restore the VM spec
	// and metadata from the snapshot.

	vm := vmopv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmCtx.VM.Name,
			Namespace: vmCtx.VM.Namespace,
		},
		// This section is not necessary but
		// to workaround the issue in the test.
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualMachine",
			APIVersion: vmopv1.GroupVersion.String(),
		},
		Spec: synthesizeVMSpecForSnapshot(*vmCtx.VM, vmCtx.MoVM, moSnap),
	}

	// Number of elements in the spec.network.interfaces list must be same
	// as number of VM's NICs from ConfigInfo.
	// Details of each interface (which network does it connect to) is
	// ignored.
	// Networking can be fixed after revert and it is not possible to map
	// the vSphere network to Supervisor networks.
	devices := object.VirtualDeviceList(moSnap.Config.Hardware.Device)
	vNICs := devices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))
	if len(vNICs) == 0 {
		// If no vNICs are defined, then disable network
		vm.Spec.Network.Disabled = true
	} else {
		interfaces := make(
			[]vmopv1.VirtualMachineNetworkInterfaceSpec,
			len(vNICs))

		for i := range vNICs {
			interfaces[i] = vmopv1.VirtualMachineNetworkInterfaceSpec{
				Name: fmt.Sprintf("eth%d", i),
			}
		}
		vm.Spec.Network.Interfaces = interfaces
	}

	// MinHardwareVersion is set to the current hardware version of the VM.
	chv, err := strconv.ParseInt(moSnap.Config.Version, 10, 32)
	if err != nil {
		logger.V(4).Info("Unable to approximate minimum hardware version",
			"hwVersionFromSnapshot", moSnap.Config.Version)
	} else {
		vm.Spec.MinHardwareVersion = int32(chv)
	}

	// TODO (APK): Labels and Annotations are skipped for now as design
	// discussions are on storing Labels and Annotations in VC are in
	// progress.
	vm.Labels = map[string]string{}
	vm.Annotations = map[string]string{}

	vmYAML, err := k8syaml.Marshal(vm)
	if err != nil {
		return "", fmt.Errorf(
			"failed to marshal VM into YAML %+v: %w", vm, err)
	}

	return string(vmYAML), nil
}

func synthesizeVMSpecForSnapshot(
	vm vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	moSnap mo.VirtualMachineSnapshot) vmopv1.VirtualMachineSpec {

	hostName := ""
	if moVM.Guest != nil {
		hostName = moVM.Guest.HostName
	}

	return vmopv1.VirtualMachineSpec{
		InstanceUUID: moSnap.Config.InstanceUuid,
		BiosUUID:     moSnap.Config.Uuid,
		GuestID:      moSnap.Config.GuestId,

		// Classless and Imageless VMs
		ClassName: "",
		ImageName: "",
		Image:     nil,

		// We only support VM snapshots and affinity policies are not part
		// of VMs.
		Affinity: nil,

		// It's not possible to construct the crypto spec looking at the VM.
		Crypto: nil,

		// No PVCs for imported VMs. For day 2, users can convert disks in
		// PVCs using just-in-time PVCs.
		Volumes: []vmopv1.VirtualMachineVolume{},

		// This is used for slot reservation starting VCF 9.0.  The reverted
		// VM will continue to use the reserved profile but it won't be part
		// of the spec since the plumbing from VM class to VCFA won't exist
		// (VCFA expects slots of a class to be reserved if this is set).
		Reserved: nil,

		// The VM will continue to have the same boot options from the
		// snapshot state. If user wants to change that, they can specify a
		// custom boot option (boot order).
		BootOptions: nil,

		// PowerOffMode is set to the reasonable default of TrySoft.
		// User can change it post-revert if needed.
		// If after the revert, the VM switches from being On to Off,
		// PowerOffMode is required to be set. Otherwise, switching
		// power state will fail since between restoring the VM spec
		// and the next power operation, there is no defaulting operation
		// to set the default.
		PowerOffMode: vmopv1.VirtualMachinePowerOpModeTrySoft,

		// Used to trigger a revert, so must be empty.
		CurrentSnapshotName: "",

		// Group name is overridden to the VM's current group.
		// We can't know (or guess) what the group the VM was when the
		// snapshot was taken.
		GroupName: vm.Spec.GroupName,

		// When Snapshot is taken with Policy A and then VM is updated to
		// use Policy B, after a revert to Snapshot with Policy A is
		// performed, VM continues to have Policy A. So, we just simplify by
		// setting the StorageClass to same as the VM's current
		// StorageClass.
		StorageClass: vm.Spec.StorageClass,

		Advanced: &vmopv1.VirtualMachineAdvancedSpec{
			ChangeBlockTracking: moSnap.Config.ChangeTrackingEnabled,
		},

		Network: &vmopv1.VirtualMachineNetworkSpec{
			HostName: hostName,
		},
	}
}

// ReconcileCurrentSnapshot reconciles the current snapshot owned by the VM.
func ReconcileCurrentSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vcVM *object.VirtualMachine) error {

	logger := pkglog.FromContextOrDefault(vmCtx)

	// Find all VirtualMachineSnapshot objects that reference this VM
	vmSnapshots, err := getVirtualMachineSnapshotsForVM(vmCtx, k8sClient)
	if err != nil {
		return err
	}

	// If no snapshots found, nothing to do
	if len(vmSnapshots) == 0 {
		return nil
	}

	// Skip snapshot processing for VKS/TKG nodes
	if kubeutil.HasCAPILabels(vmCtx.VM.Labels) {
		logger.V(4).Info("Skipping snapshot for VKS node")
		return nil
	}

	// When FastDeploy is enabled, wait for disk promotion sync to be ready.
	// This is prerequisite for the VM disk registration checked in next step.
	if pkgcfg.FromContext(vmCtx).Features.FastDeploy {

		if vmCtx.VM.Spec.PromoteDisksMode != vmopv1.VirtualMachinePromoteDisksModeDisabled &&
			!pkgcnd.IsTrue(vmCtx.VM, vmopv1.VirtualMachineDiskPromotionSynced) {
			logger.Info("Skipping snapshot as required condition is not ready",
				"condition", vmopv1.VirtualMachineDiskPromotionSynced)
			return nil
		}
	}

	// When AllDisksArePVCs is enabled, wait for VM's disks to be registered.
	// This is because vSphere cannot register disks as PVCs if there's disk
	// delta caused by snapshot creation.
	if pkgcfg.FromContext(vmCtx).Features.AllDisksArePVCs {

		if !pkgcnd.IsTrue(vmCtx.VM, vmconfunmanagedvolsfil.Condition) {
			logger.Info("Skipping snapshot as required condition is not ready",
				"condition", vmconfunmanagedvolsfil.Condition)
			return nil
		}

		if !pkgcnd.IsTrue(vmCtx.VM, vmconfunmanagedvolsreg.Condition) {
			logger.Info("Skipping snapshot as required condition is not ready",
				"condition", vmconfunmanagedvolsreg.Condition)
			return nil
		}
	}

	// Sort snapshots by creation timestamp to ensure consistent ordering
	sort.Slice(vmSnapshots, func(i, j int) bool {
		return vmSnapshots[i].CreationTimestamp.Before(
			&vmSnapshots[j].CreationTimestamp)
	})

	// Find the first (oldest) snapshot that is not created
	// We process snapshots in chronological order, one at a time
	// Also calculate the total pending snapshots so we can requeue the
	// request to continue processing other snapshots.
	var snapshotToProcess *vmopv1.VirtualMachineSnapshot
	totalPendingSnapshots := 0
	for _, snapshot := range vmSnapshots {

		// Skip snapshots that are already created.
		if pkgcnd.IsTrue(
			&snapshot,
			vmopv1.VirtualMachineSnapshotCreatedCondition) {

			logger.V(4).Info("Skipping snapshot that is already created",
				"snapshotName", snapshot.Name)
			continue
		}

		if snapshot.DeletionTimestamp.IsZero() {
			totalPendingSnapshots++
		}

		// Found the first snapshot that needs processing
		if snapshotToProcess == nil {
			snapshotToProcess = &snapshot
		}
	}

	// If no snapshot needs processing, we're done
	if snapshotToProcess == nil {
		logger.Info("All snapshots are ready or being deleted")
		return nil
	}

	logger = logger.WithValues("snapshotName", snapshotToProcess.Name)

	// If the oldest snapshot is being deleted, wait for it to finish.
	if !snapshotToProcess.DeletionTimestamp.IsZero() {
		logger.V(4).Info("Snapshot is being deleted, waiting for completion")
		return nil
	}

	// If the oldest snapshot is not owned by this VM, wait for the
	// owner reference to be added.  The owner ref will requeue a reconcile.
	owner, err := controllerutil.HasOwnerReference(
		snapshotToProcess.OwnerReferences,
		vmCtx.VM,
		k8sClient.Scheme())
	if err != nil {
		return fmt.Errorf("failed to check if snapshot %q is owned by vm: %w",
			snapshotToProcess.Name, err)
	}

	if !owner {
		logger.V(4).Info(
			"Snapshot is not owned by the VM, waiting for OwnerRef update")
		return nil
	}

	// If the oldest snapshot is in progress, wait for it to complete
	if pkgcnd.GetReason(
		snapshotToProcess,
		vmopv1.VirtualMachineSnapshotReadyCondition) ==
		vmopv1.VirtualMachineSnapshotCreationInProgressReason {

		logger.V(4).Info("Snapshot is in progress, waiting for completion")
		return nil
	}

	// Mark the snapshot as in progress before starting the operation to
	// prevent concurrent snapshots.
	if err := markSnapshotInProgress(vmCtx, k8sClient, snapshotToProcess); err != nil {
		return fmt.Errorf("failed to mark snapshot %q in progress: %w",
			snapshotToProcess.Name, err)
	}

	snapArgs := virtualmachine.SnapshotArgs{
		VMCtx:      vmCtx,
		VcVM:       vcVM,
		VMSnapshot: *snapshotToProcess,
	}

	logger.Info("Creating snapshot on vSphere")
	snapNode, err := virtualmachine.SnapshotVirtualMachine(snapArgs)
	if err != nil {
		// Mark the snapshot as failed and clear in-progress status
		// TODO: we wait for in-progress snapshots to complete when
		// taking a snapshot. So, if we are unable to clear the
		// in-progress condition because status patching fails, we
		// will forever be waiting.
		if err := markSnapshotFailed(vmCtx, k8sClient, snapshotToProcess, err); err != nil {
			return fmt.Errorf("failed to mark snapshot as failed: %w", err)
		}
		return fmt.Errorf("failed to create snapshot %q: %w",
			snapshotToProcess.Name, err)
	}

	// Update the snapshot status with the successful result
	if err = kubeutil.PatchSnapshotSuccessStatus(
		vmCtx,
		logger,
		k8sClient,
		snapshotToProcess,
		snapNode,
		vmCtx.VM.Spec.PowerState); err != nil {

		return fmt.Errorf("failed to update snapshot %q status: %w",
			snapshotToProcess.Name, err)
	}

	// Successfully processed one snapshot
	// Check if there are more snapshots to process after this one
	totalPendingSnapshots--
	logger.Info("Successfully completed snapshot operation",
		"remainingSnapshots", totalPendingSnapshots)

	// If there are more pending snapshots, requeue to process the next one.
	if totalPendingSnapshots > 0 {
		return pkgerr.RequeueError{
			Message: fmt.Sprintf(
				"requeuing to process %d remaining snapshots",
				totalPendingSnapshots),
		}
	}

	return nil
}

// unmarshalAndConvertVMFromYAML unmarshals VM YAML from snapshot and converts it
// to the latest version of VirtualMachine.
func (vs *vSphereVMProvider) unmarshalAndConvertVMFromYAML(
	vmYAML string) (*vmopv1.VirtualMachine, error) {

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	unstructuredObj := &unstructured.Unstructured{}
	if _, _, err := decUnstructured.Decode([]byte(vmYAML), nil, unstructuredObj); err != nil {
		return nil, fmt.Errorf("failed to decode VM YAML in to Unstructured object: %w", err)
	}

	// Convert it to newest version of VirtualMachine.
	vm := &vmopv1.VirtualMachine{}
	if err := vs.k8sClient.Scheme().Convert(unstructuredObj, vm, nil); err != nil {
		return nil, fmt.Errorf("failed to convert VM YAML to VirtualMachine: %w. "+
			"currentAPIVersion: %s, desiredAPIVersion: %s",
			err, unstructuredObj.GetAPIVersion(), vmopv1.GroupVersion.String())
	}

	return vm, nil
}
