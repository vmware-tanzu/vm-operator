// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

const (
	// Maximum number of SCSI controllers allowed per VM (4
	// controllers, indexed 0-3).
	maxSCSIControllers = int32(4)
)

// AddControllersForVolumes mutates the VM to add any controllers to
// the spec if any of the specified volumes cannot be accommodate on
// the existing controllers. If one of the current controllers
// (spec.hardware.controllers) is suitable for the volume (e.g.,
// matches the sharing mode), we check if there are any open slots. If
// none are found, we add a controller.
func AddControllersForVolumes(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	var (
		errs        []error
		err         error
		scsiMutated bool
	)
	scsiMutated, err = AddSCSIControllersForVolumes(ctx, client, vm)
	if err != nil {
		errs = append(errs, err)
	}

	// TODO: handle other controller types.

	return scsiMutated, apierrorsutil.NewAggregate(errs)
}

// AddSCSIControllersForVolumes adds SCSI controllers to the VM as
// needed based on volume requirements.
func AddSCSIControllersForVolumes(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	if len(vm.Spec.Volumes) == 0 {
		return false, nil
	}

	// Skip VMs that haven't been created on infrastructure yet. The
	// VM's spec may not contain controllers from the class/image, and
	// status won't have device information. The
	// reconcileSchemaUpgrade method will backfill controllers from
	// class/image into spec, which will trigger mutation webhook
	// again.
	if vm.Status.UniqueID == "" {
		return false, nil
	}

	// Initialize hardware spec if needed
	if vm.Spec.Hardware == nil {
		vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
	}

	// Track existing controller configurations
	existingControllers := make(map[int32]*vmopv1.SCSIControllerSpec)
	for i := range vm.Spec.Hardware.SCSIControllers {
		controller := &vm.Spec.Hardware.SCSIControllers[i]
		existingControllers[controller.BusNumber] = controller
	}

	// Get device counts from status to check availability
	statusDeviceCounts := make(map[int32]int32)
	if vm.Status.Hardware != nil {
		for _, controller := range vm.Status.Hardware.Controllers {
			if controller.Type == vmopv1.VirtualControllerTypeSCSI {
				statusDeviceCounts[controller.BusNumber] = int32(len(controller.Devices)) //nolint:gosec // disable G115
			}
		}
	}

	// Track controllers that need to be added
	controllersToAdd := make(map[int32]vmopv1.SCSIControllerSpec)

	// Track volume assignments per controller to calculate capacity
	volumeAssignments := make(map[int32]int32) // busNumber -> count of devices attached.

	// Process each volume to determine controller requirements
	for i := range vm.Spec.Volumes {
		vol := &vm.Spec.Volumes[i]

		if vol.PersistentVolumeClaim == nil {
			continue
		}

		pvc := vol.PersistentVolumeClaim

		// Determine the target controller based on volume configuration
		targetController, needsNewController := determineTargetController(
			pvc,
			existingControllers,
			controllersToAdd,
			statusDeviceCounts,
			volumeAssignments,
		)

		if needsNewController {
			// Check if we can add a new controller
			if len(existingControllers)+len(controllersToAdd) >= int(maxSCSIControllers) {
				// Cannot add more controllers. We don't return an
				// error here because it is possible that by the time
				// the request reaches the controller, a slot on an
				// existing controller frees up.
				continue
			}

			controllersToAdd[targetController.BusNumber] = targetController

			// Set the controllerBusNumber on the volume to ensure it
			// uses the newly created controller. The controller will
			// try to find the first available slot on the lowest bus
			// number of the matching controller. If we do not specify
			// the busNumber in the volume explicitly, it is possible
			// that by the time we reconcile this volume, another slot
			// has opened up on an existing controller. We just ended
			// up adding a controller does not have any devices
			// attached to it. Let's try to avoid that.
			if pvc.ControllerBusNumber == nil {
				busNum := targetController.BusNumber
				vol.PersistentVolumeClaim.ControllerBusNumber = &busNum
			}
		}

		// Track this volume assignment if we got a valid controller
		if targetController.BusNumber >= 0 {
			volumeAssignments[targetController.BusNumber]++
		}
	}

	// Add new controllers to the VM spec
	if len(controllersToAdd) > 0 {
		for busNum := range maxSCSIControllers {
			if controller, exists := controllersToAdd[busNum]; exists {
				vm.Spec.Hardware.SCSIControllers = append(vm.Spec.Hardware.SCSIControllers, controller)
			}
		}
		return true, nil
	}

	return false, nil
}

// determineTargetController determines which controller should be
// used for a volume and whether a new controller needs to be created.
func determineTargetController(
	pvc *vmopv1.PersistentVolumeClaimVolumeSource,
	existingControllers map[int32]*vmopv1.SCSIControllerSpec,
	controllersToAdd map[int32]vmopv1.SCSIControllerSpec,
	statusDeviceCounts map[int32]int32,
	volumeAssignments map[int32]int32,
) (vmopv1.SCSIControllerSpec, bool) {

	// We only handle SCSI controllers here.
	// TODO: handle other controllers in a followup.
	if pvc.ControllerType != "" && pvc.ControllerType != vmopv1.VirtualControllerTypeSCSI {
		return vmopv1.SCSIControllerSpec{BusNumber: -1}, false
	}

	// If a specific controller is requested, check if it already
	// exists. If not, we need to create it.
	if pvc.ControllerBusNumber != nil {
		busNum := *pvc.ControllerBusNumber

		if controller, exists := existingControllers[busNum]; exists {
			return *controller, false
		}

		// If we are already going to create it, return early.
		if controller, exists := controllersToAdd[busNum]; exists {
			return controller, false
		}

		// Need to create this specific controller
		return createSCSIController(busNum, vmopv1.VirtualControllerSharingModeNone), true
	}

	// Determine controller requirements based on volume configuration
	// After SetPVCVolumeDefaultsOnCreate has run, the volume will
	// have diskMode and sharingMode set

	// Check if this is an OracleRAC volume (either by applicationType
	// or by explicit sharingMode=MultiWriter) OracleRAC volumes (with
	// sharingMode=MultiWriter) need a controller with
	// sharingMode=None
	if pvc.ApplicationType == vmopv1.VolumeApplicationTypeOracleRAC ||
		pvc.SharingMode == vmopv1.VolumeSharingModeMultiWriter {
		controller, needsNew := findOrCreateControllerWithSharing(
			vmopv1.VirtualControllerSharingModeNone,
			existingControllers,
			controllersToAdd,
			statusDeviceCounts,
			volumeAssignments,
		)
		return controller, needsNew
	}

	// Check if this is a MicrosoftWSFC volume
	// WSFC volumes need a controller with sharingMode=Physical
	if pvc.ApplicationType == vmopv1.VolumeApplicationTypeMicrosoftWSFC {
		controller, needsNew := findOrCreateControllerWithSharing(
			vmopv1.VirtualControllerSharingModePhysical,
			existingControllers,
			controllersToAdd,
			statusDeviceCounts,
			volumeAssignments,
		)
		return controller, needsNew
	}

	// Default behavior: find first SCSI controller with available
	// slots If no controller with available slots exists, create a
	// new ParaVirtual SCSI controller
	controller, needsNew := findOrCreateFirstAvailableController(
		existingControllers,
		controllersToAdd,
		statusDeviceCounts,
		volumeAssignments,
	)
	return controller, needsNew
}

// findOrCreateControllerWithSharing finds an existing controller with
// the specified sharing mode that has available slots, or creates a
// new one if none exists.
func findOrCreateControllerWithSharing(
	sharingMode vmopv1.VirtualControllerSharingMode,
	existingControllers map[int32]*vmopv1.SCSIControllerSpec,
	controllersToAdd map[int32]vmopv1.SCSIControllerSpec,
	statusDeviceCounts map[int32]int32,
	volumeAssignments map[int32]int32,
) (vmopv1.SCSIControllerSpec, bool) {

	// First, check existing controllers for a match with available capacity
	for busNum := range maxSCSIControllers {
		if controller, exists := existingControllers[busNum]; exists {
			if controller.SharingMode == sharingMode {
				// Check if this controller has available slots
				if hasAvailableSlots(controller, busNum, statusDeviceCounts, volumeAssignments) {
					return *controller, false
				}
			}
		}
	}

	// Check controllers scheduled to be added
	for busNum := range maxSCSIControllers {
		if controller, exists := controllersToAdd[busNum]; exists { //nolint:gosec // disable G115
			if controller.SharingMode == sharingMode {
				// New controllers always have available slots
				return controller, false
			}
		}
	}

	// Need to create a new controller with the specified sharing mode
	// Find the first available bus number
	for busNum := range maxSCSIControllers {
		_, existsInExisting := existingControllers[busNum]
		_, existsInToAdd := controllersToAdd[busNum]

		if !existsInExisting && !existsInToAdd {
			controller := createSCSIController(busNum, sharingMode)
			return controller, true
		}
	}

	// No available bus numbers
	return vmopv1.SCSIControllerSpec{BusNumber: -1}, false
}

// findOrCreateFirstAvailableController finds the first SCSI
// controller with available slots, or creates a new one if no
// controller with available slots exists.
func findOrCreateFirstAvailableController(
	existingControllers map[int32]*vmopv1.SCSIControllerSpec,
	controllersToAdd map[int32]vmopv1.SCSIControllerSpec,
	statusDeviceCounts map[int32]int32,
	volumeAssignments map[int32]int32,
) (vmopv1.SCSIControllerSpec, bool) {

	// Check existing controllers for one with available capacity (in
	// order: 0, 1, 2, 3)
	for busNum := range maxSCSIControllers {
		if controller, exists := existingControllers[busNum]; exists {
			// Check if this controller has available slots
			if hasAvailableSlots(controller, busNum, statusDeviceCounts, volumeAssignments) {
				return *controller, false
			}
		}
	}

	// Check controllers scheduled to be added (they always have
	// available slots)
	for busNum := range maxSCSIControllers {
		if controller, exists := controllersToAdd[busNum]; exists {
			return controller, false
		}
	}

	// No existing controller with available slots found
	// Create a new controller at the first available bus number
	for busNum := range maxSCSIControllers {
		_, existsInExisting := existingControllers[busNum]
		_, existsInToAdd := controllersToAdd[busNum]

		if !existsInExisting && !existsInToAdd {
			controller := createSCSIController(busNum, vmopv1.VirtualControllerSharingModeNone)
			return controller, true
		}
	}

	// All controller slots are occupied (validation will catch this)
	return vmopv1.SCSIControllerSpec{BusNumber: -1}, false
}

// hasAvailableSlots checks if a controller has available slots for new devices.
func hasAvailableSlots(
	controller *vmopv1.SCSIControllerSpec,
	busNumber int32,
	statusDeviceCounts map[int32]int32,
	volumeAssignments map[int32]int32,
) bool {
	// Get max slots for this controller type
	maxSlots := getMaxSlotsForControllerType(controller.Type)

	// Get current device count from status
	currentDevices := statusDeviceCounts[busNumber]

	// Get number of volumes already assigned to this controller in this mutation
	assignedVolumes := volumeAssignments[busNumber]

	// Check if adding one more volume would exceed capacity
	return (currentDevices + assignedVolumes + 1) <= maxSlots
}

// getMaxSlotsForControllerType returns the maximum number of device
// slots for a controller type.
// PVSCSI supports 64 devices per controller starting
// vSphere 6.7+. Since min supported vSphere version is >=
// 8.0, this is a safe assumption.

// For all these sub-types, the max slots is 1 less than the capacity
// since SCSI slot 7 is reserved for the controller itself.
func getMaxSlotsForControllerType(ctrlType vmopv1.SCSIControllerType) int32 {
	switch ctrlType {
	case vmopv1.SCSIControllerTypeParaVirtualSCSI:
		return 63 // 64 targets - 1 for controller itself
	case vmopv1.SCSIControllerTypeBusLogic:
		return 15 // 16 targets - 1 for controller itself
	case vmopv1.SCSIControllerTypeLsiLogic:
		return 15 // 16 targets - 1 for controller itself
	case vmopv1.SCSIControllerTypeLsiLogicSAS:
		return 15 // 16 targets - 1 for controller itself
	default:
		// Default to ParaVirtual if unknown
		return 63
	}
}

// createSCSIController creates a new SCSI controller with default
// settings.
func createSCSIController(
	busNumber int32,
	sharingMode vmopv1.VirtualControllerSharingMode) vmopv1.SCSIControllerSpec {
	return vmopv1.SCSIControllerSpec{
		BusNumber:   busNumber,
		Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI, // Default to ParaVirtual
		SharingMode: sharingMode,
	}
}
