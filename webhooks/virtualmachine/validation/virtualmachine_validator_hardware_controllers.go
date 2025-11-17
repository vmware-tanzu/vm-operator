// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	invalidControllerBusNumberRangeFmt     = "must be between 0 and %d"
	invalidControllerBusNumberDoesNotExist = "controller %s:%d does not exist"
	invalidUnitNumberReserved              = "unit number %d is reserved for the %s controller itself"
	invalidUnitNumberRangeFmt              = "unit number must be between 0 and %d for %s controller"
	invalidControllerCapacityFmt           = "controller %s:%d full, maxDevices: %d"
	invalidUnitNumberInUse                 = "controller unit number %s:%d:%d is already in use"
	invalidControllerBusNumberZero         = "bus number 0 is reserved for the default controller"
	invalidControllersCountFmt             = "must have exactly %d controllers"
)

// validateControllers validates that all volumes are attached
// to controllers and that the number of devices per controller does not
// exceed the maximum number of devices per controller.
// We are not trying to be extensive here. The idea is to only perform
// basic validations to the extent we can, and fall through to the
// controller. It is possible that the VM is changed between the
// webhook check and actual reconciliation, so this is best effort
// anyway. We verify:
//   - If a controllerBusNumber is specified, it must not be invalid
//     (beyond max allowed).
//   - If a controllerBusNumber is specified, there must be an entry in
//     spec.hardware.controllers for this controller.
//   - If a unitNumber is specified, it must not be invalid (beyond max allowed).
//   - If a unitNumber is specified, it must not be reserved for the controller itself.
//   - If a unitNumber is specified, it must not be already in use.
func (v validator) validateControllers(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine) field.ErrorList {

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		return nil
	}

	if vm.Spec.Hardware == nil {
		return nil
	}

	var allErrs field.ErrorList
	hwPath := field.NewPath("spec", "hardware")

	// Validate that when creating a VM, the bus number is not 0 because
	// it is reserved for the default controller.
	if oldVM == nil {
		for i, controller := range vm.Spec.Hardware.SCSIControllers {
			if controller.BusNumber == 0 {
				allErrs = append(allErrs, field.Invalid(
					hwPath.Child("scsiControllers").Index(i).Child("busNumber"),
					controller.BusNumber,
					invalidControllerBusNumberZero,
				))
			}
		}

		for i, controller := range vm.Spec.Hardware.SATAControllers {
			if controller.BusNumber == 0 {
				allErrs = append(allErrs, field.Invalid(
					hwPath.Child("sataControllers").Index(i).Child("busNumber"),
					controller.BusNumber,
					invalidControllerBusNumberZero,
				))
			}
		}

		for i, controller := range vm.Spec.Hardware.NVMEControllers {
			if controller.BusNumber == 0 {
				allErrs = append(allErrs, field.Invalid(
					hwPath.Child("nvmeControllers").Index(i).Child("busNumber"),
					controller.BusNumber,
					invalidControllerBusNumberZero,
				))
			}
		}
	}

	maxIDEControllers := int(vmopv1.VirtualControllerTypeIDE.MaxCount())
	numIDEControllers := len(vm.Spec.Hardware.IDEControllers)
	if numIDEControllers != maxIDEControllers {
		allErrs = append(allErrs, field.Invalid(
			hwPath.Child("ideControllers"),
			fmt.Sprintf("%d controllers", numIDEControllers),
			fmt.Sprintf(invalidControllersCountFmt, maxIDEControllers),
		))
	}

	allErrs = append(allErrs, v.validateControllerSlots(ctx, vm)...)

	return allErrs
}

func (v validator) validateControllerSlots(
	_ *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	var allErrs field.ErrorList

	if len(vm.Spec.Volumes) == 0 {
		return allErrs
	}

	var (
		// controllerSpecs is a collection of controller specifications from
		// the VM's spec.hardware.controllers.
		controllerSpecs = vmopv1util.NewControllerSpecs(*vm)

		// occupiedSlots builds a map of controller ID to a map of unit numbers
		// that are already in use on the specified controller.
		occupiedSlots = make(map[pkgutil.ControllerID]sets.Set[int32])
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

	volumesPath := field.NewPath("spec", "volumes")
	for i, vol := range vm.Spec.Volumes {
		volPath := volumesPath.Index(i)

		if vol.ControllerBusNumber == nil ||
			vol.ControllerType == "" ||
			vol.UnitNumber == nil {
			// These fields are validated by the virtualmachine validator's
			// validateVolumes if they are required.
			continue
		}

		controllerKey := pkgutil.ControllerID{
			ControllerType: vol.ControllerType,
			BusNumber:      *vol.ControllerBusNumber,
		}

		maxBusNumber := vol.ControllerType.MaxCount()

		// Validate bus number is within valid range for the controller type.
		if controllerKey.BusNumber < 0 ||
			controllerKey.BusNumber >= maxBusNumber {

			allErrs = append(allErrs, field.Invalid(
				volPath.Child("controllerBusNumber"),
				controllerKey.BusNumber,
				fmt.Sprintf(invalidControllerBusNumberRangeFmt,
					maxBusNumber-1)))
			continue
		}

		targetController, exists := controllerSpecs.Get(controllerKey.ControllerType, controllerKey.BusNumber)
		if !exists {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("controllerBusNumber"),
				controllerKey.BusNumber,
				fmt.Sprintf(invalidControllerBusNumberDoesNotExist,
					controllerKey.ControllerType,
					controllerKey.BusNumber,
				)),
			)
			continue
		}

		// Validate unit number is specified.
		var (
			unitNum      = *vol.UnitNumber
			reservedUnit = targetController.ReservedUnitNumber()
			maxSlots     = targetController.MaxSlots()
		)

		// Validate unit number is within range for controller type.
		if unitNum < 0 || unitNum >= maxSlots {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("unitNumber"),
				unitNum,
				fmt.Sprintf(invalidUnitNumberRangeFmt,
					maxSlots-1, controllerKey.ControllerType)))
			continue

		} else if unitNum == reservedUnit {
			// Validate unit number is not reserved for the controller itself.
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("unitNumber"),
				unitNum,
				fmt.Sprintf(invalidUnitNumberReserved,
					reservedUnit,
					controllerKey.ControllerType)),
			)
			continue
		}

		if occupiedSlots[controllerKey] == nil {
			occupiedSlots[controllerKey] = sets.New[int32]()
		}

		// Validate unit number is not already in use.
		if occupiedSlots[controllerKey].Has(unitNum) {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("unitNumber"),
				unitNum,
				fmt.Sprintf(invalidUnitNumberInUse,
					controllerKey.ControllerType,
					controllerKey.BusNumber,
					unitNum,
				)),
			)
			continue
		}

		// Validate controller is not at capacity after adding this volume.
		if occupiedSlots[controllerKey].Len() >= int(targetController.MaxSlots()) {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("unitNumber"),
				unitNum,
				fmt.Sprintf(invalidControllerCapacityFmt,
					controllerKey.ControllerType,
					controllerKey.BusNumber,
					targetController.MaxSlots())))
		}

		// Mark this slot as occupied for subsequent volumes.
		occupiedSlots[controllerKey].Insert(unitNum)
	}

	return allErrs
}
