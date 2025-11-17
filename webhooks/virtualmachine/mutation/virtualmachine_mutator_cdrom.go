// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	callOnUpdateOnlyMessage             = "MutateCdromControllerOnUpdate should only be called on update"
	skippedNoControllerMessage          = "Skipping CD-ROM: no available controller slots"
	skippedNoSlotMessage                = "Skipping CD-ROM: no available slot on controller"
	skippedPartialPlacementMessage      = "Skipping CD-ROM: partial placement not supported"
	skippedInvalidControllerTypeMessage = "Skipping CD-ROM: controller type not supported for CD-ROMs"
)

// MutateCdromControllerOnUpdate mutates CD-ROM controller assignments for
// VirtualMachine updates. It processes explicit placements (controller type,
// bus number, and unit number all specified) first to reserve their slots,
// then assigns implicit placements. For implicit placements, if controller type
// and bus number are specified, it assigns the next available unit number on
// that controller. If all fields are empty, it auto-assigns by trying IDE
// controllers first, then falling back to SATA controllers.
func MutateCdromControllerOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		return false, nil
	}

	if oldVM == nil {
		ctx.Logger.Info(callOnUpdateOnlyMessage)
		return false, nil
	}

	if !vmopv1util.IsVirtualMachineSchemaUpgraded(ctx, *oldVM) {
		return false, nil
	}

	if vm.Spec.Hardware == nil || len(vm.Spec.Hardware.Cdrom) == 0 {
		return false, nil
	}

	var (
		hwSpec         = vm.Spec.Hardware
		wasMutated     bool
		occupiedSlots  = constructOccupiedSlots(vm)
		explicitCdroms []*vmopv1.VirtualMachineCdromSpec
		implicitCdroms []*vmopv1.VirtualMachineCdromSpec
	)

	for i := range hwSpec.Cdrom {
		cdrom := &hwSpec.Cdrom[i]

		if cdrom.ControllerType != vmopv1.VirtualControllerTypeIDE &&
			cdrom.ControllerType != vmopv1.VirtualControllerTypeSATA &&
			cdrom.ControllerType != "" {
			ctx.Logger.Info(skippedInvalidControllerTypeMessage,
				"cdrom", cdrom.Name,
				"controllerType", cdrom.ControllerType)
			continue
		}

		if cdrom.ControllerBusNumber != nil &&
			cdrom.ControllerType != "" &&
			cdrom.UnitNumber != nil {

			explicitCdroms = append(explicitCdroms, cdrom)
		} else {
			implicitCdroms = append(implicitCdroms, cdrom)
		}
	}

	if processExplicitCdroms(explicitCdroms, occupiedSlots, hwSpec) {
		wasMutated = true
	}

	if processImplicitCdroms(ctx, implicitCdroms, occupiedSlots, hwSpec) {
		wasMutated = true
	}

	return wasMutated, nil
}

// processExplicitCdroms processes CD-ROMs with complete placement information
// (controller type, bus number, and unit number all specified). It creates
// controllers if they don't exist and marks their slots as occupied. Returns
// true if any controllers were added.
func processExplicitCdroms(
	explicitPlacementCdroms []*vmopv1.VirtualMachineCdromSpec,
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
	hwSpec *vmopv1.VirtualMachineHardwareSpec) bool {

	var mutated bool

	for _, cdrom := range explicitPlacementCdroms {
		if addControllerAndInitSlotMap(
			occupiedSlots,
			hwSpec,
			cdrom.ControllerType,
			cdrom.ControllerBusNumber) {

			mutated = true
		}
		occupiedSlots[pkgutil.ControllerID{
			ControllerType: cdrom.ControllerType,
			BusNumber:      *cdrom.ControllerBusNumber,
		}].Insert(*cdrom.UnitNumber)
	}

	return mutated
}

// processImplicitCdroms processes CD-ROMs that need placement assignment.
// It handles partial placements (controller type and bus specified) and full
// auto-assignment (all fields empty). For full auto-assignment, it tries IDE
// controllers first, then falls back to SATA controllers. Returns true if any
// CD-ROMs were assigned.
func processImplicitCdroms(
	ctx *pkgctx.WebhookRequestContext,
	implicitPlacementCdroms []*vmopv1.VirtualMachineCdromSpec,
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
	hwSpec *vmopv1.VirtualMachineHardwareSpec) bool {

	var mutated bool

	for _, cdrom := range implicitPlacementCdroms {

		var (
			desiredCtrl = pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      int32(-1),
			}
		)

		switch {
		case cdrom.ControllerBusNumber != nil &&
			cdrom.ControllerType != "" &&
			cdrom.UnitNumber == nil:
			// Partial placement with controller information provided.

			desiredCtrl = pkgutil.ControllerID{
				ControllerType: cdrom.ControllerType,
				BusNumber:      *cdrom.ControllerBusNumber,
			}

		case cdrom.ControllerBusNumber == nil &&
			cdrom.ControllerType == "" &&
			cdrom.UnitNumber == nil:
			// All controller fields are omitted,
			// auto-assign by trying IDE first, then SATA.

			desiredCtrl.BusNumber = findNextAvailableBusNumber(
				occupiedSlots,
				desiredCtrl.ControllerType)

			if desiredCtrl.BusNumber == -1 {
				desiredCtrl.ControllerType = vmopv1.VirtualControllerTypeSATA
				desiredCtrl.BusNumber = findNextAvailableBusNumber(
					occupiedSlots,
					desiredCtrl.ControllerType)
			}

			if desiredCtrl.BusNumber == -1 {
				ctx.Logger.Info(skippedNoControllerMessage,
					"cdrom", cdrom.Name,
					"attemptedControllers", []string{
						string(vmopv1.VirtualControllerTypeIDE),
						string(vmopv1.VirtualControllerTypeSATA),
					})
				continue
			}

		default:
			// Skip mutation for other partial configurations.
			ctx.Logger.Info(skippedPartialPlacementMessage,
				"cdrom", cdrom.Name,
				"hasControllerType", cdrom.ControllerType != "",
				"hasBusNumber", cdrom.ControllerBusNumber != nil,
				"hasUnitNumber", cdrom.UnitNumber != nil)
			continue
		}

		if addControllerAndInitSlotMap(
			occupiedSlots,
			hwSpec,
			desiredCtrl.ControllerType,
			&desiredCtrl.BusNumber) {

			mutated = true
		}

		desiredUnitNum := vmopv1util.NextAvailableUnitNumber(
			vmopv1util.CreateNewController(
				desiredCtrl.ControllerType,
				desiredCtrl.BusNumber,
				""),
			occupiedSlots[desiredCtrl])

		if desiredUnitNum == -1 {
			ctx.Logger.Info(skippedNoSlotMessage,
				"cdrom", cdrom.Name,
				"controllerType", desiredCtrl.ControllerType,
				"busNumber", desiredCtrl.BusNumber)
			continue
		}

		occupiedSlots[desiredCtrl].Insert(desiredUnitNum)
		cdrom.ControllerType = desiredCtrl.ControllerType
		cdrom.ControllerBusNumber = &desiredCtrl.BusNumber
		cdrom.UnitNumber = &desiredUnitNum

		mutated = true
	}

	return mutated
}

// addControllerAndInitSlotMap adds a controller to the hardware spec and
// initializes its slot map if it doesn't already exist. Returns true if a
// controller was added.
func addControllerAndInitSlotMap(
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
	hwSpec *vmopv1.VirtualMachineHardwareSpec,
	ctrlType vmopv1.VirtualControllerType,
	busNumber *int32) bool {

	if busNumber == nil {
		return false
	}

	ctrlID := pkgutil.ControllerID{
		ControllerType: ctrlType,
		BusNumber:      *busNumber,
	}

	if _, exists := occupiedSlots[ctrlID]; exists {
		return false
	}

	switch ctrlType {
	case vmopv1.VirtualControllerTypeIDE:
		hwSpec.IDEControllers = append(hwSpec.IDEControllers,
			vmopv1.IDEControllerSpec{BusNumber: *busNumber})
	case vmopv1.VirtualControllerTypeSATA:
		hwSpec.SATAControllers = append(hwSpec.SATAControllers,
			vmopv1.SATAControllerSpec{BusNumber: *busNumber})
	default:
		return false
	}

	occupiedSlots[ctrlID] = sets.New[int32]()
	return true
}

// findNextAvailableBusNumber finds the next available bus number for the
// specified controller type. It first tries to find existing controllers with
// available slots, then looks for unused bus numbers to create new controllers.
// Returns -1 if no bus numbers are available.
func findNextAvailableBusNumber(
	occupiedSlots map[pkgutil.ControllerID]sets.Set[int32],
	ctrlType vmopv1.VirtualControllerType) int32 {

	firstAvailableBusNumber := int32(-1)

	for busNum := range ctrlType.MaxCount() {
		ctrlID := pkgutil.ControllerID{
			ControllerType: ctrlType,
			BusNumber:      busNum,
		}

		if _, exists := occupiedSlots[ctrlID]; exists {
			unitNum := vmopv1util.NextAvailableUnitNumber(
				vmopv1util.CreateNewController(ctrlType, busNum, ""),
				occupiedSlots[ctrlID])
			if unitNum != -1 {
				return busNum
			}
		} else if firstAvailableBusNumber == -1 {
			firstAvailableBusNumber = busNum
		}
	}

	return firstAvailableBusNumber
}

// constructOccupiedSlots builds a map of occupied controller slots from the
// VM's hardware spec and status. It tracks which slots are occupied by
// non-CD-ROM devices to avoid conflicts when assigning new CD-ROM devices.
func constructOccupiedSlots(
	vm *vmopv1.VirtualMachine) map[pkgutil.ControllerID]sets.Set[int32] {

	occupiedSlots := make(map[pkgutil.ControllerID]sets.Set[int32])

	if vm.Spec.Hardware == nil {
		return occupiedSlots
	}

	// Initialize sets for each controller in the spec.
	for _, controller := range vm.Spec.Hardware.IDEControllers {
		ctrlID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeIDE,
			BusNumber:      controller.BusNumber,
		}
		occupiedSlots[ctrlID] = sets.New[int32]()
	}

	for _, controller := range vm.Spec.Hardware.SATAControllers {
		ctrlID := pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSATA,
			BusNumber:      controller.BusNumber,
		}
		occupiedSlots[ctrlID] = sets.New[int32]()
	}

	if vm.Status.Hardware == nil {
		return occupiedSlots
	}

	// Mark slots occupied by non-CD-ROM devices.
	for _, ctrl := range vm.Status.Hardware.Controllers {
		if ctrl.Type != vmopv1.VirtualControllerTypeIDE &&
			ctrl.Type != vmopv1.VirtualControllerTypeSATA {
			continue
		}

		ctrlID := pkgutil.ControllerID{
			ControllerType: ctrl.Type,
			BusNumber:      ctrl.BusNumber,
		}

		// Skip if the controller is not in the occupied slots map since
		// we are treating controllers in the spec as the source of truth.
		if _, exists := occupiedSlots[ctrlID]; !exists {
			continue
		}

		for _, dev := range ctrl.Devices {
			if dev.Type != vmopv1.VirtualDeviceTypeCDROM {
				occupiedSlots[ctrlID].Insert(dev.UnitNumber)
			}
		}
	}

	return occupiedSlots
}
