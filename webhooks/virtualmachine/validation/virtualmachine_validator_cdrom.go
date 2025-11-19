// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// validateCdromControllerSpecsOnUpdate validates CD-ROM controller specifications
// for update operations by iterating through the provided CD-ROM specs.
func (v validator) validateCdromControllerSpecsOnUpdate(
	newCD []vmopv1.VirtualMachineCdromSpec,
	f *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	if len(newCD) == 0 {
		return allErrs
	}

	for i, cdrom := range newCD {
		cdromFieldPath := f.Index(i)
		allErrs = append(allErrs, v.validateControllerSpecOnUpdate(cdrom, cdromFieldPath)...)
	}

	allErrs = append(allErrs, v.validateDuplicateUnitNumbers(newCD, f)...)

	return allErrs
}

// validateControllerSpecOnUpdate validates a single CD-ROM controller specification
// for update operations. It ensures all required fields are present and validates
// the controller type, bus number, and unit number ranges according to the
// controller type (IDE or SATA).
func (v validator) validateControllerSpecOnUpdate(
	cdrom vmopv1.VirtualMachineCdromSpec,
	fieldPath *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	if cdrom.ControllerType == "" {
		allErrs = append(allErrs,
			field.Required(fieldPath.Child("controllerType"), ""))
	}
	if cdrom.ControllerBusNumber == nil {
		allErrs = append(allErrs,
			field.Required(fieldPath.Child("controllerBusNumber"), ""))
	}
	if cdrom.UnitNumber == nil {
		allErrs = append(allErrs,
			field.Required(fieldPath.Child("unitNumber"), ""))
	}

	// Only validate ranges if all required fields are present.
	if cdrom.ControllerType == "" || cdrom.ControllerBusNumber == nil ||
		cdrom.UnitNumber == nil {
		return allErrs
	}

	var (
		ctrlType = cdrom.ControllerType
		busNum   = *cdrom.ControllerBusNumber
		unitNum  = *cdrom.UnitNumber
	)

	switch ctrlType {
	case vmopv1.VirtualControllerTypeIDE:
		if busNum < 0 || busNum >= pkgutil.IDEControllerMaxCount {
			allErrs = append(allErrs, field.Invalid(
				fieldPath.Child("controllerBusNumber"),
				busNum,
				fmt.Sprintf(controllerBusNumberRangeFmt, ctrlType, pkgutil.IDEControllerMaxCount-1)))
		}
		if unitNum < 0 || unitNum >= pkgutil.IDEControllerMaxSlotCount {
			allErrs = append(allErrs, field.Invalid(
				fieldPath.Child("unitNumber"),
				unitNum,
				fmt.Sprintf(unitNumberRangeFmt, ctrlType, pkgutil.IDEControllerMaxSlotCount-1)))
		}
	case vmopv1.VirtualControllerTypeSATA:
		if busNum < 0 || busNum >= pkgutil.SATAControllerMaxCount {
			allErrs = append(allErrs, field.Invalid(
				fieldPath.Child("controllerBusNumber"),
				busNum,
				fmt.Sprintf(controllerBusNumberRangeFmt, ctrlType, pkgutil.SATAControllerMaxCount-1)))
		}
		if unitNum < 0 || unitNum >= pkgutil.SATAControllerMaxSlotCount {
			allErrs = append(allErrs, field.Invalid(
				fieldPath.Child("unitNumber"),
				unitNum,
				fmt.Sprintf(unitNumberRangeFmt, ctrlType, pkgutil.SATAControllerMaxSlotCount-1)))
		}
	}

	return allErrs
}

// validateDuplicateUnitNumbers checks for duplicate unit numbers within the same
// controller. It uses ControllerID to uniquely identify controllers and reports
// field.Duplicate errors for any unit numbers that are already in use.
func (v validator) validateDuplicateUnitNumbers(
	cdroms []vmopv1.VirtualMachineCdromSpec,
	f *field.Path) field.ErrorList {

	var allErrs field.ErrorList
	controllerUnitNumbers := make(map[pkgutil.ControllerID]map[int32]bool)

	for i, cdrom := range cdroms {
		if cdrom.ControllerType == "" || cdrom.ControllerBusNumber == nil || cdrom.UnitNumber == nil {
			continue
		}

		controllerID := pkgutil.ControllerID{
			ControllerType: cdrom.ControllerType,
			BusNumber:      *cdrom.ControllerBusNumber,
		}
		unitNumber := *cdrom.UnitNumber

		if controllerUnitNumbers[controllerID] == nil {
			controllerUnitNumbers[controllerID] = make(map[int32]bool)
		}

		if controllerUnitNumbers[controllerID][unitNumber] {
			allErrs = append(allErrs, field.Duplicate(
				f.Index(i).Child("unitNumber"), unitNumber))
		} else {
			controllerUnitNumbers[controllerID][unitNumber] = true
		}
	}

	return allErrs
}

// validateCdromControllerSpecsOnCreate validates CD-ROM controller specifications
// for create operations. It ensures that controllerType and controllerBusNumber
// are either both set or both omitted. This validation allows the mutation webhook
// to auto-assign controller fields when they are completely omitted, while catching
// partial specifications that would be invalid.
func (v validator) validateCdromControllerSpecsOnCreate(
	cdroms []vmopv1.VirtualMachineCdromSpec,
	f *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	for i, cdrom := range cdroms {
		cdromFieldPath := f.Index(i)

		hasControllerType := cdrom.ControllerType != ""
		hasControllerBusNumber := cdrom.ControllerBusNumber != nil

		if hasControllerType && !hasControllerBusNumber {
			allErrs = append(allErrs, field.Required(
				cdromFieldPath.Child("controllerBusNumber"),
				"must be set when controllerType is specified"))
		} else if !hasControllerType && hasControllerBusNumber {
			allErrs = append(allErrs, field.Required(
				cdromFieldPath.Child("controllerType"),
				"must be set when controllerBusNumber is specified"))
		}
	}

	return allErrs
}

// validateCdromControllerSpecWhenPoweredOn validates CD-ROM controller
// specifications when the VM is powered on.
func (v validator) validateCdromControllerSpecWhenPoweredOn(
	newCD, oldCD []vmopv1.VirtualMachineCdromSpec,
	f *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	oldCdromByName := make(map[string]vmopv1.VirtualMachineCdromSpec, len(oldCD))
	for _, c := range oldCD {
		oldCdromByName[c.Name] = c
	}

	for i, newCdrom := range newCD {
		cdromFieldPath := f.Index(i)
		oldCdrom, exists := oldCdromByName[newCdrom.Name]

		if !exists {
			continue
		}

		if newCdrom.ControllerType != oldCdrom.ControllerType {
			allErrs = append(allErrs, field.Forbidden(
				cdromFieldPath.Child("controllerType"),
				updatesNotAllowedWhenPowerOn))
		}

		if !ptr.Equal(newCdrom.ControllerBusNumber, oldCdrom.ControllerBusNumber) {
			allErrs = append(allErrs, field.Forbidden(
				cdromFieldPath.Child("controllerBusNumber"),
				updatesNotAllowedWhenPowerOn))
		}

		if !ptr.Equal(newCdrom.UnitNumber, oldCdrom.UnitNumber) {
			allErrs = append(allErrs, field.Forbidden(
				cdromFieldPath.Child("unitNumber"),
				updatesNotAllowedWhenPowerOn))
		}
	}

	return allErrs
}
