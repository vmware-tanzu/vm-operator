// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcnd "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

// CalculateReservedForSnapshotPerStorageClass calculates the reserved capacity for a snapshot.
// And return the requested capacity categorized by each storage class.
// 1. Space required for classic disks.
// 2. Space required for VM's memory.
// 3. Space required for each PVC.
func CalculateReservedForSnapshotPerStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	logger logr.Logger,
	vmSnapshot vmopv1.VirtualMachineSnapshot) ([]vmopv1.VirtualMachineSnapshotStorageStatusRequested, error) {

	if vmSnapshot.Spec.VMRef == nil {
		return nil, fmt.Errorf("vmRef is not set")
	}

	vm := &vmopv1.VirtualMachine{}
	if err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSnapshot.Namespace, Name: vmSnapshot.Spec.VMRef.Name}, vm); err != nil {
		return nil, fmt.Errorf("failed to get VM %s: %w", vmSnapshot.Spec.VMRef.Name, err)
	}

	requestedMap := make(map[string]*resource.Quantity)

	// Add the VM's memory to the total reserved capacity.
	if vmSnapshot.Spec.Memory {
		// TODO(lubron): Fetch the requested memory size from the VM.status.hardware.memory.reservation.
		vmClass := &vmopv1.VirtualMachineClass{}
		if err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSnapshot.Namespace, Name: vm.Spec.ClassName}, vmClass); err != nil {
			return nil, fmt.Errorf("failed to get VMClass %s: %w", vm.Spec.ClassName, err)
		}

		if _, ok := requestedMap[vm.Spec.StorageClass]; !ok {
			requestedMap[vm.Spec.StorageClass] = resource.NewQuantity(0, resource.BinarySI)
		}

		logger.V(4).Info("adding memory size to the total reserved capacity", "memory", vmClass.Spec.Hardware.Memory, "storageClass", vm.Spec.StorageClass)
		requestedMap[vm.Spec.StorageClass].Add(vmClass.Spec.Hardware.Memory)
	}

	volumePVCMap := make(map[string]*vmopv1.PersistentVolumeClaimVolumeSource)
	for _, volume := range vm.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil {
			volumePVCMap[volume.Name] = volume.PersistentVolumeClaim
		}
	}

	// Calculate the total capacity of the Classic disks and PVCs included in the VM.
	for _, volume := range vm.Status.Volumes {
		switch volume.Type {
		case vmopv1.VolumeTypeClassic:
			// Add the disk size to the total reserved capacity
			if volume.Requested == nil {
				return nil, fmt.Errorf("requested is not set for classic disk")
			}

			if _, ok := requestedMap[vm.Spec.StorageClass]; !ok {
				requestedMap[vm.Spec.StorageClass] = resource.NewQuantity(0, resource.BinarySI)
			}

			logger.V(4).Info("adding disk size to the total reserved capacity", "disk", volume.Name,
				"storageClass", vm.Spec.StorageClass, "requested", volume.Requested)
			requestedMap[vm.Spec.StorageClass].Add(*volume.Requested)
		case vmopv1.VolumeTypeManaged:
			// For FCD, find the pvc of the volume, then check if pvc's storage class
			// matches the SPU's storage class.
			// If it doesn't match, skip the disk.
			// If it matches, add the PVC's requested capacity to the total reserved capacity.
			pvcSpec, ok := volumePVCMap[volume.Name]
			if !ok {
				// TODO (lubron): If if this case possible?
				logger.V(4).Info("skipping disk because corresponding pvc is not found in the vm.spec.volumes", "volume", volume.Name)
				continue
			}

			if pvcSpec.InstanceVolumeClaim != nil {
				// Short-cut the rest of the function since instance storage
				// volumes already have the requested size and the PVC does
				// not need to be fetched.
				if _, ok := requestedMap[pvcSpec.InstanceVolumeClaim.StorageClass]; !ok {
					requestedMap[pvcSpec.InstanceVolumeClaim.StorageClass] = resource.NewQuantity(0, resource.BinarySI)
				}

				logger.V(4).Info("adding disk size to the total reserved capacity", "disk", volume.Name,
					"storageClass", pvcSpec.InstanceVolumeClaim.StorageClass, "requested", pvcSpec.InstanceVolumeClaim.Size)
				requestedMap[pvcSpec.InstanceVolumeClaim.StorageClass].Add(pvcSpec.InstanceVolumeClaim.Size)
				continue
			}

			pvc := &corev1.PersistentVolumeClaim{}
			if err := k8sClient.Get(ctx, ctrlclient.ObjectKey{Namespace: vmSnapshot.Namespace, Name: pvcSpec.ClaimName}, pvc); err != nil {
				return nil, fmt.Errorf("failed to get pvc %s for volume %s: %w", pvcSpec.ClaimName, volume.Name, err)
			}

			if pvc.Spec.StorageClassName == nil {
				return nil, fmt.Errorf("storage class is not set for pvc %s", pvc.Name)
			}

			if pvc.Spec.Resources.Requests == nil || pvc.Spec.Resources.Requests.Storage() == nil {
				return nil, fmt.Errorf("requests is not set for pvc %s", pvc.Name)
			}

			if _, ok := requestedMap[*pvc.Spec.StorageClassName]; !ok {
				requestedMap[*pvc.Spec.StorageClassName] = resource.NewQuantity(0, resource.BinarySI)
			}

			volumeRequested := pvc.Spec.Resources.Requests.Storage()

			logger.V(4).Info("adding disk size to the total reserved capacity", "disk", volume.Name,
				"storageClass", *pvc.Spec.StorageClassName, "requested", volumeRequested)
			requestedMap[*pvc.Spec.StorageClassName].Add(*volumeRequested)
		default:
			return nil, fmt.Errorf("unsupported volume type %s", volume.Type)
		}
	}

	requested := make([]vmopv1.VirtualMachineSnapshotStorageStatusRequested, 0, len(requestedMap))
	for storageClass, total := range requestedMap {
		requested = append(requested, vmopv1.VirtualMachineSnapshotStorageStatusRequested{
			StorageClass: storageClass,
			Total:        total,
		})
	}
	return requested, nil
}

// PatchSnapshotSuccessStatus patches the snapshot status to reflect the success
// of the snapshot operation.
func PatchSnapshotSuccessStatus(
	ctx context.Context,
	logger logr.Logger,
	k8sClient ctrlclient.Client,
	snap *vmopv1.VirtualMachineSnapshot,
	snapNode *vimtypes.VirtualMachineSnapshotTree,
	vmPowerState vmopv1.VirtualMachinePowerState) error {

	snapPatch := ctrlclient.MergeFrom(snap.DeepCopy())
	snap.Status.UniqueID = snapNode.Snapshot.Reference().Value
	snap.Status.Quiesced = snap.Spec.Quiesce != nil
	snap.Status.PowerState = vmPowerState
	snap.Status.PowerState = vmopv1util.ConvertPowerState(logger, snapNode.State)

	pkgcnd.MarkTrue(snap, vmopv1.VirtualMachineSnapshotCreatedCondition)

	if err := k8sClient.Status().Patch(ctx, snap, snapPatch); err != nil {
		return fmt.Errorf(
			"failed to patch snapshot status resource %s/%s: err: %w",
			snap.Name, snap.Namespace, err)
	}

	return nil
}
