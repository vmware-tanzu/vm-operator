// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	invalidControllerBusNumberRangeFmt     = "must be between 0 and %d"
	invalidControllerBusNumberDoesNotExist = "controller %s:%d does not exist"
	invalidUnitNumberReserved              = "unit number %d is reserved for the %s controller itself"
	invalidUnitNumberRangeFmt              = "unit number must be between 0 and %d for %s controller"
	invalidControllerCapacityFmt           = "controller %s:%d full, maxDevices: %d"
	invalidUnitNumberInUse                 = "controller unit number %s:%d:%d is already in use"
	invalidControllerBusNumberZero         = "bus number 0 is reserved for the default controller"
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

	var allErrs field.ErrorList
	volumesPath := field.NewPath("spec", "hardware")

	// Validate that when creating a VM, the bus number is not 0 because
	// it is reserved for the default controller.

	for _, controller := range vm.Spec.Hardware.SCSIControllers {
		if oldVM == nil && controller.BusNumber == 0 {
			allErrs = append(allErrs, field.Invalid(
				volumesPath.Child("scsiControllers", "busNumber"),
				controller.BusNumber,
				invalidControllerBusNumberZero,
			))
			continue
		}
	}

	for _, controller := range vm.Spec.Hardware.SATAControllers {
		if oldVM == nil && controller.BusNumber == 0 {
			allErrs = append(allErrs, field.Invalid(
				volumesPath.Child("sataControllers", "busNumber"),
				controller.BusNumber,
				invalidControllerBusNumberZero,
			))
			continue
		}
	}

	for _, controller := range vm.Spec.Hardware.NVMEControllers {
		if oldVM == nil && controller.BusNumber == 0 {
			allErrs = append(allErrs, field.Invalid(
				volumesPath.Child("nvmeControllers", "busNumber"),
				controller.BusNumber,
				invalidControllerBusNumberZero,
			))
			continue
		}
	}

	for _, controller := range vm.Spec.Hardware.IDEControllers {
		if oldVM == nil && controller.BusNumber == 0 {
			allErrs = append(allErrs, field.Invalid(
				volumesPath.Child("ideControllers", "busNumber"),
				controller.BusNumber,
				invalidControllerBusNumberZero,
			))
			continue
		}
	}

	allErrs = append(allErrs, v.validateControllerSlots(ctx, vm)...)

	return allErrs
}

func (v validator) validateControllerSlots(
	ctx *pkgctx.WebhookRequestContext,
	vm *vmopv1.VirtualMachine) field.ErrorList {

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		return nil
	}

	var allErrs field.ErrorList

	volumes := vmopv1util.GetManagedVolumesWithPVC(*vm)
	if len(volumes) == 0 {
		return allErrs
	}

	volumesPath := field.NewPath("spec", "volumes")

	var (
		// specControllers builds a map of controller ID to controller
		// specification from spec.hardware.controllers.
		specControllers = vmopv1util.BuildVMControllersMap(*vm)

		// occupiedSlots builds a map of controller ID to a map of unit numbers
		// that are already in use on the specified controller.
		occupiedSlots = make(map[pkgutil.ControllerID]map[int32]struct{})
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
				occupiedSlots[controllerID] = make(map[int32]struct{})
			}
			occupiedSlots[controllerID][*cdrom.UnitNumber] = struct{}{}
		}
	}

	for i, vol := range vm.Spec.Volumes {
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		pvc := vol.PersistentVolumeClaim
		volPath := volumesPath.Index(i)

		if pvc.ControllerBusNumber == nil ||
			pvc.ControllerType == "" ||
			pvc.UnitNumber == nil {
			// These fields are validated by the virtualmachine validator's
			// validateVolumes if they are required.
			continue
		}

		controllerKey := pkgutil.ControllerID{
			ControllerType: pvc.ControllerType,
			BusNumber:      *pvc.ControllerBusNumber,
		}

		maxBusNumber := pvc.ControllerType.MaxCount()

		// Validate bus number is within valid range for the controller type.
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

		if targetController, exists = specControllers[controllerKey.ControllerType][controllerKey.BusNumber]; !exists {
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

		// Validate unit number is specified.
		var (
			unitNum      = *pvc.UnitNumber
			reservedUnit = targetController.ReservedUnitNumber()
			maxSlots     = targetController.MaxSlots()
		)

		// Validate unit number is within range for controller type.
		if unitNum < 0 || unitNum >= maxSlots {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("persistentVolumeClaim", "unitNumber"),
				unitNum,
				fmt.Sprintf(invalidUnitNumberRangeFmt,
					maxSlots-1, controllerKey.ControllerType)))
			continue

		} else if unitNum == reservedUnit {
			// Validate unit number is not reserved for the controller itself.
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("persistentVolumeClaim", "unitNumber"),
				unitNum,
				fmt.Sprintf(invalidUnitNumberReserved,
					reservedUnit,
					controllerKey.ControllerType)),
			)
			continue
		}

		if occupiedSlots[controllerKey] == nil {
			occupiedSlots[controllerKey] = make(map[int32]struct{})
		}

		// Validate unit number is not already in use.
		if _, exists := occupiedSlots[controllerKey][unitNum]; exists {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("persistentVolumeClaim", "unitNumber"),
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
		//nolint:gosec // disable G115 - len() result is bounded by MaxSlots which is int32.
		if int32(len(occupiedSlots[controllerKey])) >= targetController.MaxSlots() {
			allErrs = append(allErrs, field.Invalid(
				volPath.Child("persistentVolumeClaim", "unitNumber"),
				unitNum,
				fmt.Sprintf(invalidControllerCapacityFmt,
					controllerKey.ControllerType,
					controllerKey.BusNumber,
					targetController.MaxSlots())))
		}

		// Mark this slot as occupied for subsequent volumes.
		occupiedSlots[controllerKey][unitNum] = struct{}{}
	}

	return allErrs
}
