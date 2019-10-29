package providers

import (
	"context"
	"fmt"

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
	AttachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToAdd map[client.ObjectKey]bool) error
	DetachVolumes(ctx context.Context, virtualMachineVolumesDeleted map[client.ObjectKey]bool) error
	UpdateVmVolumesStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error
}

type cnsVolumeProvider struct {
	client client.Client
}

func CnsVolumeProvider(client client.Client) *cnsVolumeProvider {
	return &cnsVolumeProvider{client: client}
}

//TODO: CreateAttachments, DeleteCAttachments and UpdateVmVolumesStatus should return a slice of error: 
// CreateCnsNodeVmAttachments loop on the set of virtualMachineVolumesDesired and create the CnsNodeVmAttachment instances accordingly
// Then update the VirtualMachineVolume status on the processed VirtualMachineVolume instances
func (cvp *cnsVolumeProvider) AttachVolumes(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, virtualMachineVolumesToAdd map[client.ObjectKey]bool) error {
	var err error
	var virtualMachineVolumeNamesProcessed []string
	var virtualMachineVolumeStatusUpdated []string

	for virtualMachineVolume := range virtualMachineVolumesToAdd {
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
		err = cvp.client.Create(ctx, cnsNodeVmAttachment)
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				// Update the CnsNodeVmAttachment if it already exist
				err = cvp.client.Update(ctx, cnsNodeVmAttachment)
				if err != nil {
					log.Error(err, "The CnsNodeVmAttachment exists, but unable to update it", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
				} else {
					log.Info("Updated the CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
					virtualMachineVolumeNamesProcessed = append(virtualMachineVolumeNamesProcessed, virtualMachineVolume.Name)
				}
			} else {
				log.Error(err, "Unable to create CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
			}
		} else {
			log.Info("Created the CnsNodeVmAttachment", "name", cnsNodeVmAttachment.Name, "namespace", cnsNodeVmAttachment.Namespace)
			virtualMachineVolumeNamesProcessed = append(virtualMachineVolumeNamesProcessed, virtualMachineVolume.Name)
		}
		// Continue creating next CnsNodeVmAttachment, reconciliation will try to create those failed ones in the retry.
	}

	// Set the default status for the processed VirtualMachineVolume instances
	for _, virtualMachineVolumeName := range virtualMachineVolumeNamesProcessed {
		err = cvp.updateVmVolumeStatus(ctx, vm, vmoperatorv1alpha1.VirtualMachineVolumeStatus{
			Name:     virtualMachineVolumeName,
			Attached: false,
			DiskUuid: "",
			Error:    "",
		})
		// error has been logged in UpdateVmVolumeStatus()
		if err == nil {
			virtualMachineVolumeStatusUpdated = append(virtualMachineVolumeStatusUpdated, virtualMachineVolumeName)
		}
	}

	if len(virtualMachineVolumeNamesProcessed) != len(virtualMachineVolumeStatusUpdated) {
		return err
	}
	return nil
}

func (cvp *cnsVolumeProvider) DetachVolumes(ctx context.Context, virtualMachineVolumesToDelete map[client.ObjectKey]bool) error {
	// TODO: Delete CnsNodeVMAttachments on demand if VM has been reconciled properly
	return nil
}

func (cvp *cnsVolumeProvider) UpdateVmVolumesStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine) error {
	log.Info("Updating the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
	// Updating the existing volume status
	var err error
	for _, vmVolume := range vm.Status.Volumes {
		cnsNodeVmAttachment := &cnsv1alpha1.CnsNodeVmAttachment{}
		cnsNodeVmAttachmentName := constructCnsNodeVmAttachmentName(vm.Name, vmVolume.Name)
		cnsNodeVmAttachmentNamespace := vm.Namespace
		clientGetErr := cvp.client.Get(ctx, client.ObjectKey{Name: cnsNodeVmAttachmentName, Namespace: cnsNodeVmAttachmentNamespace}, cnsNodeVmAttachment)
		if clientGetErr != nil {
			err = clientGetErr
			if apierrors.IsNotFound(clientGetErr) {
				log.Info(fmt.Sprintf("The CnsNodeVmAttachment %s under namespace %s not found. Skip updating its status", cnsNodeVmAttachmentName, cnsNodeVmAttachmentNamespace))
			} else {
				log.Error(clientGetErr, "Unable to get CnsNodeVmAttachment to update the VirtualMachineVolumeStatus", "name", cnsNodeVmAttachmentName, "namespace", cnsNodeVmAttachmentNamespace)
			}
			continue
		}
		statusUpdateErr := cvp.updateVmVolumeStatus(ctx, vm, vmoperatorv1alpha1.VirtualMachineVolumeStatus{
			Name:     vmVolume.Name,
			Attached: cnsNodeVmAttachment.Status.Attached,
			DiskUuid: cnsNodeVmAttachment.Status.AttachmentMetadata[AttributeFirstClassDiskUUID],
			Error:    cnsNodeVmAttachment.Status.Error,
		})
		if statusUpdateErr != nil {
			err = statusUpdateErr
			// Logging has been done in updateVmVolumeStatus()
			continue
		}
	}

	if err != nil {
		log.Error(err, "Unable to update the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
		return err
	}

	// Deleting the orphan volume status
	_, vmVolumesToDelete := GetVmVolumesToProcess(vm)
	for vmVolume := range vmVolumesToDelete {
		deleteStatusErr := cvp.deleteVmVolumeStatus(ctx, vm, vmVolume.Name)
		if deleteStatusErr != nil {
			err = deleteStatusErr
		}
	}
	if err != nil {
		// logging has been done in deleteVmVolumeStatus()
		return err
	}

	log.Info("Updated the VirtualMachineVolumeStatus for VirtualMachine", "name", vm.Name, "namespace", vm.Namespace)
	return nil
}

// GetVmVolumesToProcess returns a set of VirtualMachineVolume names desired, and a set of VirtualMachineVolume names need to be deleted
// by comparing vm.spec.volumes and vm.status.volumes.
// If vm.spec.volumes has [a,b,c], vm.status.volumes has [b,c,d], then vmVolumesDesired has [a], vmVolumesToDelete has [d]
func GetVmVolumesToProcess(vm *vmoperatorv1alpha1.VirtualMachine) (map[client.ObjectKey]bool, map[client.ObjectKey]bool) {
	log.Info("Getting the changes of VirtualMachineVolumes by processing the VirtualMachine object", "name", vm.NamespacedName())
	vmVolumesToAdd := map[client.ObjectKey]bool{}
	for _, desiredVirtualMachineVolume := range vm.Spec.Volumes {
		objectKey := client.ObjectKey{Name: desiredVirtualMachineVolume.Name, Namespace: vm.Namespace}
		vmVolumesToAdd[objectKey] = true
	}

	vmVolumesToDelete := map[client.ObjectKey]bool{}
	for _, currentVirtualMachineVolume := range vm.Status.Volumes {
		objectKey := client.ObjectKey{Name: currentVirtualMachineVolume.Name, Namespace: vm.Namespace}
		if vmVolumesToAdd[objectKey] {
			delete(vmVolumesToAdd, objectKey)
		} else {
			vmVolumesToDelete[objectKey] = true
		}
	}

	return vmVolumesToAdd, vmVolumesToDelete
}

// updateVmVolumeStatus updates a particular VirtualMachineVolumeStatus record under VirtualMachine spec
func (cvp *cnsVolumeProvider) updateVmVolumeStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, vmVolumeStatusToUpdate vmoperatorv1alpha1.VirtualMachineVolumeStatus) error {
	isUpdated := false
	deepCopiedVm := vm.DeepCopy()
	var newVmVolumeStatus []vmoperatorv1alpha1.VirtualMachineVolumeStatus
	for _, currentVmVolumeStatus := range deepCopiedVm.Status.Volumes {
		// Find the record needs to be updated, and then update it
		if currentVmVolumeStatus.Name == vmVolumeStatusToUpdate.Name {
			newVmVolumeStatus = append(newVmVolumeStatus, vmVolumeStatusToUpdate)
			isUpdated = true
		} else {
			newVmVolumeStatus = append(newVmVolumeStatus, currentVmVolumeStatus)
		}
	}
	// If no record has been updated, then add the vmVolumeStatusToUpdate as the new record
	if !isUpdated {
		newVmVolumeStatus = append(newVmVolumeStatus, vmVolumeStatusToUpdate)
	}

	deepCopiedVm.Status.Volumes = newVmVolumeStatus
	vm = deepCopiedVm
	err := cvp.client.Status().Update(ctx, vm)
	if err != nil {
		log.Error(err, "Unable to update the volume status of VirtualMachine", "name", vm.Name, "namespace", vm.Namespace, "volumeName", vmVolumeStatusToUpdate.Name)
		return err
	}
	return nil
}

// deleteVmVolumeStatus deletes a particular VirtualMachineVolumeStatus record under VirtualMachine spec
func (cvp *cnsVolumeProvider) deleteVmVolumeStatus(ctx context.Context, vm *vmoperatorv1alpha1.VirtualMachine, vmVolumeNameToDelete string) error {
	var newVolumesStatus []vmoperatorv1alpha1.VirtualMachineVolumeStatus
	for _, currentVmVolumeStatus := range vm.Status.Volumes {
		if currentVmVolumeStatus.Name != vmVolumeNameToDelete {
			newVolumesStatus = append(newVolumesStatus, currentVmVolumeStatus)
		}
	}

	vm.Status.Volumes = newVolumesStatus
	err := cvp.client.Status().Update(ctx, vm)
	if err != nil {
		log.Error(err, fmt.Sprintf("Unable to delete the VirtualMachineVolumeStatus %s", vmVolumeNameToDelete), "namespace", vm.Namespace)
		return err
	}
	return nil
}

// constructCnsNodeVmAttachmentName returns the name of CnsNodeVmAttachment
// 
func constructCnsNodeVmAttachmentName(vmName string, virtualMachineVolumeName string) string {
	return vmName + "-" + virtualMachineVolumeName
}
