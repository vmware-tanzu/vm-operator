/* **********************************************************
 * Copyright 2019-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package providers

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
)

const (
	LoggerName                  = "virtualmachine-volume-provider"
	AttributeFirstClassDiskUUID = "diskUUID"
	VolumeOpAttach              = "attach volume"
	VolumeOpDetach              = "detach volume"
	VolumeOpUpdateStatus        = "update volume status"
)

var log = logf.Log.WithName(LoggerName)

type VolumeProviderInterface interface {
	// AttachVolumes(): The implementation of attaching volumes
	// Arguments:
	// * ctx context.Context:
	// * vm *vmoperatorv1alpha1.VirtualMachine: The VirtualMachine instance pointer to which the volume will be attached
	// * virtualMachineVolumesToAttach map[client.ObjectKey]bool: A set of object keys which indicates the virtual machine volumes to attach
	AttachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToAttach map[client.ObjectKey]bool) error
	// AttachVolumes(): The implementation of detaching volumes
	// Arguments:
	// * ctx context.Context:
	// * vm *vmoperatorv1alpha1.VirtualMachine: The VirtualMachine instance pointer from which the volume will be detached
	// * virtualMachineVolumesToDetach map[client.ObjectKey]bool: A set of object keys which indicates the virtual machine volumes to detach
	DetachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToDetach map[client.ObjectKey]bool) error
	// UpdateVmVolumesStatus(): The implementation of updating virtual machine volumes status
	// Arguments:
	// * ctx context.Context:
	// * vm *vmoperatorv1alpha1.VirtualMachine: The VirtualMachine instance pointer from which the virtual machine volumes status needs to be updated
	UpdateVmVolumesStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error
}

type cnsVolumeProvider struct {
	client client.Client
}

func CnsVolumeProvider(client client.Client) *cnsVolumeProvider {
	return &cnsVolumeProvider{client: client}
}

//TODO: CreateAttachments, DeleteCAttachments and UpdateVmVolumesStatus should return a slice of error: 
// CreateCnsNodeVmAttachments loop on the set of virtualMachineVolumesToAdd and create the CnsNodeVmAttachment instances accordingly
// Return error when fails to create CnsNodeVmAttachment instances (partially or completely)
// Note: AttachVolumes() does not call client.Status().Update(), it just updates vm object and vitualmachine_controller.go eventually
//       will call apiserver to update the vm object
func (cvp *cnsVolumeProvider) AttachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToAttach map[client.ObjectKey]bool) error {
	log := log.WithValues("vm", vm.NamespacedName())
	volumeOpErrors := newVolumeErrorsByOpType(VolumeOpAttach)
	for virtualMachineVolume := range virtualMachineVolumesToAttach {
		cnsNodeVmAttachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		cnsNodeVmAttachment.SetName(constructCnsNodeVmAttachmentName(vm.Name, virtualMachineVolume.Name))
		cnsNodeVmAttachment.SetNamespace(virtualMachineVolume.Namespace)
		cnsNodeVmAttachment.Spec.NodeUUID = vm.Status.BiosUUID
		// From  it puts volume name as pcvsc-<nodeuuid>
		// cnsNodeVmAttachment.Spec.VolumeName = "pvcsc-" + cnsNodeVmAttachment.Spec.NodeUUID
		cnsNodeVmAttachment.Spec.VolumeName = virtualMachineVolume.Name
		cnsNodeVmAttachment.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         "vmoperator.vmware.com/v1alpha1",
				Controller:         pointer.BoolPtr(true),
				BlockOwnerDeletion: pointer.BoolPtr(true),
				Kind:               "VirtualMachine",
				Name:               vm.Name,
				UID:                vm.UID,
			},
		}
		log.V(4).Info("Attempting to create the CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
		clientCreateError := cvp.client.Create(ctx, cnsNodeVmAttachment)
		if clientCreateError != nil {
			// Ignore if the CRD instance already exists
			if !apierrors.IsAlreadyExists(clientCreateError) {
				volumeOpErrors.add(virtualMachineVolume.Name, clientCreateError)
				log.Error(clientCreateError, "Unable to create CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
			}
		} else {
			log.Info("Created the CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
		}
		// Continue creating next CnsNodeVmAttachment, reconciliation will try to create those failed ones in the retry.
	}

	if volumeOpErrors.hasOccurred() {
		return volumeOpErrors
	}
	return nil
}

// This function loops on virtualMachineVolumesToDelete and delete the CnsNodeVmAttachment instance respectively
// Return error when fails to delete CnsNodeVmAttachment instances (partially or completely)
// Note: DetachVolumes() does not update the vm.Status.Volumes since it has been handled by UpdateVmVolumesStatus() by checking the existence of
//       respective CnsNodeVmAttachment instance
func (cvp *cnsVolumeProvider) DetachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToDetach map[client.ObjectKey]bool) error {
	log := log.WithValues("vm", vm.NamespacedName())
	volumeOpErrors := newVolumeErrorsByOpType(VolumeOpDetach)
	for virtualMachineVolumeToDelete := range virtualMachineVolumesToDetach {
		cnsNodeVmAttachmentToDelete := &cnsv1alpha1.CnsNodeVmAttachment{}
		cnsNodeVmAttachmentToDelete.SetName(constructCnsNodeVmAttachmentName(vm.Name, virtualMachineVolumeToDelete.Name))
		cnsNodeVmAttachmentToDelete.SetNamespace(virtualMachineVolumeToDelete.Namespace)
		log.Info("Attempting to delete the CnsNodeVmAttachment", "name", cnsNodeVmAttachmentToDelete.Name, "namespace", cnsNodeVmAttachmentToDelete.Namespace)
		deleteError := cvp.client.Delete(ctx, cnsNodeVmAttachmentToDelete)
		if deleteError != nil {
			if apierrors.IsNotFound(deleteError) {
				log.Info("The CnsNodeVmAttachment instance not found. It might have been deleted already")
			} else {
				volumeOpErrors.add(virtualMachineVolumeToDelete.Name, deleteError)
				log.Error(deleteError, "Unable to delete the CnsNodeVmAttachment instance", "name", cnsNodeVmAttachmentToDelete.Name, "namespace", cnsNodeVmAttachmentToDelete.Namespace)
			}
		}
	}

	if volumeOpErrors.hasOccurred() {
		return volumeOpErrors
	}
	return nil
}

// This function loops on vm.Status.Volumes and update its status by checking the corresponding CnsNodeVmAttachment instance
// Note: UpdateVmVolumesStatus() does not call client.Status().Update(), it just updates vm object and vitualmachine_controller.go
//       eventually will call apiserver to update the vm object
func (cvp *cnsVolumeProvider) UpdateVmVolumesStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	log := log.WithValues("vm", vm.NamespacedName())
	log.V(4).Info("Updating the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
	// Updating the existing volume status
	volumeOpErrors := newVolumeErrorsByOpType(VolumeOpUpdateStatus)
	volumesForStatusUpdate := getVolumesForStatusUpdate(vm)
	newVmVolumeStatus := make([]vmoperatorv1alpha1.VirtualMachineVolumeStatus, 0, len(volumesForStatusUpdate))
	for volumeName, currVolumeStatus := range volumesForStatusUpdate {
		cnsNodeVmAttachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		cnsNodeVmAttachmentName := constructCnsNodeVmAttachmentName(vm.Name, volumeName)
		cnsNodeVmAttachmentNamespace := vm.Namespace
		clientGetErr := cvp.client.Get(ctx, client.ObjectKey{Name: cnsNodeVmAttachmentName, Namespace: cnsNodeVmAttachmentNamespace}, cnsNodeVmAttachment)
		if clientGetErr != nil {
			/*
				When error happens from client.Get against CnsNodeVmAttachment instances
				Updating status for newly attached volumes:
				- If CnsNodeVmAttachment instance is not found:
				    Action: No need to update its status (reconcile will attempt to re-create CNS CRs)
				- If other errors:
				    Action: No need to update its status (reconcile will attempt to re-attach CNS CRs)

				Updating status for for existing volumes:
				- If CnsNodeVmAttachment instance is not found:
				    Action: No need to update its status (reconcile will attempt to re-create CNS CRs)
				- If other errors:
				    Action: Retains the status (reconcile will attempt to update its status in next loop)

				Updating status for detached volumes:
				- If CnsNodeVmAttachment instance is not found:
				    Action: No need to update its status (CNS CRs have been successfully deleted)
				- If other errors:
				    Action: Retains the status (reconcile will attempt to update its status in next loop)
			*/
			if apierrors.IsNotFound(clientGetErr) {
				log.Info("The CnsNodeVmAttachment not found. Skip updating its status", "name", cnsNodeVmAttachmentName, "namespace", cnsNodeVmAttachmentNamespace)
			} else {
				volumeOpErrors.add(volumeName, clientGetErr)
				log.Error(clientGetErr, "Unable to get CnsNodeVmAttachment to update the VirtualMachineVolumeStatus", "name", cnsNodeVmAttachmentName, "namespace", cnsNodeVmAttachmentNamespace)
				// Based on the inline comments above, we retain the volume status if it exists under this error condition
				if currVolumeStatus.Name != "" {
					newVmVolumeStatus = append(newVmVolumeStatus, currVolumeStatus)
				}
			}
		} else {
			newVmVolumeStatus = append(newVmVolumeStatus, vmoperatorv1alpha1.VirtualMachineVolumeStatus{
				Name:     volumeName,
				Attached: cnsNodeVmAttachment.Status.Attached,
				DiskUuid: cnsNodeVmAttachment.Status.AttachmentMetadata[AttributeFirstClassDiskUUID],
				Error:    cnsNodeVmAttachment.Status.Error,
			})
		}
	}

	vm.Status.Volumes = newVmVolumeStatus

	if volumeOpErrors.hasOccurred() {
		return volumeOpErrors
	}

	log.V(4).Info("Updated the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
	return nil
}

// GetVmVolumesToProcess returns a set of VirtualMachineVolume names desired, and a set of VirtualMachineVolume names need to be deleted
// by comparing vm.spec.volumes and vm.status.volumes.
// If vm.spec.volumes has [a,b,c], vm.status.volumes has [b,c,d], then vmVolumesDesired has [a], vmVolumesToDelete has [d]
//
// Note: virtualmachine_strategy.go#validateVolumes() has been implemented to validate the vm.Spec.Volumes, so the vm.Spec.Volumes only contain distinct element
func GetVmVolumesToProcess(vm *vmoperatorv1alpha1.VirtualMachine) (map[client.ObjectKey]bool, map[client.ObjectKey]bool) {
	log := log.WithValues("vm", vm.NamespacedName())
	log.V(4).Info("Getting the changes of VirtualMachineVolumes by processing the VirtualMachine object", "name", vm.NamespacedName())
	vmVolumesToAttach := map[client.ObjectKey]bool{}
	for _, desiredVirtualMachineVolume := range vm.Spec.Volumes {
		objectKey := client.ObjectKey{Name: desiredVirtualMachineVolume.Name, Namespace: vm.Namespace}
		vmVolumesToAttach[objectKey] = true
	}

	vmVolumesToDetach := map[client.ObjectKey]bool{}
	for _, currentVirtualMachineVolume := range vm.Status.Volumes {
		objectKey := client.ObjectKey{Name: currentVirtualMachineVolume.Name, Namespace: vm.Namespace}
		if vmVolumesToAttach[objectKey] {
			delete(vmVolumesToAttach, objectKey)
		} else {
			vmVolumesToDetach[objectKey] = true
		}
	}

	return vmVolumesToAttach, vmVolumesToDetach
}

// constructCnsNodeVmAttachmentName returns the name of CnsNodeVmAttachment
// 
func constructCnsNodeVmAttachmentName(vmName string, virtualMachineVolumeName string) string {
	return vmName + "-" + virtualMachineVolumeName
}

// getVmVolumesToUpdateStatus() returns the volumes need to be scanned for status updates.
// returns a map which contains the union of the {key: VirtualMachineVolumeName, value: VirtualMachineVolumeStatus} from both Spec.volumes and Status.volumes
func getVolumesForStatusUpdate(vm *vmoperatorv1alpha1.VirtualMachine) map[string]vmoperatorv1alpha1.VirtualMachineVolumeStatus {
	vmVolumesSet := make(map[string]vmoperatorv1alpha1.VirtualMachineVolumeStatus)
	for _, volume := range vm.Spec.Volumes {
		// Assign an empty status for the volumes in vm.Spec.Volumes
		vmVolumesSet[volume.Name] = vmoperatorv1alpha1.VirtualMachineVolumeStatus{}
	}
	for _, volumeStatus := range vm.Status.Volumes {
		vmVolumesSet[volumeStatus.Name] = volumeStatus
	}
	return vmVolumesSet
}

// This struct implements the error interface and makes caller of volume_provider.go to handle all errors for all types
// of volume operations easily.
//
// Caller of volume_provider.go might invoke AttachVolumes/DetachVolumes/UpdateVmVolumesStatus in the same controller,
// and want to handle all those volume operation errors in one place. This struct helps to append all errors and could be returned as an error
type CombinedVolumeOpsErrors struct {
	volumeErrorsByOpType []error
}

// This function appends error into the combined errors slice
func (cvoe *CombinedVolumeOpsErrors) Append(volumeErrorsByOpType error) {
	if volumeErrorsByOpType != nil {
		cvoe.volumeErrorsByOpType = append(cvoe.volumeErrorsByOpType, volumeErrorsByOpType)
	}
}

// This function indicates if there are any error occurred
func (cvoe CombinedVolumeOpsErrors) HasOccurred() bool {
	return len(cvoe.volumeErrorsByOpType) != 0
}

// The implementation of Error() interface which construct the final error messages
func (cvoe CombinedVolumeOpsErrors) Error() string {
	var errorMessages []string
	for _, errorMessage := range cvoe.volumeErrorsByOpType {
		errorMessages = append(errorMessages, errorMessage.Error())
	}
	return strings.Join(errorMessages, "\n")
}

// Instantiate a new VolumeErrorsByOpType instance
func newVolumeErrorsByOpType(volumeOptType string) *VolumeErrorsPerOpType {
	return &VolumeErrorsPerOpType{
		volumeOpType: volumeOptType,
		errors:       make(map[string]error),
	}
}

// This struct handles the errors when processing all volumes against one particular operation type.
// e.g. attach operation runs against multiple volumes of a vm, and this struct handles all volume errors in one place
type VolumeErrorsPerOpType struct {
	// volumeOpType indicates the volume operation type
	volumeOpType string
	// volumeOpErrors is a map which stores the error against each volume. Note: There won't duplicates in volume names.
	// key: volume name; value: error
	errors map[string]error
}

// Using voe *VolumeErrorsByOpType as the pointer receiver is because this function updates errorsList.
// Map and slice values behave like pointers: they are descriptors that contain pointers to the underlying map or slice data.
func (voe *VolumeErrorsPerOpType) add(volumeName string, err error) {
	if err != nil {
		voe.errors[volumeName] = err
	}
}

// HasOccurred() indicates whether the error occurred
func (voe VolumeErrorsPerOpType) hasOccurred() bool {
	return len(voe.errors) != 0
}

// Construct the final error message which shows the overall situation of current volume operation
func (voe VolumeErrorsPerOpType) Error() string {
	var errorMessages []string
	for volumeName, err := range voe.errors {
		errorMessages = append(errorMessages, fmt.Sprintf("operation: [%s] against volume: [%s] fails due to error: [%s]", voe.volumeOpType, volumeName, err.Error()))
	}
	return strings.Join(errorMessages, "\n")
}
