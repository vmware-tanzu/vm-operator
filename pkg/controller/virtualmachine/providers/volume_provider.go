package providers

import (
	"context"

	ptr "github.com/kubernetes/utils/pointer"
	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
	cnsv1alpha1 "gitlab.eng.vmware.com/hatchway/vsphere-csi-driver/pkg/syncer/cnsoperator/apis/cnsnodevmattachment/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const (
	LoggerName                         = "virtualmachine-volume-provider"
	AttributeFirstClassDiskUUID        = "diskUUID"
	cnsNodeVmAttachmentOwnerRefVersion = "vmoperator.vmware.com"
	cnsNodeVmAttachmentOwnerRefKind    = "VirtualMachine"
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
	var err error
	for virtualMachineVolume := range virtualMachineVolumesToAttach {
		cnsNodeVmAttachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		cnsNodeVmAttachment.SetName(constructCnsNodeVmAttachmentName(vm.Name, virtualMachineVolume.Name))
		cnsNodeVmAttachment.SetNamespace(virtualMachineVolume.Namespace)
		cnsNodeVmAttachment.Spec.NodeUUID = vm.Status.BiosUuid
		// From  it puts volume name as pcvsc-<nodeuuid>
		// cnsNodeVmAttachment.Spec.VolumeName = "pvcsc-" + cnsNodeVmAttachment.Spec.NodeUUID
		cnsNodeVmAttachment.Spec.VolumeName = virtualMachineVolume.Name
		cnsNodeVmAttachment.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion:         cnsNodeVmAttachmentOwnerRefVersion,
				Controller:         ptr.BoolPtr(true),
				BlockOwnerDeletion: ptr.BoolPtr(true),
				Kind:               cnsNodeVmAttachmentOwnerRefKind,
				Name:               vm.Name,
				UID:                vm.UID,
			},
		}
		log.Info("Attempting to create the CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
		clientCreateError := cvp.client.Create(ctx, cnsNodeVmAttachment)
		if clientCreateError != nil {
			// Ignore if the CRD instance already exists
			if !apierrors.IsAlreadyExists(clientCreateError) {
				err = clientCreateError
				log.Error(clientCreateError, "Unable to create CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
			}
		} else {
			log.Info("Created the CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
		}
		// Continue creating next CnsNodeVmAttachment, reconciliation will try to create those failed ones in the retry.
	}

	if err != nil {
		return err
	}
	return nil
}

// This function loops on virtualMachineVolumesToDelete and delete the CnsNodeVmAttachment instance respectively
// Return error when fails to delete CnsNodeVmAttachment instances (partially or completely)
// Note: DetachVolumes() does not update the vm.Status.Volumes since it has been handled by UpdateVmVolumesStatus() by checking the existence of
//       respective CnsNodeVmAttachment instance
func (cvp *cnsVolumeProvider) DetachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToDetach map[client.ObjectKey]bool) error {
	var err error
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
				err = deleteError
				log.Error(deleteError, "Unable to delete the CnsNodeVmAttachment instance", "name", cnsNodeVmAttachmentToDelete.Name, "namespace", cnsNodeVmAttachmentToDelete.Namespace)
			}
		}
	}

	if err != nil {
		return err
	}
	return nil
}

// This function loops on vm.Status.Volumes and update its status by checking the corresponding CnsNodeVmAttachment instance
// Note: UpdateVmVolumesStatus() does not call client.Status().Update(), it just updates vm object and vitualmachine_controller.go
//       eventually will call apiserver to update the vm object
func (cvp *cnsVolumeProvider) UpdateVmVolumesStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	log.Info("Updating the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
	// Updating the existing volume status
	var err error
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
				err = clientGetErr
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

	if err != nil {
		log.Error(err, "Unable to update the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
		return err
	}

	log.Info("Updated the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
	return nil
}

// GetVmVolumesToProcess returns a set of VirtualMachineVolume names desired, and a set of VirtualMachineVolume names need to be deleted
// by comparing vm.spec.volumes and vm.status.volumes.
// If vm.spec.volumes has [a,b,c], vm.status.volumes has [b,c,d], then vmVolumesDesired has [a], vmVolumesToDelete has [d]
//
// Note: virtualmachine_strategy.go#validateVolumes() has been implemented to validate the vm.Spec.Volumes, so the vm.Spec.Volumes only contain distinct element
func GetVmVolumesToProcess(vm *vmoperatorv1alpha1.VirtualMachine) (map[client.ObjectKey]bool, map[client.ObjectKey]bool) {
	log.Info("Getting the changes of VirtualMachineVolumes by processing the VirtualMachine object", "name", vm.NamespacedName())
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
