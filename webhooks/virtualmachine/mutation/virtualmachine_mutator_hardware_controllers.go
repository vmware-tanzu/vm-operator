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
	_ ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	if len(vm.Spec.Volumes) == 0 {
		return false, nil
	}

	var (
		controllerSpecs = vmopv1util.NewControllerSpecs(*vm)
		occupiedSlots   = make(map[pkgutil.ControllerID]sets.Set[int32])
	)

	// Add CD-ROM controllers to the occupied slots to check for conflicts.
	if vm.Spec.Hardware != nil {
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
	}

	explicitPlacementVolumes := []*vmopv1.VirtualMachineVolume{}
	implicitPlacementVolumes := []*vmopv1.VirtualMachineVolume{}

	// Separate volumes into explicit and implicit placement.
	for i := range vm.Spec.Volumes {
		vol := &vm.Spec.Volumes[i]
		if vol.ControllerBusNumber != nil &&
			vol.ControllerType != "" &&
			vol.UnitNumber != nil && *vol.UnitNumber >= 0 {

			explicitPlacementVolumes = append(explicitPlacementVolumes, vol)
		} else {
			implicitPlacementVolumes = append(implicitPlacementVolumes, vol)
		}
	}

	// Add controllers for explicit placement volumes. This should happen first
	// because it will reserve slots that may be available for implicit placement.
	wasMutated := processVolumes(
		ctx,
		vm,
		explicitPlacementVolumes,
		controllerSpecs,
		occupiedSlots,
	)

	// Add controllers for implicit placement volumes. This should happen last
	// because it will not reserve slots that may be available for implicit placement.
	wasMutated = processVolumes(
		ctx,
		vm,
		implicitPlacementVolumes,
		controllerSpecs,
		occupiedSlots,
	) || wasMutated

	return wasMutated, nil
}

func processVolumes(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine,
	volumes []*vmopv1.VirtualMachineVolume,
	controllerSpecs vmopv1util.ControllerSpecs,
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
) bool {

	var wasMutated bool

	// Process each volume to determine controller requirements.
	for _, vol := range volumes {

		// Determine the target controller based on volume configuration.
		targetController := determineTargetController(
			*ctx,
			vol,
			controllerSpecs,
			occupiedSlots,
		)

		controllerID := vmopv1util.GenerateControllerID(targetController)

		if targetController != nil && controllerID.BusNumber >= 0 {

			if _, ok := controllerSpecs.Get(controllerID.ControllerType, controllerID.BusNumber); !ok {
				// Check if we can add a new controller.
				if controllerSpecs.CountControllers(controllerID.ControllerType) >= int(targetController.MaxCount()) {

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
					)
					continue
				}

				if vm.Spec.Hardware == nil {
					vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
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
					)
					continue
				}
				controllerSpecs.Set(controllerID.ControllerType,
					controllerID.BusNumber, targetController)
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
			if vol.ControllerType == "" {
				vol.ControllerType = controllerID.ControllerType
				wasMutated = true
			}
			if vol.ControllerBusNumber == nil {
				vol.ControllerBusNumber = &controllerID.BusNumber
				wasMutated = true
			}

			if occupiedSlots[controllerID] == nil {
				occupiedSlots[controllerID] = sets.New[int32]()
			}

			// If this volume doesn't have a unit number assigned yet,
			// we need to track that a slot will be occupied.
			if vol.UnitNumber != nil {
				occupiedSlots[controllerID].Insert(*vol.UnitNumber)
			} else {
				// Find and reserve the next available slot for this volume.
				// The validation webhook will throw an error if a slot is
				// not available.
				if nextUnit := vmopv1util.NextAvailableUnitNumber(
					targetController,
					occupiedSlots[controllerID],
				); nextUnit >= 0 {
					occupiedSlots[controllerID].Insert(nextUnit)
					vol.UnitNumber = &nextUnit
					wasMutated = true
				}
			}
		}
	}

	return wasMutated
}

// determineTargetController determines a controller for the passed PVC.
// The method will either return an available controller or create one.
// If there are no slots in any controllers or all the bus numbers are
// occupied, the methods returns nil.
func determineTargetController(
	ctx pkgctx.WebhookRequestContext,
	vol *vmopv1.VirtualMachineVolume,
	controllerSpecs vmopv1util.ControllerSpecs,
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
) vmopv1util.ControllerSpec {

	// Default to SCSI if controllerType is not set.
	controllerType := vol.ControllerType
	if controllerType == "" {
		controllerType = vmopv1.VirtualControllerTypeSCSI
	}

	sharingMode := vmopv1.VirtualControllerSharingModeNone
	if vol.ApplicationType == vmopv1.VolumeApplicationTypeMicrosoftWSFC {
		sharingMode = vmopv1.VirtualControllerSharingModePhysical
	}

	// If a specific controller bus number is requested, return if one exists
	// or create one.
	if vol.ControllerBusNumber != nil {

		if controller, ok := controllerSpecs.
			Get(controllerType, *vol.ControllerBusNumber); ok {
			return controller
		}

		return vmopv1util.CreateNewController(controllerType,
			*vol.ControllerBusNumber, sharingMode)
	}

	// If a specific bus number is not requested, get the first controller
	// matching the type and sharing mode and that has an available slot.
	for busNum := int32(0); busNum < controllerType.MaxCount(); busNum++ {
		controllerID := pkgutil.ControllerID{
			ControllerType: controllerType,
			BusNumber:      busNum,
		}

		if controller, ok := controllerSpecs.Get(controllerType, busNum); ok {

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

	// If no available controller is found, create a new one.
	for busNum := int32(0); busNum < controllerType.MaxCount(); busNum++ {
		if _, ok := controllerSpecs.Get(controllerType, busNum); !ok {
			return vmopv1util.CreateNewController(controllerType, busNum, sharingMode)
		}
	}

	// No available bus numbers found.
	return nil
}

// SetPVCVolumeDefaults sets the default configuration for a volume
// based on its application type.
func SetPVCVolumeDefaults(
	ctx *pkgctx.WebhookRequestContext,
	c ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	var wasMutated bool
	for i := range vm.Spec.Volumes {
		v := &vm.Spec.Volumes[i]

		switch v.ApplicationType {
		case vmopv1.VolumeApplicationTypeOracleRAC:
			if v.DiskMode == "" {
				v.DiskMode = vmopv1.VolumeDiskModeIndependentPersistent
				wasMutated = true
			}
			if v.SharingMode == "" {
				v.SharingMode = vmopv1.VolumeSharingModeMultiWriter
				wasMutated = true
			}
		case vmopv1.VolumeApplicationTypeMicrosoftWSFC:
			if v.DiskMode == "" {
				v.DiskMode = vmopv1.VolumeDiskModeIndependentPersistent
				wasMutated = true
			}
			if v.SharingMode == "" {
				v.SharingMode = vmopv1.VolumeSharingModeNone
				wasMutated = true
			}
		default:
			// No application type specified. Set default values:
			// DiskMode defaults to Persistent and SharingMode defaults to None.
			if v.DiskMode == "" {
				v.DiskMode = vmopv1.VolumeDiskModePersistent
				wasMutated = true
			}
			if v.SharingMode == "" {
				v.SharingMode = vmopv1.VolumeSharingModeNone
				wasMutated = true
			}
		}
	}
	return wasMutated, nil
}
