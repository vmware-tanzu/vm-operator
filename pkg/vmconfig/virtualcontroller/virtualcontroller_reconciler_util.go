// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualcontroller

import (
	"context"
	"errors"
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

var (
	errInvalidSharingMode = errors.New("invalid sharing mode")
)

// newIDEController returns a new IDE controller.
// It assigns the current value of newDeviceKey to the controller, then
// decrements newDeviceKey to ensure the next assigned controller gets a
// unique temporary key.
func newIDEController(
	spec vmopv1.IDEControllerSpec,
	pciController *vimtypes.VirtualPCIController,
	newDeviceKey *int32,
) *vimtypes.VirtualIDEController {

	*newDeviceKey--

	return &vimtypes.VirtualIDEController{
		VirtualController: vimtypes.VirtualController{
			VirtualDevice: vimtypes.VirtualDevice{
				ControllerKey: pciController.Key,
				Key:           *newDeviceKey,
			},
			BusNumber: spec.BusNumber,
		},
	}
}

func validateNVMEController(spec vmopv1.NVMEControllerSpec) error {
	switch spec.SharingMode {
	case vmopv1.VirtualControllerSharingModeNone,
		vmopv1.VirtualControllerSharingModePhysical:
		return nil
	default:
		return fmt.Errorf("invalid sharingMode for NVME Controller: %s", spec.SharingMode)
	}
}

// newNVMEController returns a new NVME controller.
// It assigns the current value of newDeviceKey to the controller, then
// decrements newDeviceKey to ensure the next assigned controller gets a
// unique temporary key.
func newNVMEController(
	ctx context.Context,
	spec vmopv1.NVMEControllerSpec,
	pciController *vimtypes.VirtualPCIController,
	newDeviceKey *int32,
) *vimtypes.VirtualNVMEController {

	*newDeviceKey--

	controller := &vimtypes.VirtualNVMEController{
		VirtualController: vimtypes.VirtualController{
			VirtualDevice: vimtypes.VirtualDevice{
				ControllerKey: pciController.Key,
				Key:           *newDeviceKey,
			},
			BusNumber: spec.BusNumber,
		},
		SharedBus: string(sharingModeToNVMEControllerSharing(ctx, spec.SharingMode)),
	}

	return controller
}

// newSATAController returns a new SATA controller.
// It assigns the current value of newDeviceKey to the controller, then
// decrements newDeviceKey to ensure the next assigned controller gets a
// unique temporary key.
func newSATAController(
	spec vmopv1.SATAControllerSpec,
	pciController *vimtypes.VirtualPCIController,
	newDeviceKey *int32,
) *vimtypes.VirtualAHCIController {

	*newDeviceKey--

	controller := &vimtypes.VirtualAHCIController{
		VirtualSATAController: vimtypes.VirtualSATAController{
			VirtualController: vimtypes.VirtualController{
				VirtualDevice: vimtypes.VirtualDevice{
					ControllerKey: pciController.Key,
					Key:           *newDeviceKey,
				},
				BusNumber: spec.BusNumber,
			},
		},
	}

	return controller
}

// newSCSIController returns a new SCSI controller.
// It assigns the current value of newDeviceKey to the controller, then
// decrements newDeviceKey to ensure the next assigned controller gets a
// unique temporary key.
func newSCSIController(
	ctx context.Context,
	spec vmopv1.SCSIControllerSpec,
	pciController *vimtypes.VirtualPCIController,
	newDeviceKey *int32,
) vimtypes.BaseVirtualSCSIController {

	*newDeviceKey--

	var controller vimtypes.BaseVirtualSCSIController
	switch spec.Type {
	case vmopv1.SCSIControllerTypeParaVirtualSCSI:
		controller = &vimtypes.ParaVirtualSCSIController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: pciController.Key,
						Key:           *newDeviceKey,
					},
					BusNumber: spec.BusNumber,
				},
				SharedBus: sharingModeToSCSIVimTypes(ctx, spec.SharingMode),
			},
		}
	case vmopv1.SCSIControllerTypeBusLogic:
		controller = &vimtypes.VirtualBusLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: pciController.Key,
						Key:           *newDeviceKey,
					},
					BusNumber: spec.BusNumber,
				},
				SharedBus: sharingModeToSCSIVimTypes(ctx, spec.SharingMode),
			},
		}
	case vmopv1.SCSIControllerTypeLsiLogic:
		controller = &vimtypes.VirtualLsiLogicController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: pciController.Key,
						Key:           *newDeviceKey,
					},
					BusNumber: spec.BusNumber,
				},
				SharedBus: sharingModeToSCSIVimTypes(ctx, spec.SharingMode),
			},
		}
	case vmopv1.SCSIControllerTypeLsiLogicSAS:
		controller = &vimtypes.VirtualLsiLogicSASController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: pciController.Key,
						Key:           *newDeviceKey,
					},
					BusNumber: spec.BusNumber,
				},
				SharedBus: sharingModeToSCSIVimTypes(ctx, spec.SharingMode),
			},
		}
	}

	return controller
}

// sharingModeToSCSIVimTypes converts the vmopv1 sharing mode to the
// vimtypes VirtualSCSISharing enum.
func sharingModeToSCSIVimTypes(
	ctx context.Context,
	sharingMode vmopv1.VirtualControllerSharingMode,
) vimtypes.VirtualSCSISharing {

	switch sharingMode {
	case vmopv1.VirtualControllerSharingModeNone:
		return vimtypes.VirtualSCSISharingNoSharing
	case vmopv1.VirtualControllerSharingModePhysical:
		return vimtypes.VirtualSCSISharingPhysicalSharing
	case vmopv1.VirtualControllerSharingModeVirtual:
		return vimtypes.VirtualSCSISharingVirtualSharing
	default:
		pkglog.FromContextOrDefault(ctx).
			Error(errInvalidSharingMode, "defaulting to noSharing", "sharingMode", sharingMode)
		return vimtypes.VirtualSCSISharingNoSharing
	}
}

// sharingModeToNVMEControllerSharing converts the vmopv1 sharing mode to the
// vimtypes VirtualNVMEControllerSharing enum.
func sharingModeToNVMEControllerSharing(
	ctx context.Context,
	sharingMode vmopv1.VirtualControllerSharingMode,
) vimtypes.VirtualNVMEControllerSharing {

	switch sharingMode {
	case vmopv1.VirtualControllerSharingModeNone:
		return vimtypes.VirtualNVMEControllerSharingNoSharing
	case vmopv1.VirtualControllerSharingModePhysical:
		return vimtypes.VirtualNVMEControllerSharingPhysicalSharing
	default:
		pkglog.FromContextOrDefault(ctx).
			Error(errInvalidSharingMode, "defaulting to noSharing", "sharingMode", sharingMode)
		return vimtypes.VirtualNVMEControllerSharingNoSharing
	}
}

// addDeviceChange adds a DeviceChange operation to configSpec.DeviceChange.
func addDeviceChangeOp(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	controller vimtypes.BaseVirtualDevice,
	op vimtypes.VirtualDeviceConfigSpecOperation,
) {
	configSpec.DeviceChange = append(configSpec.DeviceChange,
		&vimtypes.VirtualDeviceConfigSpec{
			Operation: op,
			Device:    controller,
		})
}

// diffByBusNumber compares two maps keyed by bus number.
// It returns the devices to be added, edited, and removed.
//   - toAdd:    present in desired, absent in existing
//   - toEdit:   present in both but not matching (match(dev, spec) == false)
//     (edit(dev, spec) is invoked before returning it in toEdit). A special
//     case is when edit(dev, spec) returns true, which means the device needs
//     to be recreated instead of edited in place.
//   - toRemove: present in existing, absent in desired
func diffByBusNumber[Dev any, Spec any](
	ctx context.Context,
	existing map[int32]Dev,
	desired map[int32]Spec,
	match func(context.Context, Dev, Spec) bool,
	edit func(context.Context, Dev, Spec) bool,
) (toAdd []Spec, toEdit []Dev, toRemove []Dev) {

	// Add
	for bus, spec := range desired {
		if _, ok := existing[bus]; !ok {
			toAdd = append(toAdd, spec)
		}
	}
	// Remove / Edit
	for bus, dev := range existing {
		spec, ok := desired[bus]
		if !ok {
			toRemove = append(toRemove, dev)
			continue
		}

		// If the device does not match the spec, edit it if possible.
		if match != nil && !match(ctx, dev, spec) {
			// If the edit function is not nil, edit the device if possible.
			if edit != nil {
				needRecreate := edit(ctx, dev, spec)
				// If the device needs to be recreated, remove the existing
				// device and add the new one.
				if needRecreate {
					toRemove = append(toRemove, dev)
					toAdd = append(toAdd, spec)
					continue
				}

				toEdit = append(toEdit, dev)
			}
			// If the edit function is nil, do nothing.
		}
	}

	return toAdd, toEdit, toRemove
}

// diffIDEControllerByBusNumber compares two maps keyed by bus number by
// calling diffByBusNumber().
func diffIDEControllerByBusNumber(
	ctx context.Context,
	existing map[int32]*vimtypes.VirtualIDEController,
	desired map[int32]vmopv1.IDEControllerSpec,
) (
	toAdd []vmopv1.IDEControllerSpec,
	toEdit []*vimtypes.VirtualIDEController,
	toRemove []*vimtypes.VirtualIDEController,
) {

	// No match and edit function, since we only expose bus number,
	// which is immutable.
	return diffByBusNumber(ctx, existing, desired, nil, nil)
}

// diffNVMEControllerByBusNumber compares two maps keyed by bus number by
// calling diffByBusNumber().
func diffNVMEControllerByBusNumber(
	ctx context.Context,
	existing map[int32]*vimtypes.VirtualNVMEController,
	desired map[int32]vmopv1.NVMEControllerSpec,
) (
	toAdd []vmopv1.NVMEControllerSpec,
	toEdit []*vimtypes.VirtualNVMEController,
	toRemove []*vimtypes.VirtualNVMEController,
) {

	return diffByBusNumber(ctx, existing, desired, nvmeDeviceMatchSpec, editNVMEController)
}

// diffSATAControllerByBusNumber compares two maps keyed by bus number by
// calling diffByBusNumber().
func diffSATAControllerByBusNumber(
	ctx context.Context,
	existing map[int32]*vimtypes.VirtualAHCIController,
	desired map[int32]vmopv1.SATAControllerSpec,
) (toAdd []vmopv1.SATAControllerSpec,
	toEdit []*vimtypes.VirtualAHCIController,
	toRemove []*vimtypes.VirtualAHCIController,
) {

	// No match and edit function, since we only expose bus number,
	// which is immutable.
	return diffByBusNumber(ctx, existing, desired, nil, nil)
}

// diffSCSIControllerByBusNumber compares two maps keyed by bus number by
// calling diffByBusNumber().
func diffSCSIControllerByBusNumber(
	ctx context.Context,
	existing map[int32]vimtypes.BaseVirtualSCSIController,
	desired map[int32]vmopv1.SCSIControllerSpec,
) (toAdd []vmopv1.SCSIControllerSpec,
	toEdit []vimtypes.BaseVirtualSCSIController,
	toRemove []vimtypes.BaseVirtualSCSIController,
) {

	return diffByBusNumber(ctx, existing, desired, scsiDeviceMatchSpec, editSCSIController)
}

// editNVMEController edits the device to match the spec.
// Modify the PCI slot number and ShareingMode if needed.
// Return bool to indicate that the device needs to be recreated.
func editNVMEController(
	ctx context.Context,
	dev *vimtypes.VirtualNVMEController,
	spec vmopv1.NVMEControllerSpec,
) (needRecreate bool) {

	if dev.SharedBus != string(sharingModeToNVMEControllerSharing(ctx, spec.SharingMode)) {
		dev.SharedBus = string(sharingModeToNVMEControllerSharing(ctx, spec.SharingMode))
	}

	return false
}

// nvmeDeviceMatchSpec checks if the device matches the spec.
// Only match when:
// - PCI slot number matches.
// - Sharing mode matches.
func nvmeDeviceMatchSpec(
	ctx context.Context,
	dev *vimtypes.VirtualNVMEController,
	spec vmopv1.NVMEControllerSpec,
) bool {

	return string(sharingModeToNVMEControllerSharing(ctx, spec.SharingMode)) == dev.SharedBus
}

// scsiDeviceMatchSpec checks if the device matches the spec.
// Only match when:
// - PCI slot number matches.
// - Sharing mode matches.
// - Type matches.
func scsiDeviceMatchSpec(
	ctx context.Context,
	dev vimtypes.BaseVirtualSCSIController,
	spec vmopv1.SCSIControllerSpec) bool {

	scsi := dev.GetVirtualSCSIController()

	return sharingModeToSCSIVimTypes(ctx, spec.SharingMode) == scsi.SharedBus &&
		SCSIControllerTypeMatch(dev, spec.Type)
}

// SCSIControllerTypeMatch checks if the controller type matches the spec.
func SCSIControllerTypeMatch(dev vimtypes.BaseVirtualSCSIController, specType vmopv1.SCSIControllerType) bool {
	switch dev.(type) {
	case *vimtypes.ParaVirtualSCSIController:
		return specType == vmopv1.SCSIControllerTypeParaVirtualSCSI
	case *vimtypes.VirtualBusLogicController:
		return specType == vmopv1.SCSIControllerTypeBusLogic
	case *vimtypes.VirtualLsiLogicController:
		return specType == vmopv1.SCSIControllerTypeLsiLogic
	case *vimtypes.VirtualLsiLogicSASController:
		return specType == vmopv1.SCSIControllerTypeLsiLogicSAS
	default:
		return false
	}
}

// editSCSIController edits the device to match the spec.
// Modify the PCI slot number and ShareingMode if needed.
// If the controller type does not match the spec, return true to indicate
// that the device needs to be recreated.
func editSCSIController(
	ctx context.Context,
	dev vimtypes.BaseVirtualSCSIController,
	spec vmopv1.SCSIControllerSpec,
) (needRecreate bool) {

	if !SCSIControllerTypeMatch(dev, spec.Type) {
		// A recreate is needed, return true and let the caller remove
		// the existing device and add the new one.
		return true
	}

	if dev.GetVirtualSCSIController().SharedBus != sharingModeToSCSIVimTypes(ctx, spec.SharingMode) {
		dev.GetVirtualSCSIController().SharedBus = sharingModeToSCSIVimTypes(ctx, spec.SharingMode)
	}

	return false
}
