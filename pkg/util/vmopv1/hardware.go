// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
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

	for unitNumber := int32(0); unitNumber < controller.MaxSlots(); unitNumber++ {
		if _, exists := occupiedSlots[unitNumber]; !exists &&
			unitNumber != controller.ReservedUnitNumber() {
			return unitNumber
		}
	}
	return -1
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

// GetManagedVolumesWithPVC returns all volumes from the VM spec that have
// a PersistentVolumeClaim and are managed (not instance storage).
func GetManagedVolumesWithPVC(
	vm vmopv1.VirtualMachine,
) []vmopv1.VirtualMachineVolume {

	var volumes []vmopv1.VirtualMachineVolume
	for _, v := range vm.Spec.Volumes {
		if v.PersistentVolumeClaim != nil &&
			v.PersistentVolumeClaim.UnmanagedVolumeClaim == nil {
			volumes = append(volumes, v)
		}
	}

	return volumes
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
