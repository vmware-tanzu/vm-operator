// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// AddControllersForVolumes mutates the VM to add any controllers to
// the spec if any of the specified volumes cannot be accommodated on
// the existing controllers. If one of the current controllers
// (spec.hardware.controllers) is suitable for the volume (e.g.,
// matches the sharing mode), we check if there are any open slots. If
// none are found, we add a controller.
func AddControllersForVolumes(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	volumes := vmopv1util.GetManagedVolumesWithPVC(*vm)
	if len(volumes) == 0 {
		return false, nil
	}

	var (
		existingControllers = vmopv1util.BuildVMControllersMap(vm)
		occupiedSlots       = make(map[pkgutil.ControllerID]sets.Set[int32])
		wasMutated          = false
	)

	// Add CD-ROM controllers to the occupied slots to check for conflicts.
	for _, cdrom := range vm.Spec.Hardware.Cdrom {
		if cdrom.ControllerBusNumber != nil &&
			cdrom.ControllerType != "" &&
			cdrom.UnitNumber != nil && *cdrom.UnitNumber >= 0 {

			controllerID := pkgutil.ControllerID{
				ControllerType: cdrom.ControllerType,
				BusNumber:      *cdrom.ControllerBusNumber,
			}

			if occupiedSlots[controllerID] == nil {
				occupiedSlots[controllerID] = sets.New[int32]()
			}
			occupiedSlots[controllerID].Insert(*cdrom.UnitNumber)
		}
	}

	// Process each volume to determine controller requirements.
	for i := range volumes {

		vol := &volumes[i]
		pvc := vol.PersistentVolumeClaim

		// Determine the target controller based on volume configuration.
		targetController := determineTargetController(
			pvc,
			existingControllers,
			occupiedSlots,
		)

		controllerID := vmopv1util.GenerateControllerID(targetController)

		if targetController != nil && controllerID.BusNumber >= 0 {

			if _, exists := existingControllers[controllerID]; !exists {
				// Check if we can add a new controller.
				if len(existingControllers) >= int(targetController.MaxCount()) {

					// Skipping this volume because we cannot add more
					// controllers. The validation webhook will throw an error
					// when it checks if the volume does not have a controller
					// assigned.
					ctx.Logger.Info(
						"Skipping volume because we cannot add more controllers",
						"busNumber", controllerID.BusNumber,
						"controllerType", controllerID.ControllerType,
						"maxCount", targetController.MaxCount(),
						"volume", vol.Name,
						"pvc", pvc.ClaimName,
					)
					continue
				}

				switch targetController := targetController.(type) {
				case vmopv1.SCSIControllerSpec:
					vm.Spec.Hardware.SCSIControllers = append(
						vm.Spec.Hardware.SCSIControllers,
						targetController,
					)
				case vmopv1.SATAControllerSpec:
					vm.Spec.Hardware.SATAControllers = append(
						vm.Spec.Hardware.SATAControllers,
						targetController,
					)
				case vmopv1.NVMEControllerSpec:
					vm.Spec.Hardware.NVMEControllers = append(
						vm.Spec.Hardware.NVMEControllers,
						targetController,
					)
				case vmopv1.IDEControllerSpec:
					vm.Spec.Hardware.IDEControllers = append(
						vm.Spec.Hardware.IDEControllers,
						targetController,
					)
				default:
					ctx.Logger.Info(
						"Skipping unsupported controller type",
						"busNumber", controllerID.BusNumber,
						"controllerType", controllerID.ControllerType,
						"volume", vol.Name,
						"pvc", pvc.ClaimName,
					)
					continue
				}
				existingControllers[controllerID] = targetController
				wasMutated = true
			}

			// Set the controllerType and controllerBusNumber on the volume to
			// ensure it uses the target controller. The controller will try to find
			// the first available slot on the lowest bus number of the
			// matching controller. If we do not specify the busNumber in
			// the volume explicitly, it is possible that by the time we
			// reconcile this volume, another slot has opened up on an
			// existing controller. We just ended up adding a controller
			// that does not have any devices attached to it. Let's try to
			// avoid that.
			if pvc.ControllerType == "" {
				pvc.ControllerType = controllerID.ControllerType
				wasMutated = true
			}
			if pvc.ControllerBusNumber == nil {
				pvc.ControllerBusNumber = &controllerID.BusNumber
				wasMutated = true
			}

			if occupiedSlots[controllerID] == nil {
				occupiedSlots[controllerID] = sets.New[int32]()
			}

			// If this volume doesn't have a unit number assigned yet,
			// we need to track that a slot will be occupied.
			if pvc.UnitNumber != nil {
				occupiedSlots[controllerID][*pvc.UnitNumber] = struct{}{}
			} else {
				// Find and reserve the next available slot for this volume.
				// The validation webhook will throw an error if a slot is
				// not available.
				if nextUnit := vmopv1util.NextAvailableUnitNumber(
					targetController,
					occupiedSlots[controllerID],
				); nextUnit >= 0 {
					occupiedSlots[controllerID][nextUnit] = struct{}{}
					pvc.UnitNumber = &nextUnit
					wasMutated = true
				}
			}
		}
	}

	return wasMutated, nil
}

// determineTargetController determines a controller for the passed PVC.
// The method will either return an available controller or create one.
// If there are no slots in any any controllers or all the bus numbers are
// occupied, the methods returns nil.
func determineTargetController(
	pvc *vmopv1.PersistentVolumeClaimVolumeSource,
	existingControllers map[pkgutil.ControllerID]vmopv1util.ControllerSpec,
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
) vmopv1util.ControllerSpec {

	// Default to SCSI if controllerType is not set.
	controllerType := pvc.ControllerType
	if controllerType == "" {
		controllerType = vmopv1.VirtualControllerTypeSCSI
	}

	sharingMode := vmopv1.VirtualControllerSharingModeNone
	if pvc.ApplicationType == vmopv1.VolumeApplicationTypeMicrosoftWSFC {
		sharingMode = vmopv1.VirtualControllerSharingModePhysical
	}

	// If a specific controller bus number is requested, return if one exists
	// or create one.
	if pvc.ControllerBusNumber != nil {
		controllerID := pkgutil.ControllerID{
			ControllerType: controllerType,
			BusNumber:      *pvc.ControllerBusNumber,
		}

		if controller, exists := existingControllers[controllerID]; exists {
			return controller
		}

		return vmopv1util.CreateNewController(controllerID, sharingMode)
	}

	// If a specific bus number is not requested, get the first controller
	// matching the type and sharing mode and that has an available slot.
	for busNum := range controllerType.MaxCount() {
		controllerID := pkgutil.ControllerID{
			ControllerType: controllerType,
			BusNumber:      busNum,
		}

		if controller, exists := existingControllers[controllerID]; exists {

			if occupiedSlots[controllerID] == nil {
				occupiedSlots[controllerID] = sets.New[int32]()
			}

			hasAvailableSlots := vmopv1util.NextAvailableUnitNumber(
				controller,
				occupiedSlots[controllerID],
			) >= 0
			sharingModeMatches := vmopv1util.GetControllerSharingMode(controller) == sharingMode

			if sharingModeMatches && hasAvailableSlots {
				return controller
			}
		}
	}

	// If an existing controller does not exist with an available slot,
	// create a new one if a bus is available.
	for busNum := range controllerType.MaxCount() {
		controllerID := pkgutil.ControllerID{
			ControllerType: controllerType,
			BusNumber:      busNum,
		}
		if _, exists := existingControllers[controllerID]; !exists {
			return vmopv1util.CreateNewController(controllerID, sharingMode)
		}
	}

	// No available bus numbers found.
	return nil
}
