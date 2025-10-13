// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	// Maximum number of SCSI controllers allowed per VM.
	maxSCSIControllers = int32(4)

	// Maximum number of devices per SCSI controller type.
	// Note: The controller itself occupies one slot.
	maxParaVirtualSCSISlots = 63 // 64 targets - 1 for controller
	maxBusLogicSlots        = 15 // 16 targets - 1 for controller
	maxLsiLogicSlots        = 15 // 16 targets - 1 for controller
	maxLsiLogicSASSlots     = 15 // 16 targets - 1 for controller

	// Unit number reserved for SCSI controller on its own bus.
	scsiControllerUnitNumber = 7
)

const (
	invalidControllerBusNumberRangeFmt     = "must be between 0 and %d"
	invalidControllerBusNumberDoesNotExist = "SCSI controller with bus number %d does not exist"
	invalidUnitNumberReserved              = "unit number 7 is reserved for the SCSI controller itself"
	invalidUnitNumberRangeFmt              = "unit number must be less than %d for %s controller"
	invalidControllerCapacityFmt           = "SCSI controller %d (type=%s) has %d devices and cannot accommodate %d more volumes (%v). Maximum slots: %d"
)

// validateControllerSlots validates that all volumes can be attached
// to controllers without exceeding the maximum number of devices per
// controller.
// We are not trying to be extensive here. The idea is to only perform
// basic validations to the extent we can, and fall through to the
// controller. It is possible that the VM is changed between the
// webhook check and actual reconciliation, so this is best effort
// anyway. We verify:
//   - If a controllerBusNumber is specified, it must not be invalid
//     (beyond max allowed). and there must be an entry in
//     spec.hardware.controllers for thiscontroller.
//   - verify that we don't have more devices than max allowed for
//     each controller type.
//
// Importantly, if a volume doesn't specify a controllerBusNumber, or
// if we can't find a controller for it, we don't reject the request.
func (v validator) validateControllerSlots(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if len(vm.Spec.Volumes) == 0 {
		return allErrs
	}

	volumesPath := field.NewPath("spec", "volumes")

	// Build controller map from spec.hardware.controllers
	specControllers := make(map[int32]controllerInfo)
	if vm.Spec.Hardware != nil {
		for _, controller := range vm.Spec.Hardware.SCSIControllers {
			specControllers[controller.BusNumber] = controllerInfo{
				busNumber:   controller.BusNumber,
				ctrlType:    controller.Type,
				sharingMode: controller.SharingMode,
				maxSlots:    getMaxSlotsForControllerType(controller.Type),
				usedSlots:   0,
			}
		}
	}

	// Build device count map from status.hardware.controllers
	statusDeviceCounts := make(map[int32]int32)
	if vm.Status.Hardware != nil {
		for _, controller := range vm.Status.Hardware.Controllers {
			if controller.Type == vmopv1.VirtualControllerTypeSCSI {
				statusDeviceCounts[controller.BusNumber] = int32(len(controller.Devices)) //nolint:gosec // disable G115
			}
		}
	}

	// Track volume assignments to controllers
	volumeAssignments := make(map[int32][]string) // busNumber -> volume names

	// Process each volume
	for i, vol := range vm.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		pvc := vol.PersistentVolumeClaim
		volPath := volumesPath.Index(i)

		// Determine which controller this volume should use
		var targetBusNumber int32
		var targetController controllerInfo
		var hasTarget bool

		// Check if a specific controller is requested
		if pvc.ControllerBusNumber != nil {
			targetBusNumber = *pvc.ControllerBusNumber

			// Validate bus number range
			if targetBusNumber < 0 || targetBusNumber >= maxSCSIControllers {
				allErrs = append(allErrs, field.Invalid(
					volPath.Child("persistentVolumeClaim", "controllerBusNumber"),
					targetBusNumber,
					fmt.Sprintf(invalidControllerBusNumberRangeFmt, maxSCSIControllers-1)))
				continue
			}

			targetController, hasTarget = specControllers[targetBusNumber]
			if !hasTarget {
				allErrs = append(allErrs, field.Invalid(
					volPath.Child("persistentVolumeClaim", "controllerBusNumber"),
					targetBusNumber,
					fmt.Sprintf(invalidControllerBusNumberDoesNotExist, targetBusNumber)))
				continue
			}
		} else {
			// Find appropriate controller based on application type
			switch pvc.ApplicationType {
			case vmopv1.VolumeApplicationTypeOracleRAC:
				targetController, hasTarget = findControllerWithSharing(
					vmopv1.VirtualControllerSharingModeNone,
					specControllers,
				)
				if !hasTarget {
					continue
				}

			case vmopv1.VolumeApplicationTypeMicrosoftWSFC:
				targetController, hasTarget = findControllerWithSharing(
					vmopv1.VirtualControllerSharingModePhysical,
					specControllers,
				)
				if !hasTarget {
					continue
				}

			default:
				// Use first available SCSI controller (bus 0)
				targetController, hasTarget = specControllers[0]
				if !hasTarget {
					// No controller at bus 0 - mutation webhook will add one
					// Skip tracking this volume
					continue
				}
			}

			if hasTarget {
				targetBusNumber = targetController.busNumber
			}
		}

		// Validate unit number if specified
		if pvc.UnitNumber != nil {
			unitNum := *pvc.UnitNumber

			// Unit number 7 is reserved for the controller itself
			if unitNum == scsiControllerUnitNumber {
				allErrs = append(allErrs, field.Invalid(
					volPath.Child("persistentVolumeClaim", "unitNumber"),
					unitNum,
					invalidUnitNumberReserved))
				continue
			}

			// Validate unit number is within range for controller type
			if hasTarget && unitNum >= targetController.maxSlots {
				allErrs = append(allErrs, field.Invalid(
					volPath.Child("persistentVolumeClaim", "unitNumber"),
					unitNum,
					fmt.Sprintf(invalidUnitNumberRangeFmt,
						targetController.maxSlots, targetController.ctrlType)))
				continue
			}
		}

		// Track this volume assignment so we can figure out max
		// devices attached to a controller.
		if hasTarget {
			volumeAssignments[targetBusNumber] = append(volumeAssignments[targetBusNumber], vol.Name)
		}
	}

	// Validate that each controller has enough available slots
	for busNumber, volumeNames := range volumeAssignments {
		controller := specControllers[busNumber]

		// Get current device count from status
		currentDevices := statusDeviceCounts[busNumber]

		// Calculate total required slots
		requiredSlots := int32(len(volumeNames)) //nolint:gosec // disable G115
		totalDevices := currentDevices + requiredSlots

		if totalDevices > controller.maxSlots {
			// Build error message with volume names
			allErrs = append(allErrs, field.Forbidden(
				volumesPath,
				fmt.Sprintf(invalidControllerCapacityFmt,
					busNumber, controller.ctrlType, currentDevices, requiredSlots, volumeNames, controller.maxSlots)))
		}
	}

	return allErrs
}

// controllerInfo holds information about a controller.
type controllerInfo struct {
	busNumber   int32
	ctrlType    vmopv1.SCSIControllerType
	sharingMode vmopv1.VirtualControllerSharingMode
	maxSlots    int32
	usedSlots   int32
}

// getMaxSlotsForControllerType returns the maximum number of slots for a controller type.
func getMaxSlotsForControllerType(ctrlType vmopv1.SCSIControllerType) int32 {
	switch ctrlType {
	case vmopv1.SCSIControllerTypeParaVirtualSCSI:
		return maxParaVirtualSCSISlots
	case vmopv1.SCSIControllerTypeBusLogic:
		return maxBusLogicSlots
	case vmopv1.SCSIControllerTypeLsiLogic:
		return maxLsiLogicSlots
	case vmopv1.SCSIControllerTypeLsiLogicSAS:
		return maxLsiLogicSASSlots
	default:
		// Default to ParaVirtual if unknown
		return maxParaVirtualSCSISlots
	}
}

// findControllerWithSharing finds a controller with the specified sharing mode.
func findControllerWithSharing(
	sharingMode vmopv1.VirtualControllerSharingMode,
	controllers map[int32]controllerInfo,
) (controllerInfo, bool) {

	for busNum := range maxSCSIControllers {
		if controller, exists := controllers[busNum]; exists {
			if controller.sharingMode == sharingMode {
				return controller, true
			}
		}
	}

	return controllerInfo{}, false
}
