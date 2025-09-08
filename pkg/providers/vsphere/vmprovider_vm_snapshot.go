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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	k8syaml "sigs.k8s.io/yaml"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	backupapi "github.com/vmware-tanzu/vm-operator/pkg/backup/api"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// DeleteSnapshot deletes the snapshot from the VM.
// The boolean indicating if the VM associated is deleted.
func (vs *vSphereVMProvider) DeleteSnapshot(
	ctx context.Context,
	vmSnapshot *vmopv1.VirtualMachineSnapshot,
	vm *vmopv1.VirtualMachine,
	removeChildren bool,
	consolidate *bool) (bool, error) {

	logger := pkglog.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
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

	logger := pkglog.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
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

	if err := vcVM.Properties(
		ctx, vcVM.Reference(),
		[]string{"snapshot", "layoutEx", "config.hardware.device"},
		&vmCtx.MoVM); err != nil {
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
	logger := pkglog.FromContextOrDefault(ctx).WithValues("vmName", vm.NamespacedName())
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
		return fmt.Errorf("failed to get snapshot: %w", err)
	}

	// Sync current and root snapshots status of the VM
	return vmlifecycle.SyncVMSnapshotTreeStatus(vmCtx, vs.k8sClient)
}

// markSnapshotInProgress marks a snapshot as currently being processed.
func (vs *vSphereVMProvider) markSnapshotInProgress(
	vmCtx pkgctx.VirtualMachineContext,
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
	if err := vs.k8sClient.Status().Patch(vmCtx, vmSnapshot, patch); err != nil {
		return fmt.Errorf("failed to mark snapshot as in progress: %w", err)
	}

	vmCtx.Logger.V(4).Info("Marked snapshot as in progress", "snapshotName", vmSnapshot.Name)
	return nil
}

// markSnapshotFailed marks a snapshot as failed and clears the in-progress status.
func (vs *vSphereVMProvider) markSnapshotFailed(vmCtx pkgctx.VirtualMachineContext,
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
	if updateErr := vs.k8sClient.Status().Patch(vmCtx, vmSnapshot, patch); updateErr != nil {
		return fmt.Errorf("failed to update snapshot status after failure: %w", updateErr)
	}

	vmCtx.Logger.V(4).Info("Marked snapshot as failed", "snapshotName", vmSnapshot.Name, "error", err.Error())
	return nil
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
	if vmCtx.VM.Spec.CurrentSnapshot == nil {
		logger.V(4).Info("Skipping revert for empty spec.currentSnapshot")
		return nil
	}

	desiredSnapshotName := vmCtx.VM.Spec.CurrentSnapshot.Name
	logger = logger.WithValues(
		"desiredSnapshotName",
		vmCtx.VM.Spec.CurrentSnapshot.Name)
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
	ref, err := virtualmachine.FindSnapshot(vmCtx, desiredSnapshotName)
	if err != nil || ref == nil {
		err := fmt.Errorf("failed to find vSphere snapshot %q: %w",
			desiredSnapshotName, err)

		pkgcnd.MarkError(vmCtx.VM,
			vmopv1.VirtualMachineSnapshotRevertSucceeded,
			vmopv1.VirtualMachineSnapshotRevertFailedReason,
			err,
		)

		return err
	}

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

	// Restore VM spec and metadata from the snapshot.
	logger.Info("Starting VM spec and metadata restoration from snapshot",
		"beforeRestoreAnnotations", vmCtx.VM.Annotations,
		"beforeRestoreLabels", vmCtx.VM.Labels,
		"beforeRestoreSpecCurrentSnapshot", vmCtx.VM.Spec.CurrentSnapshot)

	if err := vs.restoreVMSpecFromSnapshot(
		vmCtx, vcVM, obj, ref); err != nil {

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
		"afterRestoreSpecCurrentSnapshot", vmCtx.VM.Spec.CurrentSnapshot)

	logger.Info("Successfully completed reverted snapshot")

	// Return immediately after successful revert and spec restoration
	// to trigger requeue. This ensures the next reconcile operates on
	// the updated VM spec, labels, and annotations. The VM status
	// will be updated in the subsequent reconcile loop.
	return ErrSnapshotRevert
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
	snapObj *vimtypes.ManagedObjectReference) error {

	vmYAML, err := vs.getVMYamlFromSnapshot(vmCtx, vcVM, snapCR, snapObj)
	if err != nil {
		return fmt.Errorf("failed to parse VM spec from snapshot: %w", err)
	}

	// Handle cases where the snapshot doesn't contain any VM spec and metadata.
	if vmYAML == "" {
		return fmt.Errorf("no VM YAML in snapshot config")
	}

	vmCtx.Logger.V(5).Info("Restoring VM spec from snapshot", "vmYAML", vmYAML)
	// Unmarshal and convert VM from yaml in snapshot based on API version
	vm, err := vs.unmarshalAndConvertVMFromYAML(vmCtx, vmYAML)
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
	vmCtx.VM.Spec.CurrentSnapshot = nil

	// TODO: AKP: We need to merge (and skip) some labels so that we
	// are not simply reverting to the previous labels. Otherwise, we
	// risk the VM being impacted after a snapshot revert.
	vmCtx.VM.Labels = maps.Clone(vm.Labels)
	vmCtx.VM.Annotations = maps.Clone(vm.Annotations)
	// Empty out the status.
	vmCtx.VM.Status = vmopv1.VirtualMachineStatus{}

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
	snapRef *vimtypes.ManagedObjectReference) (string, error) {

	var (
		moSnap mo.VirtualMachineSnapshot
		logger = pkglog.FromContextOrDefault(vmCtx)
	)

	// Fetch the config stored with the snapshot.
	if err := vcVM.Properties(
		vmCtx, *snapRef, []string{"config"}, &moSnap); err != nil {

		return "", fmt.Errorf("failed to fetch snapshot config: %w", err)
	}

	logger.V(4).Info("Successfully fetched snapshot properties")

	ecList := object.OptionValueList(moSnap.Config.ExtraConfig)
	raw, _ := ecList.GetString(backupapi.VMResourceYAMLExtraConfigKey)
	if raw == "" {
		// If the Snapshot has ImportedSnapshotAnnotation, then approximate the
		// VM spec.
		return getVMYamlFromSnapshotFromSynthesizedSpec(
			vmCtx, snap, moSnap, snapRef)
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
	moSnap mo.VirtualMachineSnapshot,
	snap *vimtypes.ManagedObjectReference) (string, error) {

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

	// Inferred from the power state of the snapshot
	// It doesn't matter what the power state of the VM is at the time of
	// revert.
	// The VM is always reverted in the same power state as the snapshot.
	snapshot := vmlifecycle.FindSnapshotInTree(
		vmCtx.MoVM.Snapshot.RootSnapshotList, snap.Value)
	if snapshot == nil {
		// weird situation here
		// the snapshot being processed doesn't exist in the snapshot tree
		// this shouldn't be happening
		return "", fmt.Errorf(
			"failed to find snapshot %q in tree", snap.Value)
	}

	// Convert from vimtypes.VirtualMachinePowerState to
	// vmopv1.VirtualMachinePowerState
	if snapshot.State == vimtypes.VirtualMachinePowerStatePoweredOn {
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOn
	} else {
		vm.Spec.PowerState = vmopv1.VirtualMachinePowerStateOff
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
		CurrentSnapshot: nil,

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

func (vs *vSphereVMProvider) reconcileCurrentSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	logger := pkglog.FromContextOrDefault(vmCtx)

	// Find all VirtualMachineSnapshot objects that reference this VM
	vmSnapshots, err := vs.getVirtualMachineSnapshotsForVM(vmCtx)
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
		vs.k8sClient.Scheme())
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
	if err := vs.markSnapshotInProgress(vmCtx, snapshotToProcess); err != nil {
		return fmt.Errorf("failed to mark snapshot %q in progress: %w",
			snapshotToProcess.Name, err)
	}

	snapArgs := virtualmachine.SnapshotArgs{
		VMCtx:      vmCtx,
		VcVM:       vcVM,
		VMSnapshot: *snapshotToProcess,
	}

	logger.Info("Creating snapshot on vSphere")
	snapMoRef, err := virtualmachine.SnapshotVirtualMachine(snapArgs)
	if err != nil {
		// Mark the snapshot as failed and clear in-progress status
		// TODO: we wait for in-progress snapshots to complete when
		// taking a snapshot. So, if we are unable to clear the
		// in-progress condition because status patching fails, we
		// will forever be waiting.
		if err := vs.markSnapshotFailed(vmCtx, snapshotToProcess, err); err != nil {
			return fmt.Errorf("failed to mark snapshot as failed: %w", err)
		}
		return fmt.Errorf("failed to create snapshot %q: %w",
			snapshotToProcess.Name, err)
	}

	// Update the snapshot status with the successful result
	if err = kubeutil.PatchSnapshotSuccessStatus(
		vmCtx,
		vs.k8sClient,
		snapshotToProcess,
		snapMoRef,
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
	vmCtx pkgctx.VirtualMachineContext,
	vmYAML string) (*vmopv1.VirtualMachine, error) {

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	unstructuredObj := &unstructured.Unstructured{}
	if _, _, err := decUnstructured.Decode([]byte(vmYAML), nil, unstructuredObj); err != nil {
		vmCtx.Logger.Error(err, "failed to decode VM YAML in to Unstructured object")
		return nil, err
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
