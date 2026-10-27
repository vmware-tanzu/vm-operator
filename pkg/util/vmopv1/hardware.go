// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
)

const (
	MicrosoftWSFCControllerSharingMode = vmopv1.VirtualControllerSharingModePhysical
)

const (
	// NICUnitNumberFirst is the first PCI unit number an ethernet card can
	// occupy. The virtual PCI bus is shared with other devices that occupy
	// the lower slots, and ethernet cards are allocated units 7-16 by the
	// platform's static, per-device-class allocation.
	NICUnitNumberFirst = 7

	// NICUnitNumberMax is the last PCI unit number an ethernet card can
	// occupy.
	NICUnitNumberMax = 16
)

// ControllerSpec is an interface describing a controller specification.
type ControllerSpec interface {
	// MaxSlots returns the maximum number of slots per controller type.
	MaxSlots() int32

	// MaxCount returns the maximum number of controllers per VM.
	MaxCount() int32

	// ReservedUnitNumber returns any reserved unit numbers or negative one
	// if no reserved unit numbers are present.
	ReservedUnitNumber() int32
}

// firstUnitNumberer is an optional ControllerSpec extension. Controllers
// whose valid unit numbers do not start at zero implement it: FirstUnitNumber
// returns the first assignable unit number on the controller's bus, and
// NextAvailableUnitNumber scans from there. Controllers that start at zero
// (the default) need not implement it.
type firstUnitNumberer interface {
	FirstUnitNumber() int32
}

// NextAvailableUnitNumber returns the first available unit number for the
// specified controller. The occupiedSlots parameter should contain all unit
// numbers that are already in use on the specified bus.
// Returns the first available unit number, or negative one if no slots are
// available.
func NextAvailableUnitNumber(
	controller ControllerSpec,
	occupiedSlots sets.Set[int32],
) int32 {

	if controller == nil {
		return -1
	}

	// Controllers default to a first unit number of zero; only controllers
	// implementing firstUnitNumberer scan from a different base.
	first := int32(0)
	if f, ok := controller.(firstUnitNumberer); ok {
		first = f.FirstUnitNumber()
	}

	// MaxSlots is a count of slots, not an upper bound on unit numbers: the
	// scan covers first..first+MaxSlots-1.
	for unitNumber := first; unitNumber < first+controller.MaxSlots(); unitNumber++ {
		if _, exists := occupiedSlots[unitNumber]; !exists &&
			unitNumber != controller.ReservedUnitNumber() {
			return unitNumber
		}
	}
	return -1
}

// NICBusSpec implements ControllerSpec for the VM's single, implicit PCI bus
// hosting ethernet cards. The NIC bus has no spec object — the bus is always
// present — so this type exists only for unit-number slot computation, with
// no API surface. Ethernet cards own PCI units NICUnitNumberFirst through
// NICUnitNumberMax by the platform's static, per-device-class unit
// allocation, and no unit inside the band is reserved (unlike SCSI).
type NICBusSpec struct{}

// MaxSlots returns the number of unit numbers in the ethernet-card band.
func (NICBusSpec) MaxSlots() int32 {
	return NICUnitNumberMax - NICUnitNumberFirst + 1
}

// MaxCount returns the maximum number of NIC buses per VM: one implicit bus.
func (NICBusSpec) MaxCount() int32 {
	return 1
}

// ReservedUnitNumber returns -1: the NIC bus has no reserved unit number.
func (NICBusSpec) ReservedUnitNumber() int32 {
	return -1
}

// FirstUnitNumber returns the first unit number an ethernet card can occupy.
func (NICBusSpec) FirstUnitNumber() int32 {
	return NICUnitNumberFirst
}

// GenerateControllerID generates a controller ID from a controller specification.
// Returns a ControllerID with BusNumber set to negative one if the controller
// type is not supported.
func GenerateControllerID(
	controller any,
) pkgutil.ControllerID {

	if controller == nil {
		return pkgutil.ControllerID{BusNumber: -1}
	}

	switch c := controller.(type) {
	case vmopv1.SCSIControllerSpec:
		return pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSCSI,
			BusNumber:      c.BusNumber,
		}
	case vmopv1.SATAControllerSpec:
		return pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeSATA,
			BusNumber:      c.BusNumber,
		}
	case vmopv1.NVMEControllerSpec:
		return pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeNVME,
			BusNumber:      c.BusNumber,
		}
	case vmopv1.IDEControllerSpec:
		return pkgutil.ControllerID{
			ControllerType: vmopv1.VirtualControllerTypeIDE,
			BusNumber:      c.BusNumber,
		}
	default:
		return pkgutil.ControllerID{BusNumber: -1}
	}
}

// GetControllerSharingMode returns the sharing mode for a controller.
// Returns the sharing mode from the controller if supported, or None otherwise.
func GetControllerSharingMode(
	controller any,
) vmopv1.VirtualControllerSharingMode {
	switch c := controller.(type) {
	case vmopv1.SCSIControllerSpec:
		return c.SharingMode
	case vmopv1.NVMEControllerSpec:
		return c.SharingMode
	}
	return vmopv1.VirtualControllerSharingModeNone
}

// CreateNewController creates a new controller with the specified
// controller type, bus number, and sharing mode.
// For SCSI controller type, the type is ParaVirtualSCSI by default.
func CreateNewController(
	controllerType vmopv1.VirtualControllerType,
	busNumber int32,
	sharingMode vmopv1.VirtualControllerSharingMode,
) ControllerSpec {
	switch controllerType {
	case vmopv1.VirtualControllerTypeSCSI:
		return vmopv1.SCSIControllerSpec{
			BusNumber:   busNumber,
			Type:        vmopv1.SCSIControllerTypeParaVirtualSCSI,
			SharingMode: sharingMode,
		}
	case vmopv1.VirtualControllerTypeSATA:
		return vmopv1.SATAControllerSpec{
			BusNumber: busNumber,
		}
	case vmopv1.VirtualControllerTypeNVME:
		return vmopv1.NVMEControllerSpec{
			BusNumber:   busNumber,
			SharingMode: sharingMode,
		}
	case vmopv1.VirtualControllerTypeIDE:
		return vmopv1.IDEControllerSpec{
			BusNumber: busNumber,
		}
	default:
		return nil
	}
}

// DefaultControllerSharingMode returns the default controller sharing mode for
// the specified volume.
func DefaultControllerSharingMode(
	vol vmopv1.VirtualMachineVolume,
) vmopv1.VirtualControllerSharingMode {

	if vol.ApplicationType == vmopv1.VolumeApplicationTypeMicrosoftWSFC {
		return MicrosoftWSFCControllerSharingMode
	}
	return vmopv1.VirtualControllerSharingModeNone
}

// ControllerSpecs is a collection of controller specifications.
type ControllerSpecs struct {
	// controllers is a map of controller type to a map of bus numbers to
	// controller specifications.
	controllers map[vmopv1.VirtualControllerType]map[int32]ControllerSpec
}

// NewControllerSpecs creates a new ControllerSpecs from the specified VM's
// spec.hardware.controllers.
func NewControllerSpecs(
	vm vmopv1.VirtualMachine,
) ControllerSpecs {

	controllerSpecs := ControllerSpecs{
		controllers: make(map[vmopv1.VirtualControllerType]map[int32]ControllerSpec),
	}

	if vm.Spec.Hardware == nil {
		return controllerSpecs
	}

	for _, controller := range vm.Spec.Hardware.SCSIControllers {
		controllerSpecs.Set(
			vmopv1.VirtualControllerTypeSCSI,
			controller.BusNumber,
			controller,
		)
	}

	for _, controller := range vm.Spec.Hardware.SATAControllers {
		controllerSpecs.Set(
			vmopv1.VirtualControllerTypeSATA,
			controller.BusNumber,
			controller,
		)
	}

	for _, controller := range vm.Spec.Hardware.NVMEControllers {
		controllerSpecs.Set(
			vmopv1.VirtualControllerTypeNVME,
			controller.BusNumber,
			controller,
		)
	}

	for _, controller := range vm.Spec.Hardware.IDEControllers {
		controllerSpecs.Set(
			vmopv1.VirtualControllerTypeIDE,
			controller.BusNumber,
			controller,
		)
	}

	return controllerSpecs
}

// Get returns the controller specification for the specified
// controller type and bus number. Returns (nil, false) if not found.
func (c ControllerSpecs) Get(
	controllerType vmopv1.VirtualControllerType,
	busNumber int32,
) (ControllerSpec, bool) {
	if controllers, exists := c.controllers[controllerType]; exists {
		if controller, exists := controllers[busNumber]; exists {
			return controller, true
		}
	}
	return nil, false
}

// Set sets the controller specification for the specified controller type and
// bus number.
func (c ControllerSpecs) Set(
	controllerType vmopv1.VirtualControllerType,
	busNumber int32,
	controller ControllerSpec,
) {
	if _, exists := c.controllers[controllerType]; !exists {
		c.controllers[controllerType] = make(map[int32]ControllerSpec)
	}
	c.controllers[controllerType][busNumber] = controller
}

// CountControllers returns the number of controllers for the specified
// controller type.
func (c ControllerSpecs) CountControllers(
	controllerType vmopv1.VirtualControllerType,
) int {
	if _, exists := c.controllers[controllerType]; !exists {
		return 0
	}
	return len(c.controllers[controllerType])
}
