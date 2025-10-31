// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	invalidControllerBusNumberRangeFmt     = "must be between 0 and %d"
	invalidControllerBusNumberDoesNotExist = "controller %s:%d does not exist"
	invalidUnitNumberReserved              = "unit number %d is reserved for the %s controller itself"
	invalidUnitNumberRangeFmt              = "unit number must be less than %d for %s controller"
	invalidControllerCapacityFmt           = "controller %s:%d full, maxDevices: %d"
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
//     spec.hardware.controllers for this controller.
//   - verify that we don't have more devices than max allowed for
//     each controller type.
func (v validator) validateControllerSlots(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if len(vm.Spec.Volumes) == 0 {
		return allErrs
	}

	volumesPath := field.NewPath("spec", "volumes")

	var (
		// Build controller map from spec.hardware.controllers
		specControllers = make(map[pkgutil.ControllerID]vmopv1util.ControllerSpec)

		// Build device count map from status.hardware.controllers
		statusDeviceCounts = make(map[pkgutil.ControllerID]int32)

		// Track volume assignments to controllers
		volumeAssignments = make(map[pkgutil.ControllerID]int32)
	)

	if vm.Spec.Hardware != nil {
		for _, controller := range vm.Spec.Hardware.SCSIControllers {
			controllerKey := pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSCSI,
				BusNumber:      controller.BusNumber,
			}
			specControllers[controllerKey] = controller
		}

		for _, controller := range vm.Spec.Hardware.SATAControllers {
			controllerKey := pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				BusNumber:      controller.BusNumber,
			}
			specControllers[controllerKey] = controller
		}
		
		for _, controller := range vm.Spec.Hardware.NVMEControllers {
			controllerKey := pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeNVME,
				BusNumber:      controller.BusNumber,
			}
			specControllers[controllerKey] = controller
		}
		
		for _, controller := range vm.Spec.Hardware.IDEControllers {
			controllerKey := pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      controller.BusNumber,
			}
			specControllers[controllerKey] = controller
		}
	}

	if vm.Status.Hardware != nil {
		for _, controller := range vm.Status.Hardware.Controllers {
			controllerKey := pkgutil.ControllerID{
				ControllerType: controller.Type,
				BusNumber:      controller.BusNumber,
			}
			statusDeviceCounts[controllerKey] = int32(len(controller.Devices)) //nolint:gosec // disable G115
		}
	}

	// Process each volume
	for i, vol := range vm.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		pvc := vol.PersistentVolumeClaim
		volPath := volumesPath.Index(i)

		// Check if a specific controller is requested
		if pvc.ControllerBusNumber == nil {
			allErrs = append(allErrs, field.Required(
				volPath.Child("persistentVolumeClaim", "controllerBusNumber"),
				"",
			))
			continue
		}

		if pvc.ControllerType == "" {
			allErrs = append(allErrs, field.Required(
				volPath.Child("persistentVolumeClaim", "controllerType"),
				"",
			))
			continue
		}

		controllerKey := pkgutil.ControllerID{
			ControllerType: pvc.ControllerType,
			BusNumber:      *pvc.ControllerBusNumber,
		}
		maxBusNumber := pvc.ControllerType.MaxPerVM()

		// Validate bus number range
		if controllerKey.BusNumber < 0 ||
			controllerKey.BusNumber >= maxBusNumber {

			allErrs = append(allErrs, field.Invalid(
				volPath.Child("persistentVolumeClaim", "controllerBusNumber"),
				controllerKey.BusNumber,
				fmt.Sprintf(invalidControllerBusNumberRangeFmt,
					maxBusNumber-1)))
			continue
		}

		var (
			targetController vmopv1util.ControllerSpec
			exists           bool
		)

		if targetController, exists = specControllers[controllerKey]; !exists {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("persistentVolumeClaim", "controllerBusNumber"),
				controllerKey.BusNumber,
				fmt.Sprintf(invalidControllerBusNumberDoesNotExist,
					controllerKey.ControllerType,
					controllerKey.BusNumber,
				)),
			)
			continue
		}

		// Validate unit number if specified
		if pvc.UnitNumber != nil {
			unitNum := *pvc.UnitNumber

			if unitNum == pvc.ControllerType.UnitNumber() {
				allErrs = append(allErrs, field.Invalid(
					volPath.Child("persistentVolumeClaim", "unitNumber"),
					unitNum,
					fmt.Sprintf(invalidUnitNumberReserved,
						pvc.ControllerType.UnitNumber(),
						pvc.ControllerType)),
				)
				continue
			}

			// Validate unit number is within range for controller type
			maxSlots := targetController.MaxSlots()
			if unitNum >= maxSlots {
				allErrs = append(allErrs, field.Invalid(
					volPath.Child("persistentVolumeClaim", "unitNumber"),
					unitNum,
					fmt.Sprintf(invalidUnitNumberRangeFmt,
						maxSlots, controllerKey.ControllerType)))
				continue
			}
		}

		// Track this volume assignment so we can figure out max
		// devices attached to a controller.
		volumeAssignments[controllerKey]++
	}

	// Validate that each controller has enough available slots
	for controllerKey, assignedVolumes := range volumeAssignments {
		controller := specControllers[controllerKey]

		// Get current device count from status
		currentDevices := statusDeviceCounts[controllerKey]

		// Calculate total required slots
		totalDevices := currentDevices + assignedVolumes

		maxSlots := controller.MaxSlots()
		if totalDevices > maxSlots {
			allErrs = append(allErrs, field.Forbidden(
				volumesPath,
				fmt.Sprintf(invalidControllerCapacityFmt,
					controllerKey.ControllerType,
					controllerKey.BusNumber,
					maxSlots)))
		}
	}

	return allErrs
}
