// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	couldNotFindSlotMessage    = "Could not find an available slot for CD-ROM device"
	incompletePlacementMessage = "CD-ROM with incomplete placement " +
		"information will be rejected by validating webhook"
)

// ControllerSlot represents a controller slot position with bus number, unit
// number, and controller type. It is used to track available slots for CD-ROM
// devices on IDE and SATA controllers during mutation.
type ControllerSlot struct {
	BusNumber      int32
	UnitNumber     int32
	ControllerType vmopv1.VirtualControllerType
}

// MutateCdromControllerOnUpdate mutates CD-ROM controller assignments for
// VirtualMachine updates. The mutator only handles two specific scenarios:
// complete omission (auto-assign) or
// explicit controller selection with existing controller (find unit number).
// It also updates the vm.spec.hardware.controllers field to reflect the
// new controller assignments.
func MutateCdromControllerOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm, _ *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		return false, nil
	}

	if vm.Spec.Hardware == nil {
		return false, nil
	}

	var (
		hwSpec      = vm.Spec.Hardware
		usedSlotMap = constructUsedSlotMap(vm)
		wasMutated  bool
	)

	for i := range hwSpec.Cdrom {
		cdrom := &hwSpec.Cdrom[i]

		var (
			mutated          bool
			placementDefined bool
			availableSlot    *ControllerSlot
		)

		switch {
		case cdrom.ControllerBusNumber != nil &&
			cdrom.ControllerType != "" &&
			cdrom.UnitNumber != nil:
			// When all placement values are specified, use the specified
			// values and add the controller if it doesn't exist.
			if addControllerAndInitSlotMap(
				usedSlotMap, hwSpec, cdrom.ControllerType, *cdrom.ControllerBusNumber) {
				wasMutated = true
			}
			availableSlot = &ControllerSlot{
				BusNumber:      *cdrom.ControllerBusNumber,
				UnitNumber:     *cdrom.UnitNumber,
				ControllerType: cdrom.ControllerType,
			}
			placementDefined = true

		case cdrom.ControllerBusNumber != nil &&
			cdrom.ControllerType != "" &&
			cdrom.UnitNumber == nil:
			// Controller type and bus number are specified,
			// find an available unit number.
			availableSlot, mutated = findNextAvailableSlot(
				usedSlotMap, hwSpec, cdrom.ControllerBusNumber, cdrom.ControllerType)
			if mutated {
				wasMutated = true
			}

		case cdrom.ControllerBusNumber == nil &&
			cdrom.ControllerType == "" &&
			cdrom.UnitNumber == nil:
			// All controller fields are omitted,
			// auto-assign by trying IDE first, then SATA.
			availableSlot, mutated = findNextAvailableSlot(
				usedSlotMap, hwSpec, nil, vmopv1.VirtualControllerTypeIDE)
			if mutated {
				wasMutated = true
			}

			if availableSlot == nil {
				availableSlot, mutated = findNextAvailableSlot(
					usedSlotMap, hwSpec, nil, vmopv1.VirtualControllerTypeSATA)
				if mutated {
					wasMutated = true
				}
			}

		default:
			// Skip mutation for other partial configurations.
			continue
		}

		if availableSlot == nil {
			ctx.Logger.Info(couldNotFindSlotMessage,
				"vmName", vm.Name,
				"vmNamespace", vm.Namespace,
				"cdromName", cdrom.Name,
				"cdromIndex", i,
				"message", incompletePlacementMessage)
			continue
		}

		cdrom.ControllerType = availableSlot.ControllerType
		cdrom.ControllerBusNumber = &availableSlot.BusNumber
		cdrom.UnitNumber = &availableSlot.UnitNumber

		ctrlID := pkgutil.ControllerID{
			ControllerType: availableSlot.ControllerType,
			BusNumber:      availableSlot.BusNumber,
		}

		switch availableSlot.ControllerType {
		case vmopv1.VirtualControllerTypeIDE:
			if availableSlot.UnitNumber < 0 ||
				availableSlot.UnitNumber >= pkgutil.IDEControllerMaxSlotCount {
				continue
			}
		case vmopv1.VirtualControllerTypeSATA:
			if availableSlot.UnitNumber < 0 ||
				availableSlot.UnitNumber >= pkgutil.SATAControllerMaxSlotCount {
				continue
			}
		default:
			// Skip unsupported controller types.
			continue
		}

		usedSlotMap[ctrlID][availableSlot.UnitNumber] = true

		if !placementDefined {
			wasMutated = true
		}
	}

	return wasMutated, nil
}

// getControllerLimits returns the maximum controller count and slot count for the
// specified controller type. Returns (0, 0) for unsupported controller types.
func getControllerLimits(controllerType vmopv1.VirtualControllerType) (
	maxControllerCount, maxSlotCount int32) {

	switch controllerType {
	case vmopv1.VirtualControllerTypeIDE:
		return pkgutil.IDEControllerMaxCount, pkgutil.IDEControllerMaxSlotCount
	case vmopv1.VirtualControllerTypeSATA:
		return pkgutil.SATAControllerMaxCount, pkgutil.SATAControllerMaxSlotCount
	default:
		return 0, 0
	}

}

// addControllerAndInitSlotMap adds a controller to the hardware spec and initializes
// its slot map if it doesn't already exist. Returns true if a controller was added.
func addControllerAndInitSlotMap(
	usedSlotMap map[pkgutil.ControllerID][]bool,
	hwSpec *vmopv1.VirtualMachineHardwareSpec,
	controllerType vmopv1.VirtualControllerType,
	busNumber int32) bool {

	ctrlID := pkgutil.ControllerID{
		ControllerType: controllerType,
		BusNumber:      busNumber,
	}

	// Check if controller already exists.
	if _, exists := usedSlotMap[ctrlID]; exists {
		return false
	}

	// Get the max slot count for this controller type.
	_, maxSlotCount := getControllerLimits(controllerType)
	if maxSlotCount == 0 {
		return false
	}

	// Initialize the slot map.
	usedSlotMap[ctrlID] = make([]bool, maxSlotCount)

	// Add the controller to the hardware spec.
	switch controllerType {
	case vmopv1.VirtualControllerTypeIDE:
		hwSpec.IDEControllers = append(hwSpec.IDEControllers,
			vmopv1.IDEControllerSpec{BusNumber: busNumber})
		return true
	case vmopv1.VirtualControllerTypeSATA:
		hwSpec.SATAControllers = append(hwSpec.SATAControllers,
			vmopv1.SATAControllerSpec{BusNumber: busNumber})
		return true
	}

	return false
}

// findNextAvailableSlot finds the next available slot on a controller of the
// specified type. If desiredBusNum is specified, it will add the controller
// if it doesn't exist. If desiredBusNum is nil (auto-assignment), it will
// create a new controller if needed and there is available bus.
func findNextAvailableSlot(
	usedSlotMap map[pkgutil.ControllerID][]bool,
	hwSpec *vmopv1.VirtualMachineHardwareSpec,
	desiredBusNum *int32,
	controllerType vmopv1.VirtualControllerType) (*ControllerSlot, bool) {

	var (
		wasMutated                       bool
		nextBusNum                       *int32
		maxControllerCount, maxSlotCount = getControllerLimits(controllerType)
	)

	if maxSlotCount == 0 {
		return nil, false
	}

	// Add the controller to the hardware spec if desired bus number is
	// specified and it is not in the spec.
	if desiredBusNum != nil && addControllerAndInitSlotMap(
		usedSlotMap, hwSpec, controllerType, *desiredBusNum) {
		wasMutated = true
	}

	for busNum := range maxControllerCount {

		// Skip if the bus number is not the desired bus number
		// when desired bus number is specified.
		if desiredBusNum != nil && *desiredBusNum != busNum {
			continue
		}

		ctrlID := pkgutil.ControllerID{
			ControllerType: controllerType,
			BusNumber:      busNum,
		}
		if _, exists := usedSlotMap[ctrlID]; !exists {
			if nextBusNum == nil {
				nextBusNum = &busNum
			}
			continue
		}

		for unitNum := range maxSlotCount {
			if !usedSlotMap[ctrlID][unitNum] {
				return &ControllerSlot{
					BusNumber:      busNum,
					UnitNumber:     unitNum,
					ControllerType: controllerType,
				}, wasMutated
			}
		}

	}

	if nextBusNum != nil {

		if addControllerAndInitSlotMap(
			usedSlotMap, hwSpec, controllerType, *nextBusNum) {
			wasMutated = true
		}

		return &ControllerSlot{
			BusNumber:      *nextBusNum,
			UnitNumber:     0,
			ControllerType: controllerType,
		}, wasMutated
	}

	return nil, wasMutated
}

// constructUsedSlotMap builds a map of used controller slots from the VM's
// hardware spec and status. It initializes slot maps for IDE and SATA
// controllers and marks slots as used by non-CD-ROM devices to avoid
// conflicts when assigning new CD-ROM devices.
func constructUsedSlotMap(
	vm *vmopv1.VirtualMachine) map[pkgutil.ControllerID][]bool {

	usedSlotMap := make(map[pkgutil.ControllerID][]bool)

	if vm.Spec.Hardware == nil {
		return usedSlotMap
	}

	for _, controller := range vm.Spec.Hardware.IDEControllers {
		usedSlotMap[pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeIDE,
			BusNumber:      controller.BusNumber,
		}] = make([]bool, pkgutil.IDEControllerMaxSlotCount)
	}

	for _, controller := range vm.Spec.Hardware.SATAControllers {
		usedSlotMap[pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSATA,
			BusNumber:      controller.BusNumber,
		}] = make([]bool, pkgutil.SATAControllerMaxSlotCount)
	}

	if vm.Status.Hardware == nil {
		return usedSlotMap
	}

	for _, ctrl := range vm.Status.Hardware.Controllers {
		if ctrl.Type != vmopv1.VirtualControllerTypeIDE &&
			ctrl.Type != vmopv1.VirtualControllerTypeSATA {
			continue
		}

		ctrlID := pkgutil.ControllerID{
			ControllerType: ctrl.Type,
			BusNumber:      ctrl.BusNumber,
		}

		// Skip if the controller is not in the used slot map since
		// we are treating controllers in the spec as the source of truth.
		if _, exists := usedSlotMap[ctrlID]; !exists {
			continue
		}

		for _, dev := range ctrl.Devices {
			if dev.Type != vmopv1.VirtualDeviceTypeCDROM {
				usedSlotMap[ctrlID][dev.UnitNumber] = true
			}
		}
	}

	return usedSlotMap
}
