// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package volumebatch

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// reconcileVMOwnedAttachVolumes processes all BA status volumes that carry
// the Reconfig attach method signal set by CSI. For each such volume it drives
// the ReconfigVM disk-add, transitions the CsiVolumeInfo to VM_MANAGED, and
// labels the PVC as vm-owned.
//
// This function only runs when the VMOwnedVolumes feature gate is enabled and
// the VM has the VM-owned storage annotation.
func (r *Reconciler) reconcileVMOwnedAttachVolumes(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
) error {
	if ba == nil {
		return nil
	}
	if !pkgcfg.FromContext(ctx).Features.VMOwnedVolumes {
		return nil
	}
	if !vmopv1util.IsVMOwnedStorageVM(ctx.VM) {
		return nil
	}

	logger := pkglog.FromContextOrDefault(ctx)
	var firstErr error

	for _, volStatus := range ba.Status.VolumeStatus {
		if !hasAttachMethodReconfig(volStatus) {
			continue
		}

		pvcStatus := volStatus.PersistentVolumeClaim
		if pvcStatus.DiskPath == "" || pvcStatus.DiskUUID == "" {
			logger.Info("Skipping VM-owned attach: diskPath or diskUUID missing",
				"volumeName", volStatus.Name,
				"diskPath", pvcStatus.DiskPath,
				"diskUUID", pvcStatus.DiskUUID)
			continue
		}

		logger.Info("Processing VM-owned attach for volume",
			"volumeName", volStatus.Name,
			"diskPath", pvcStatus.DiskPath,
			"diskUUID", pvcStatus.DiskUUID)

		if err := r.reconcileVMOwnedAttach(ctx, ba, volStatus); err != nil {
			logger.Error(err, "Failed VM-owned attach for volume",
				"volumeName", volStatus.Name)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// reconcileVMOwnedAttach handles the ReconfigVM add + CVI state transition
// + PVC label update for a single volume whose attach method is Reconfig.
func (r *Reconciler) reconcileVMOwnedAttach(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volStatus cnsv1alpha1.VolumeStatus,
) error {
	logger := pkglog.FromContextOrDefault(ctx)
	pvcStatus := volStatus.PersistentVolumeClaim
	diskPath := pvcStatus.DiskPath
	diskUUID := pvcStatus.DiskUUID
	pvcName := pvcStatus.ClaimName

	// Look up the CVI for this volume via the deterministic name.
	pvc, pv, err := r.getPVCAndPV(ctx, pvcName)
	if err != nil {
		return err
	}

	cvi, err := vmopv1util.GetCVIByVolumeID(ctx, r.Client, ctx.VM.Namespace, pv.Spec.CSI.VolumeHandle)
	if err != nil {
		return fmt.Errorf("failed to get CsiVolumeInfo for volume %s: %w", pvcName, err)
	}
	if cvi == nil {
		// CVI absent means this is not a VM-owned volume.
		logger.Info("CsiVolumeInfo not found; skipping VM-owned attach",
			"pvcName", pvcName)
		return nil
	}

	// Only proceed if the CVI is in the TRANSFERRING_TO_VM transient state
	// set by CSI.
	if cvi.Status.OwnershipState != cnsv1alpha1.OwnershipStateTransferringToVM {
		logger.Info("CsiVolumeInfo not in expected state for VM-owned attach",
			"pvcName", pvcName,
			"ownershipState", cvi.Status.OwnershipState)
		return nil
	}

	// Check idempotency: if the disk is already on the VM, skip ReconfigVM.
	existing, err := r.VMProvider.GetVirtualDiskByUUID(ctx, ctx.VM, diskUUID)
	if err != nil {
		return fmt.Errorf("failed to check disk presence on VM %s: %w", ctx.VM.Name, err)
	}

	if existing == nil {
		// Disk is not yet on the VM: build controller placement from the BA
		// spec entry and call AddExistingDiskToVM.
		controllerKey, unitNumber, diskMode, err := r.lookupVolumeControllerPlacement(ba, volStatus.Name)
		if err != nil {
			return fmt.Errorf("failed to look up controller placement for volume %s: %w",
				volStatus.Name, err)
		}

		logger.Info("Calling ReconfigVM to add existing disk",
			"vmName", ctx.VM.Name,
			"diskPath", diskPath,
			"controllerKey", controllerKey,
			"unitNumber", unitNumber)

		if err := r.VMProvider.AddExistingDiskToVM(ctx, ctx.VM, diskPath,
			controllerKey, unitNumber, diskMode); err != nil {
			return fmt.Errorf("ReconfigVM to add disk %s to VM %s failed: %w",
				diskPath, ctx.VM.Name, err)
		}

		// Re-read to get the live diskPath after attachment.
		existing, err = r.VMProvider.GetVirtualDiskByUUID(ctx, ctx.VM, diskUUID)
		if err != nil {
			return fmt.Errorf("failed to verify disk %s on VM %s after add: %w",
				diskUUID, ctx.VM.Name, err)
		}
		if existing == nil {
			return fmt.Errorf("disk %s not found on VM %s after ReconfigVM add",
				diskUUID, ctx.VM.Name)
		}
	} else {
		logger.Info("Disk already present on VM; skipping ReconfigVM add",
			"diskUUID", diskUUID, "vmName", ctx.VM.Name)
	}

	// Transition CVI from TRANSFERRING_TO_VM to VM_MANAGED.
	cviPatch := client.MergeFrom(cvi.DeepCopy())
	cvi.Status.OwnershipState = cnsv1alpha1.OwnershipStateVMManaged
	cvi.Status.DiskPath = existing.DiskPath
	logger.Info("Transitioning CsiVolumeInfo to VM_MANAGED",
		"cviName", cvi.Name,
		"diskUUID", diskUUID,
		"diskPath", existing.DiskPath)
	if err := r.Client.Status().Patch(ctx, cvi, cviPatch); err != nil {
		return fmt.Errorf("failed to patch CsiVolumeInfo %s to VM_MANAGED: %w",
			cvi.Name, err)
	}

	// Label the PVC as vm-owned.
	if err := r.labelPVCVolumeOwnership(ctx, pvc, pkgconst.PVCOwnershipVMOwned); err != nil {
		return fmt.Errorf("failed to label PVC %s as vm-owned: %w", pvcName, err)
	}

	// Set BA condition ReconfigCompleted=True to signal CSI that vm-operator
	// completed the disk add.
	if err := r.setReconfigCompletedCondition(ctx, ba, volStatus.Name); err != nil {
		// Non-fatal: the CVI is already VM_MANAGED, so the attach is durable.
		logger.Error(err, "Failed to set ReconfigCompleted condition on BA",
			"volumeName", volStatus.Name)
	}

	logger.Info("VM-owned attach completed",
		"vmName", ctx.VM.Name,
		"pvcName", pvcName,
		"diskUUID", diskUUID)
	return nil
}

// hasAttachMethodReconfig returns true when the BA VolumeStatus has an
// AttachMethod condition with reason Reconfig set by CSI.
func hasAttachMethodReconfig(vs cnsv1alpha1.VolumeStatus) bool {
	for _, cond := range vs.PersistentVolumeClaim.Conditions {
		if cond.Type == cnsv1alpha1.ConditionAttachMethod &&
			cond.Reason == cnsv1alpha1.ReasonReconfig {
			return true
		}
	}
	return false
}

// lookupVolumeControllerPlacement finds the controllerKey, unitNumber, and
// diskMode for a volume by name from the BA spec.
func (r *Reconciler) lookupVolumeControllerPlacement(
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volName string,
) (controllerKey, unitNumber int32, diskMode string, err error) {
	for _, vol := range ba.Spec.Volumes {
		if vol.Name == volName {
			if vol.PersistentVolumeClaim.ControllerKey == nil ||
				vol.PersistentVolumeClaim.UnitNumber == nil {
				return 0, 0, "", fmt.Errorf(
					"volume %s is missing controllerKey or unitNumber in BA spec", volName)
			}
			return *vol.PersistentVolumeClaim.ControllerKey,
				*vol.PersistentVolumeClaim.UnitNumber,
				string(vol.PersistentVolumeClaim.DiskMode),
				nil
		}
	}
	return 0, 0, "", fmt.Errorf("volume %s not found in BA spec", volName)
}

// getPVCAndPV fetches both the PVC and its bound PV. Used to derive the
// CNS volumeHandle for deterministic CVI lookups.
func (r *Reconciler) getPVCAndPV(
	ctx *pkgctx.VolumeContext,
	pvcName string,
) (*corev1.PersistentVolumeClaim, *corev1.PersistentVolume, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: ctx.VM.Namespace,
		Name:      pvcName,
	}, pvc); err != nil {
		return nil, nil, fmt.Errorf("failed to get PVC %s: %w", pvcName, err)
	}

	if pvc.Spec.VolumeName == "" {
		return nil, nil, fmt.Errorf("PVC %s is not yet bound to a PV", pvcName)
	}

	pv := &corev1.PersistentVolume{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name: pvc.Spec.VolumeName,
	}, pv); err != nil {
		return nil, nil, fmt.Errorf("failed to get PV %s for PVC %s: %w",
			pvc.Spec.VolumeName, pvcName, err)
	}

	if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeHandle == "" {
		return nil, nil, fmt.Errorf("PV %s has no CSI volume handle", pv.Name)
	}

	return pvc, pv, nil
}

// labelPVCVolumeOwnership patches the PVC's volume-ownership label to the
// given value.
func (r *Reconciler) labelPVCVolumeOwnership(
	ctx *pkgctx.VolumeContext,
	pvc *corev1.PersistentVolumeClaim,
	ownershipValue string,
) error {
	patch := client.MergeFrom(pvc.DeepCopy())
	if pvc.Labels == nil {
		pvc.Labels = make(map[string]string)
	}
	pvc.Labels[pkgconst.PVCVolumeOwnershipLabel] = ownershipValue
	return r.Client.Patch(ctx, pvc, patch)
}

// setReconfigCompletedCondition patches BA status to add a
// ReconfigCompleted=True condition for the given volume.
func (r *Reconciler) setReconfigCompletedCondition(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volName string,
) error {
	baPatch := client.MergeFrom(ba.DeepCopy())

	for i := range ba.Status.VolumeStatus {
		if ba.Status.VolumeStatus[i].Name == volName {
			conditions := ba.Status.VolumeStatus[i].PersistentVolumeClaim.Conditions
			// Remove any existing ReconfigCompleted entry.
			for j, c := range conditions {
				if c.Type == cnsv1alpha1.ConditionReconfigCompleted {
					conditions = append(conditions[:j], conditions[j+1:]...)
					break
				}
			}
			conditions = append(conditions, metav1.Condition{
				Type:               cnsv1alpha1.ConditionReconfigCompleted,
				Status:             metav1.ConditionTrue,
				Reason:             "DiskAdded",
				LastTransitionTime: metav1.Now(),
			})
			ba.Status.VolumeStatus[i].PersistentVolumeClaim.Conditions = conditions
			break
		}
	}

	if err := r.Client.Status().Patch(ctx, ba, baPatch); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to set ReconfigCompleted on BA %s: %w", ba.Name, err)
	}
	return nil
}

// updateVMStatusForVMOwnedAttach sets the vm.status.volumes entry for a
// VM-owned volume that is now VM-owned.
func updateVMStatusForVMOwnedAttach(
	vm *vmopv1.VirtualMachine,
	volName, diskUUID string,
) {
	for i := range vm.Status.Volumes {
		if vm.Status.Volumes[i].Name == volName {
			vm.Status.Volumes[i].Attached = true
			vm.Status.Volumes[i].DiskUUID = diskUUID
			vm.Status.Volumes[i].Type = vmopv1.VolumeTypeManaged
			vm.Status.Volumes[i].Error = ""
			return
		}
	}
}

// reconcileVMOwnedDetachVolumes handles the normal detach path for
// VMs using the VM-owned storage path. It is called with the set of volume names that are being
// removed from the BA spec (volumes the user removed from vm.spec.volumes).
//
// For each volume in the removal set that belongs to a VM-owned CVI, it:
//  1. Refreshes the CVI diskPath from the live VM device.
//  2. Transitions the CVI to TRANSFERRING_TO_CSI.
//  3. Removes the disk from the VM via ReconfigVM (VMDK file is preserved).
//
// On ReconfigVM failure, the CVI is reverted to VM_MANAGED, the error is
// surfaced on vm.status.volumes, and a DetachBlocked condition is set on the
// BA so the caller can retry with backoff.
//
// Returns an error when any detach fails so that the BA spec update that would
// remove the volume is skipped — the volume stays in the BA spec until it can
// actually be removed from the VM.
func (r *Reconciler) reconcileVMOwnedDetachVolumes(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	removedVolumeNames []string,
) error {
	if ba == nil || len(removedVolumeNames) == 0 {
		return nil
	}
	if !pkgcfg.FromContext(ctx).Features.VMOwnedVolumes {
		return nil
	}
	if !vmopv1util.IsVMOwnedStorageVM(ctx.VM) {
		return nil
	}

	logger := pkglog.FromContextOrDefault(ctx)
	var firstErr error

	for _, volName := range removedVolumeNames {
		pvcName := r.pvcNameFromBASpec(ba, volName)
		if pvcName == "" {
			continue
		}

		logger.Info("Processing VM-owned detach for volume",
			"volumeName", volName, "pvcName", pvcName, "vmName", ctx.VM.Name)

		if err := r.reconcileVMOwnedDetach(ctx, ba, volName, pvcName); err != nil {
			logger.Error(err, "Failed VM-owned detach for volume",
				"volumeName", volName)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// reconcileVMOwnedDetach handles the C.2 → C.3 → C.4 sequence for a
// single volume being detached from a a VM using the VM-owned storage path.
func (r *Reconciler) reconcileVMOwnedDetach(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volName, pvcName string,
) error {
	logger := pkglog.FromContextOrDefault(ctx)

	pvc, pv, err := r.getPVCAndPV(ctx, pvcName)
	if err != nil {
		return err
	}

	cvi, err := vmopv1util.GetCVIByVolumeID(ctx, r.Client, ctx.VM.Namespace, pv.Spec.CSI.VolumeHandle)
	if err != nil {
		return fmt.Errorf("failed to get CsiVolumeInfo for PVC %s: %w", pvcName, err)
	}
	if cvi == nil {
		logger.Info("CsiVolumeInfo not found; using legacy detach path",
			"pvcName", pvcName)
		return nil
	}

	// Only handle VM-owned volumes that are currently VM_MANAGED by this VM.
	switch cvi.Status.OwnershipState {
	case cnsv1alpha1.OwnershipStateVMManaged:
		if cvi.Status.VMName != ctx.VM.Name {
			return nil
		}
	case cnsv1alpha1.OwnershipStateTransferringToCSI:
		// A previous reconcile started the detach; continue from step C.3 if
		// the disk is still on the VM.
	default:
		return nil
	}

	// C.2 — Refresh diskPath from live VM device before removal.
	diskInfo, err := r.VMProvider.GetVirtualDiskByUUID(ctx, ctx.VM, cvi.Status.DiskUUID)
	if err != nil {
		return fmt.Errorf("failed to look up disk %s on VM %s: %w",
			cvi.Status.DiskUUID, ctx.VM.Name, err)
	}

	if diskInfo != nil {
		// Patch the CVI diskPath while the disk is still reachable.
		if diskInfo.DiskPath != cvi.Status.DiskPath {
			cviPatch := client.MergeFrom(cvi.DeepCopy())
			cvi.Status.DiskPath = diskInfo.DiskPath
			if err := r.Client.Status().Patch(ctx, cvi, cviPatch); err != nil {
				return fmt.Errorf("failed to refresh CsiVolumeInfo diskPath for %s: %w",
					cvi.Name, err)
			}
		}
	} else if cvi.Status.OwnershipState == cnsv1alpha1.OwnershipStateTransferringToCSI {
		// Disk already absent; ReconfigVM must have succeeded in a previous
		// pass. Skip to the CVI state update below.
		logger.Info("Disk already gone from VM; skipping ReconfigVM remove",
			"diskUUID", cvi.Status.DiskUUID, "vmName", ctx.VM.Name)
		return r.finalizeVMOwnedDetach(ctx, ba, cvi, pvc, volName)
	}

	// C.3 — Transition CVI to TRANSFERRING_TO_CSI.
	if cvi.Status.OwnershipState == cnsv1alpha1.OwnershipStateVMManaged {
		cviPatch := client.MergeFrom(cvi.DeepCopy())
		cvi.Status.OwnershipState = cnsv1alpha1.OwnershipStateTransferringToCSI
		logger.Info("Transitioning CsiVolumeInfo to TRANSFERRING_TO_CSI",
			"cviName", cvi.Name, "vmName", ctx.VM.Name)
		if err := r.Client.Status().Patch(ctx, cvi, cviPatch); err != nil {
			return fmt.Errorf("failed to patch CsiVolumeInfo %s to TRANSFERRING_TO_CSI: %w",
				cvi.Name, err)
		}
	}

	// Remove the disk from the VM, preserving the VMDK file.
	logger.Info("Calling ReconfigVM to remove disk from VM",
		"diskUUID", cvi.Status.DiskUUID, "vmName", ctx.VM.Name)
	if err := r.VMProvider.RemoveDiskFromVM(ctx, ctx.VM, cvi.Status.DiskUUID); err != nil {
		// C.3 failure — revert CVI to VM_MANAGED so the volume is still tracked.
		logger.Error(err, "ReconfigVM disk remove failed; reverting CVI to VM_MANAGED",
			"diskUUID", cvi.Status.DiskUUID, "vmName", ctx.VM.Name)

		revertPatch := client.MergeFrom(cvi.DeepCopy())
		cvi.Status.OwnershipState = cnsv1alpha1.OwnershipStateVMManaged
		if patchErr := r.Client.Status().Patch(ctx, cvi, revertPatch); patchErr != nil {
			logger.Error(patchErr, "Failed to revert CsiVolumeInfo to VM_MANAGED",
				"cviName", cvi.Name)
		}

		// Surface error in vm.status.volumes.
		for i := range ctx.VM.Status.Volumes {
			if ctx.VM.Status.Volumes[i].Name == volName {
				ctx.VM.Status.Volumes[i].Error = fmt.Sprintf("detach failed: %v", err)
				break
			}
		}

		// Set BA-level condition to signal the detach is blocked.
		if setErr := r.setDetachBlockedCondition(ctx, ba, volName, err.Error()); setErr != nil {
			logger.Error(setErr, "Failed to set DetachBlocked condition on BA")
		}

		return fmt.Errorf("failed to remove disk %s from VM %s: %w",
			cvi.Status.DiskUUID, ctx.VM.Name, err)
	}

	return r.finalizeVMOwnedDetach(ctx, ba, cvi, pvc, volName)
}

// finalizeVMOwnedDetach completes the post-ReconfigVM bookkeeping: clears
// the DetachBlocked condition if set and removes the volume from the BA spec.
func (r *Reconciler) finalizeVMOwnedDetach(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	cvi *cnsv1alpha1.CsiVolumeInfo,
	pvc *corev1.PersistentVolumeClaim,
	volName string,
) error {
	logger := pkglog.FromContextOrDefault(ctx)

	// Clear any DetachBlocked condition on the BA.
	if err := r.clearDetachBlockedCondition(ctx, ba, volName); err != nil {
		logger.Error(err, "Failed to clear DetachBlocked condition", "volumeName", volName)
	}

	logger.Info("VM-owned detach completed for volume; CSI will complete re-registration",
		"volumeName", volName, "cviName", cvi.Name, "vmName", ctx.VM.Name)
	return nil
}

// setDetachBlockedCondition patches the BA CR-level condition to signal that
// detach of one or more volumes failed.
func (r *Reconciler) setDetachBlockedCondition(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	volName, message string,
) error {
	baPatch := client.MergeFrom(ba.DeepCopy())

	newCond := metav1.Condition{
		Type:               cnsv1alpha1.ConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             cnsv1alpha1.ReasonDetachBlocked,
		Message:            fmt.Sprintf("Cannot detach volume %s: %s", volName, message),
		LastTransitionTime: metav1.Now(),
	}
	// Replace any existing Ready condition.
	for i, c := range ba.Status.Conditions {
		if c.Type == cnsv1alpha1.ConditionReady {
			ba.Status.Conditions[i] = newCond
			if err := r.Client.Status().Patch(ctx, ba, baPatch); err != nil {
				return fmt.Errorf("failed to patch BA DetachBlocked condition: %w", err)
			}
			return nil
		}
	}
	ba.Status.Conditions = append(ba.Status.Conditions, newCond)
	if err := r.Client.Status().Patch(ctx, ba, baPatch); err != nil {
		return fmt.Errorf("failed to patch BA DetachBlocked condition: %w", err)
	}
	return nil
}

// clearDetachBlockedCondition removes the DetachBlocked condition from the BA
// once a detach succeeds.
func (r *Reconciler) clearDetachBlockedCondition(
	ctx *pkgctx.VolumeContext,
	ba *cnsv1alpha1.CnsNodeVMBatchAttachment,
	_ string,
) error {
	for i, c := range ba.Status.Conditions {
		if c.Type == cnsv1alpha1.ConditionReady &&
			c.Reason == cnsv1alpha1.ReasonDetachBlocked {
			baPatch := client.MergeFrom(ba.DeepCopy())
			ba.Status.Conditions = append(ba.Status.Conditions[:i], ba.Status.Conditions[i+1:]...)
			if err := r.Client.Status().Patch(ctx, ba, baPatch); err != nil {
				return fmt.Errorf("failed to clear DetachBlocked condition: %w", err)
			}
			break
		}
	}
	return nil
}

// pvcNameFromBASpec finds the PVC claim name for the given volume name in the
// BA spec.
func (r *Reconciler) pvcNameFromBASpec(ba *cnsv1alpha1.CnsNodeVMBatchAttachment, volName string) string {
	for _, vol := range ba.Spec.Volumes {
		if vol.Name == volName {
			return vol.PersistentVolumeClaim.ClaimName
		}
	}
	return ""
}
