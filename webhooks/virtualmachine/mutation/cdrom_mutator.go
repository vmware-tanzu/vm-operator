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

// ControllerSlot represents a controller slot position with bus number, unit
// number, and controller type. It is used to track available slots for CD-ROM
// devices on IDE and SATA controllers during mutation.
type ControllerSlot struct {
	BusNumber      int32
	UnitNumber     int32
	ControllerType vmopv1.VirtualControllerType
}

// MutateCdromControllerOnUpdate mutates CD-ROM controller assignments for
// VirtualMachine updates. It automatically assigns controller types, bus
// numbers, and unit numbers for CD-ROM devices when not specified.
func MutateCdromControllerOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm, _ *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		return false, nil
	}

	if vm.Spec.Hardware == nil {
		vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
	}

	var (
		mutated     bool
		hwSpec      = vm.Spec.Hardware
		usedSlotMap = constructUsedSlotMap(vm)
	)

	for i := range hwSpec.Cdrom {
		cdrom := &hwSpec.Cdrom[i]
		var availableSlot *ControllerSlot

		if cdrom.UnitNumber != nil {
			if cdrom.ControllerBusNumber != nil && cdrom.ControllerType != "" {
				ctrlID := pkgutil.ControllerID{
					ControllerType: cdrom.ControllerType,
					BusNumber:      *cdrom.ControllerBusNumber,
				}
				if addNewController(ctrlID, usedSlotMap, hwSpec) {
					mutated = true
				}
			}
			// Skip if unit number is set but controller information is
			// incomplete since we cannot identify the controller.
			continue
		}

		// Find available slot based on CD-ROM configuration
		availableSlot = findAvailableSlot(cdrom, usedSlotMap, hwSpec)

		if availableSlot == nil {
			continue
		}

		cdrom.ControllerType = availableSlot.ControllerType
		cdrom.ControllerBusNumber = &availableSlot.BusNumber
		cdrom.UnitNumber = &availableSlot.UnitNumber

		ctrlID := pkgutil.ControllerID{
			ControllerType: cdrom.ControllerType,
			BusNumber:      *cdrom.ControllerBusNumber,
		}
		usedSlotMap[ctrlID][*cdrom.UnitNumber] = true
		mutated = true
	}

	return mutated, nil
}

// findNextAvailableSATASlot finds the next available slot on a SATA controller.
// It returns a ControllerSlot with the bus number and unit number, or nil if no
// slots are available.
func findNextAvailableSATASlot(
	usedSlotMap map[pkgutil.ControllerID][]bool,
	hwSpec *vmopv1.VirtualMachineHardwareSpec) *ControllerSlot {

	var nextBusNum *int32

	for busNum := range pkgutil.SATAControllerMaxCount {
		ctrlID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSATA,
			BusNumber:      busNum,
		}
		if _, exists := usedSlotMap[ctrlID]; !exists {
			if nextBusNum == nil {
				nextBusNum = &busNum
			}
			continue
		}

		for unitNum := range pkgutil.SATAControllerMaxSlotCount {
			if !usedSlotMap[ctrlID][unitNum] {
				return &ControllerSlot{
					BusNumber:      busNum,
					UnitNumber:     unitNum,
					ControllerType: vmopv1.VirtualControllerTypeSATA,
				}
			}
		}
	}

	if nextBusNum != nil {
		addNewController(pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSATA,
			BusNumber:      *nextBusNum,
		}, usedSlotMap, hwSpec)
		return &ControllerSlot{
			BusNumber:      *nextBusNum,
			UnitNumber:     0,
			ControllerType: vmopv1.VirtualControllerTypeSATA,
		}
	}

	return nil
}

// findNextAvailableIDESlot finds the next available slot on an IDE controller.
// It returns a ControllerSlot with the bus number and unit number, or nil if no
// slots are available. If desiredBusNum is specified, it will try to use
// that specific bus number.
func findNextAvailableIDESlot(
	usedSlotMap map[pkgutil.ControllerID][]bool,
	hwSpec *vmopv1.VirtualMachineHardwareSpec,
	desiredBusNum *int32) *ControllerSlot {

	var nextBusNum *int32

	for busNum := range pkgutil.IDEControllerMaxCount {
		if desiredBusNum != nil && *desiredBusNum != busNum {
			continue
		}

		ctrlID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeIDE,
			BusNumber:      busNum,
		}
		if _, exists := usedSlotMap[ctrlID]; !exists {
			if nextBusNum == nil {
				nextBusNum = &busNum
			}
			continue
		}

		for unitNum := range pkgutil.IDEControllerMaxSlotCount {
			if !usedSlotMap[ctrlID][unitNum] {
				return &ControllerSlot{
					BusNumber:      busNum,
					UnitNumber:     unitNum,
					ControllerType: vmopv1.VirtualControllerTypeIDE,
				}
			}
		}
	}

	if desiredBusNum == nil && nextBusNum != nil {
		addNewController(pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeIDE,
			BusNumber:      *nextBusNum,
		}, usedSlotMap, hwSpec)
		return &ControllerSlot{
			BusNumber:      *nextBusNum,
			UnitNumber:     0,
			ControllerType: vmopv1.VirtualControllerTypeIDE,
		}
	}

	return nil
}

// addNewController adds a new controller to the hardware spec and initializes
// its slot map. It returns true if a new controller was added, false if it
// already existed.
func addNewController(
	ctrlID pkgutil.ControllerID,
	usedSlotMap map[pkgutil.ControllerID][]bool,
	hwSpec *vmopv1.VirtualMachineHardwareSpec) bool {

	if _, exists := usedSlotMap[ctrlID]; exists {
		return false
	}

	switch ctrlID.ControllerType {
	case vmopv1.VirtualControllerTypeIDE:
		hwSpec.IDEControllers = append(hwSpec.IDEControllers,
			vmopv1.IDEControllerSpec{BusNumber: ctrlID.BusNumber})
		usedSlotMap[ctrlID] = make([]bool, pkgutil.IDEControllerMaxSlotCount)
	case vmopv1.VirtualControllerTypeSATA:
		hwSpec.SATAControllers = append(hwSpec.SATAControllers,
			vmopv1.SATAControllerSpec{BusNumber: ctrlID.BusNumber})
		usedSlotMap[ctrlID] = make([]bool, pkgutil.SATAControllerMaxSlotCount)
	default:
		// This should never happen per the API schema validation.
		return false
	}

	return true
}

// constructUsedSlotMap builds a map of used controller slots from the VM's
// hardware spec and status. It initializes slot maps for IDE and SATA
// controllers and marks slots as used by non-CD-ROM devices to avoid
// conflicts when assigning new CD-ROM devices.
func constructUsedSlotMap(
	vm *vmopv1.VirtualMachine) map[pkgutil.ControllerID][]bool {

	usedSlotMap := make(map[pkgutil.ControllerID][]bool)

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

// findAvailableSlot finds an available controller slot for a CD-ROM device
// based on its configuration. It handles all combinations of missing controller
// information and returns the appropriate slot assignment.
func findAvailableSlot(
	cdrom *vmopv1.VirtualMachineCdromSpec,
	usedSlotMap map[pkgutil.ControllerID][]bool,
	hwSpec *vmopv1.VirtualMachineHardwareSpec) *ControllerSlot {

	if cdrom.ControllerBusNumber == nil && cdrom.ControllerType == "" {
		// When all controller information is omitted, try IDE first, then SATA
		availableSlot := findNextAvailableIDESlot(usedSlotMap, hwSpec, nil)
		if availableSlot == nil {
			availableSlot = findNextAvailableSATASlot(usedSlotMap, hwSpec)
		}
		return availableSlot
	}

	if cdrom.ControllerBusNumber == nil && cdrom.ControllerType != "" {
		// When controller type is defined, find slot for that type
		switch cdrom.ControllerType {
		case vmopv1.VirtualControllerTypeIDE:
			return findNextAvailableIDESlot(usedSlotMap, hwSpec, nil)
		case vmopv1.VirtualControllerTypeSATA:
			return findNextAvailableSATASlot(usedSlotMap, hwSpec)
		}
	}

	if cdrom.ControllerBusNumber != nil && cdrom.ControllerType == "" {
		// When bus number is set but type is not, default to IDE
		return findNextAvailableIDESlot(usedSlotMap, hwSpec,
			cdrom.ControllerBusNumber)
	}

	return nil
}
