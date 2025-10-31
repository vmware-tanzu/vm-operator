// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	apierrorsutil "k8s.io/apimachinery/pkg/util/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
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

	var (
		// Track existing controller configurations
		existingControllers = make(map[int32]vmopv1.SCSIControllerSpec)

		// Get device counts from status to check availability
		statusDeviceCounts = make(map[int32]int32)

		// Track volume assignments per controller to calculate capacity
		// busNumber -> count of devices attached.
		volumeAssignments = make(map[int32]int32)

		wasMutated = false
	)

	for i := range vm.Spec.Hardware.SCSIControllers {
		controller := vm.Spec.Hardware.SCSIControllers[i]
		existingControllers[controller.BusNumber] = controller
	}

	if vm.Status.Hardware != nil {
		for _, controller := range vm.Status.Hardware.Controllers {
			if controller.Type == vmopv1.VirtualControllerTypeSCSI {
				statusDeviceCounts[controller.BusNumber] = int32(len(controller.Devices)) //nolint:gosec // disable G115
			}
		}
	}

	// Process each volume to determine controller requirements
	for i := range vm.Spec.Volumes {
		vol := &vm.Spec.Volumes[i]

		if vol.PersistentVolumeClaim != nil {

			pvc := vol.PersistentVolumeClaim

			// Determine the target controller based on volume configuration
			targetController, needsNewController := determineTargetController(
				pvc,
				existingControllers,
				statusDeviceCounts,
				volumeAssignments,
			)

			if targetController.IsValid() {

				// Track this volume assignment if we got a valid controller
				volumeAssignments[targetController.BusNumber]++

				if needsNewController {
					// Check if we can add a new controller
					if len(existingControllers) >=
						int(vmopv1.VirtualControllerTypeSCSI.MaxPerVM()) {

						// Cannot add more controllers. We don't return an
						// error here because it is possible that by the time
						// the request reaches the controller, a slot on an
						// existing controller frees up.
						continue
					}

					vm.Spec.Hardware.SCSIControllers = append(
						vm.Spec.Hardware.SCSIControllers,
						targetController,
					)
					existingControllers[targetController.BusNumber] = targetController
					wasMutated = true
				}

				// Set the controllerBusNumber and controllerType on the volume to
				// ensure it uses the target controller. The controller will try to find
				// the first available slot on the lowest bus number of the
				// matching controller. If we do not specify the busNumber in
				// the volume explicitly, it is possible that by the time we
				// reconcile this volume, another slot has opened up on an
				// existing controller. We just ended up adding a controller
				// that does not have any devices attached to it. Let's try to
				// avoid that.
				if pvc.ControllerBusNumber == nil {
					busNum := targetController.BusNumber
					vol.PersistentVolumeClaim.ControllerBusNumber = &busNum
					wasMutated = true
				}

				// Set controllerType to SCSI if not already set
				if pvc.ControllerType == "" {
					vol.PersistentVolumeClaim.ControllerType = vmopv1.VirtualControllerTypeSCSI
					wasMutated = true
				}
			}
		}
	}

	return wasMutated, nil
}

// determineTargetController determines which controller should be
// used for a volume and whether a new controller needs to be created.
// This function currently only supports SCSI controllers.
//
// TODO(OracleRAC): Implement support in determineTargetController for
// IDE, NVME, and SATA controllers.
func determineTargetController(
	pvc *vmopv1.PersistentVolumeClaimVolumeSource,
	existingControllers map[int32]vmopv1.SCSIControllerSpec,
	statusDeviceCounts, volumeAssignments map[int32]int32,
) (vmopv1.SCSIControllerSpec, bool) {

	if pvc.ControllerType != "" &&
		pvc.ControllerType != vmopv1.VirtualControllerTypeSCSI {
		return vmopv1.NewEmptySCSIControllerSpec(), false
	}

	sharingMode := pvc.GetVirtualControllerSharingMode()

	// If a specific controller is requested, check if it already
	// exists. If not, we need to create it.
	if pvc.ControllerBusNumber != nil {
		busNum := *pvc.ControllerBusNumber

		if controller, exists := existingControllers[busNum]; exists {
			return controller, false
		}

		// Need to create this specific controller
		return createSCSIController(busNum, sharingMode), true
	}

	// Determine controller requirements based on volume configuration.
	// After SetPVCVolumeDefaultsOnCreate has run, the volume will
	// have diskMode and sharingMode set.
	return findOrCreateControllerWithSharing(
		sharingMode,
		existingControllers,
		statusDeviceCounts,
		volumeAssignments,
	)
}

// findOrCreateControllerWithSharing finds an existing controller with
// the specified sharing mode that has available slots, or creates a
// new one if none exist with the specified sharing mode.
// Returns the controller and a boolean indicating if the controller was new.
func findOrCreateControllerWithSharing(
	sharingMode vmopv1.VirtualControllerSharingMode,
	existingControllers map[int32]vmopv1.SCSIControllerSpec,
	statusDeviceCounts, volumeAssignments map[int32]int32,
) (vmopv1.SCSIControllerSpec, bool) {

	// First, check existing controllers for a match with available capacity.
	for busNum := range vmopv1.VirtualControllerTypeSCSI.MaxPerVM() {
		if controller, exists := existingControllers[busNum]; exists {

			hasAvailableSlots := vmopv1util.ControllerHasAvailableSlots(
				&controller, busNum, statusDeviceCounts, volumeAssignments)
			sharingModeMatches := controller.SharingMode == sharingMode

			if sharingModeMatches && hasAvailableSlots {
				return controller, false
			}
		}
	}

	// Create a new controller at the first available bus number.
	for busNum := range vmopv1.VirtualControllerTypeSCSI.MaxPerVM() {
		if _, exists := existingControllers[busNum]; !exists {
			return createSCSIController(busNum, sharingMode), true
		}
	}

	// No available bus numbers found.
	return vmopv1.NewEmptySCSIControllerSpec(), false
}

// createSCSIController creates a new SCSI controller with the specified
// bus number, sharing mode, and ParaVirtual type.
func createSCSIController(
	busNumber int32,
	sharingMode vmopv1.VirtualControllerSharingMode,
) vmopv1.SCSIControllerSpec {
	return vmopv1.SCSIControllerSpec{
		BusNumber:   busNumber,
		Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
		SharingMode: sharingMode,
	}
}
