// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"maps"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"golang.org/x/tools/container/intsets"

	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// EnsureDisksHaveControllers ensures that all disks in the provided
// ConfigSpec point to a controller. If no controller exists, new (in
// order of preference) PVSCSI, AHCI, or NVME controllers are added
// to the ConfigSpec as necessary for the disks.
//
// Please note the following table for the number of controllers of each type
// that are supported as well as how many disks (per controller) each supports:
//
// SATA
//   - controllers                                    4
//   - disks                                         30
//
// SCSI
//   - controllers                                    4
//   - disks (non-paravirtual)                       16
//   - disks (paravirtual, hardware version <14)     16
//   - disks (paravirtual, hardware version >=14)   256
//
// NVME
//   - controllers                                    4
//   - disks (hardware version <20)                  15
//   - disks (hardware version >=21)                255
func EnsureDisksHaveControllers(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	existingDevices ...vimtypes.BaseVirtualDevice) error {

	if configSpec == nil {
		return fmt.Errorf("configSpec is nil")
	}

	var (
		disks           []*vimtypes.VirtualDisk
		newDeviceKey    int32
		pciController   *vimtypes.VirtualPCIController
		diskControllers = ensureDiskControllerData{
			keyToDiskControllers: map[int32]*diskController{},
		}
	)

	// Inspect the ConfigSpec
	for i := range configSpec.DeviceChange {
		var (
			bdc vimtypes.BaseVirtualDeviceConfigSpec
			bvd vimtypes.BaseVirtualDevice
			dc  *vimtypes.VirtualDeviceConfigSpec
			d   *vimtypes.VirtualDevice
		)

		if bdc = configSpec.DeviceChange[i]; bdc == nil {
			continue
		}

		if dc = bdc.GetVirtualDeviceConfigSpec(); dc == nil {
			continue
		}

		if dc.Operation == vimtypes.VirtualDeviceConfigSpecOperationRemove {
			// Do not consider devices being removed.
			continue
		}

		bvd = dc.Device
		if bvd == nil {
			continue
		}

		if d = bvd.GetVirtualDevice(); d == nil {
			continue
		}

		switch tvd := bvd.(type) {
		case *vimtypes.VirtualPCIController:
			pciController = tvd

		case
			// SCSI
			*vimtypes.ParaVirtualSCSIController,
			*vimtypes.VirtualBusLogicController,
			*vimtypes.VirtualLsiLogicController,
			*vimtypes.VirtualLsiLogicSASController,
			*vimtypes.VirtualSCSIController,

			// SATA
			*vimtypes.VirtualSATAController,
			*vimtypes.VirtualAHCIController,

			// NVME
			*vimtypes.VirtualNVMEController:

			diskControllers.add(bvd)

		case *vimtypes.VirtualDisk:

			disks = append(disks, tvd)

			if controllerKey := d.ControllerKey; controllerKey != 0 {
				// If the disk points to a controller key, then add a placeholder
				// entry for the controller. Please note that at this point it is
				// not yet known if the controller key is a *valid* controller.
				// validateAttachments() will remove placeholder entries that did
				// not have a corresponding controller device.
				diskControllers.addDisk(controllerKey, tvd.UnitNumber)
			}
		}

		// Keep track of the smallest device key used. Please note, because
		// device keys in a ConfigSpec are negative numbers, -200 going to be
		// smaller than -1.
		if d.Key < newDeviceKey {
			newDeviceKey = d.Key
		}
	}

	if len(disks) == 0 {
		// If there are no disks, then go ahead and return early.
		return nil
	}

	// Categorize any controllers that already exist.
	for i := range existingDevices {
		var (
			d   *vimtypes.VirtualDevice
			bvd = existingDevices[i]
		)

		if bvd == nil {
			continue
		}

		if d = bvd.GetVirtualDevice(); d == nil {
			continue
		}

		switch tvd := bvd.(type) {
		case *vimtypes.VirtualPCIController:
			pciController = tvd
		case
			// SCSI
			*vimtypes.ParaVirtualSCSIController,
			*vimtypes.VirtualBusLogicController,
			*vimtypes.VirtualLsiLogicController,
			*vimtypes.VirtualLsiLogicSASController,
			*vimtypes.VirtualSCSIController,

			// SATA
			*vimtypes.VirtualSATAController,
			*vimtypes.VirtualAHCIController,

			// NVME
			*vimtypes.VirtualNVMEController:

			diskControllers.add(bvd)

		case *vimtypes.VirtualDisk:
			diskControllers.addDisk(tvd.ControllerKey, tvd.UnitNumber)
		}
	}

	// Decrement the newDeviceKey so the next device has a unique key.
	newDeviceKey--

	if pciController == nil {
		// Add a PCI controller if one is not present.
		pciController = &vimtypes.VirtualPCIController{
			VirtualController: vimtypes.VirtualController{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: newDeviceKey,
				},
			},
		}

		// Decrement the newDeviceKey so the next device has a unique key.
		newDeviceKey--

		// Add the new PCI controller to the ConfigSpec.
		configSpec.DeviceChange = append(
			configSpec.DeviceChange,
			&vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    pciController,
			})
	}

	// Ensure all the recorded controller keys that disks pointed to are valid
	// controller keys.
	diskControllers.validateAttachments()

	// For any disks that specified a valid controller but not a unit number,
	// try to allocate a unit number now. These disks need to get first dibs
	// on available unit numbers.
	var disksWithoutController []*vimtypes.VirtualDisk
	for _, disk := range disks {
		if !diskControllers.exists(disk.ControllerKey) {
			disksWithoutController = append(disksWithoutController, disk)
			continue
		}

		if disk.UnitNumber == nil {
			unitNumber, ok := diskControllers.getNextUnitNumber(disk.ControllerKey)
			if !ok {
				return fmt.Errorf("no available unit number for controller key %d", disk.ControllerKey)
			}
			disk.UnitNumber = &unitNumber
		}
	}

	for _, disk := range disksWithoutController {
		// The disk does not point to a controller, so try to locate one.
		if ensureDiskControllerFind(disk, &diskControllers) {
			// A controller was located for the disk, so go ahead and skip to
			// the next disk.
			continue
		}

		// No controller was located for the disk, so a controller must be
		// created.
		if err := ensureDiskControllerCreate(
			configSpec,
			pciController,
			newDeviceKey,
			&diskControllers); err != nil {

			return err
		}

		// Point the disk to the new controller.
		disk.ControllerKey = newDeviceKey
		unitNumber := diskControllers.mustGetNextUnitNumber(disk.ControllerKey)
		disk.UnitNumber = &unitNumber

		// Decrement the newDeviceKey so the next device has a unique key.
		newDeviceKey--
	}

	return nil
}

const (
	maxSCSIControllers                     = 4
	maxSATAControllers                     = 4
	maxNVMEControllers                     = 4
	maxDisksPerSCSIController              = 16
	maxDisksPerPVSCSIControllerHWVersion14 = 256 // TODO(akutz)
	maxDisksPerSATAController              = 30
	maxDisksPerNVMEController              = 15
	maxDisksPerNVMEControllerHWVersion21   = 255 // TODO(akutz)
)

type ensureDiskControllerBusNumbers struct {
	zero bool
	one  bool
	two  bool
}

func (d ensureDiskControllerBusNumbers) free() int32 {
	switch {
	case !d.zero:
		return 0
	case !d.one:
		return 1
	case !d.two:
		return 2
	default:
		return 3
	}
}

func (d *ensureDiskControllerBusNumbers) set(busNumber int32) {
	switch busNumber {
	case 0:
		d.zero = true
	case 1:
		d.one = true
	case 2:
		d.two = true
	}
}

type diskController struct {
	dev vimtypes.BaseVirtualController

	// Disk unit number allocation:
	curUN, maxUN int32
	reservedUN   intsets.Sparse
}

func (dc *diskController) reserveUnitNumber(unitNumber int32) {
	// TODO: check !dc.reservedUN.Has()?
	dc.reservedUN.Insert(int(unitNumber))
}

func (dc *diskController) nextUnitNumber() (int32, bool) {
	for un := dc.curUN; un < dc.maxUN; un++ {
		if dc.reservedUN.IsEmpty() || !dc.reservedUN.Has(int(un)) {
			dc.curUN = un + 1
			return un, true
		}
	}

	return -1, false
}

type ensureDiskControllerData struct {
	// TODO(akutz) Use the hardware version when calculating the max disks for
	//             a given controller type.
	// hardwareVersion int

	keyToDiskControllers map[int32]*diskController

	// SCSI
	scsiBusNumbers             ensureDiskControllerBusNumbers
	pvSCSIControllerKeys       []int32
	busLogicSCSIControllerKeys []int32
	lsiLogicControllerKeys     []int32
	lsiLogicSASControllerKeys  []int32
	scsiControllerKeys         []int32

	// SATA
	sataBusNumbers     ensureDiskControllerBusNumbers
	sataControllerKeys []int32
	ahciControllerKeys []int32

	// NVME
	nvmeBusNumbers     ensureDiskControllerBusNumbers
	nvmeControllerKeys []int32
}

func (d ensureDiskControllerData) numSCSIControllers() int {
	return len(d.pvSCSIControllerKeys) +
		len(d.busLogicSCSIControllerKeys) +
		len(d.lsiLogicControllerKeys) +
		len(d.lsiLogicSASControllerKeys) +
		len(d.scsiControllerKeys)
}

func (d ensureDiskControllerData) numSATAControllers() int {
	return len(d.sataControllerKeys) + len(d.ahciControllerKeys)
}

func (d ensureDiskControllerData) numNVMEControllers() int {
	return len(d.nvmeControllerKeys)
}

// validateAttachments removes any controllers that do not have a device
// associated with it.
func (d ensureDiskControllerData) validateAttachments() {
	maps.DeleteFunc(d.keyToDiskControllers, func(_ int32, dc *diskController) bool {
		return dc.dev == nil
	})
}

// exists returns true if a controller with the provided key exists.
func (d ensureDiskControllerData) exists(key int32) bool {
	_, ok := d.keyToDiskControllers[key]
	return ok
}

// add records the provided controller in the map that relates keys to
// controllers as well as appends the key to the list of controllers of that
// given type.
// TODO(akutz) Consider the hardware version for the max number of disks.
func (d *ensureDiskControllerData) add(controller vimtypes.BaseVirtualDevice) {

	// Get the controller's device key.
	bvc := controller.(vimtypes.BaseVirtualController)
	key := bvc.GetVirtualController().Key
	busNumber := bvc.GetVirtualController().BusNumber

	dc, ok := d.keyToDiskControllers[key]
	if !ok {
		dc = &diskController{}
		d.keyToDiskControllers[key] = dc
	}

	// TODO: Check dc.dev == nil?
	dc.dev = bvc

	// Record the controller's device key in the list for that type of
	// controller.
	switch controller.(type) {

	// SCSI
	// TODO: Shouldn't we reserve the scsiCtlrUnitNumber?
	case *vimtypes.ParaVirtualSCSIController:
		d.pvSCSIControllerKeys = append(d.pvSCSIControllerKeys, key)
		d.scsiBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSCSIController // TODO: maxDisksPerPVSCSIControllerHWVersion14
	case *vimtypes.VirtualBusLogicController:
		d.busLogicSCSIControllerKeys = append(d.busLogicSCSIControllerKeys, key)
		d.scsiBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSCSIController
	case *vimtypes.VirtualLsiLogicController:
		d.lsiLogicControllerKeys = append(d.lsiLogicControllerKeys, key)
		d.scsiBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSCSIController
	case *vimtypes.VirtualLsiLogicSASController:
		d.lsiLogicSASControllerKeys = append(d.lsiLogicSASControllerKeys, key)
		d.scsiBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSCSIController
	case *vimtypes.VirtualSCSIController:
		d.scsiControllerKeys = append(d.scsiControllerKeys, key)
		d.scsiBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSCSIController

	// SATA
	case *vimtypes.VirtualSATAController:
		d.sataControllerKeys = append(d.sataControllerKeys, key)
		d.sataBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSATAController
	case *vimtypes.VirtualAHCIController:
		d.ahciControllerKeys = append(d.ahciControllerKeys, key)
		d.sataBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerSATAController

	// NVME
	case *vimtypes.VirtualNVMEController:
		d.nvmeControllerKeys = append(d.nvmeControllerKeys, key)
		d.nvmeBusNumbers.set(busNumber)
		dc.maxUN = maxDisksPerNVMEController // TODO: maxDisksPerNVMEControllerHWVersion21

	default:
		panic(fmt.Sprintf("unexpected controller type: %T", controller))
	}
}

// addDisk adds a placeholder for this disks controller if add() wasn't already
// called for that controller, and reserves the unit number if specified.
func (d *ensureDiskControllerData) addDisk(controllerKey int32, unitNumber *int32) {
	dc, ok := d.keyToDiskControllers[controllerKey]
	if !ok {
		dc = &diskController{}
		d.keyToDiskControllers[controllerKey] = dc
	}

	if unitNumber != nil {
		dc.reserveUnitNumber(*unitNumber)
	}
}

// getNextUnitNumber returns the next available unit number for the controller
// to assign to a disk.
func (d *ensureDiskControllerData) getNextUnitNumber(controllerKey int32) (int32, bool) {
	dc, ok := d.keyToDiskControllers[controllerKey]
	if !ok {
		// We shouldn't have an invalid controller key at this point.
		return -1, false
	}

	return dc.nextUnitNumber()
}

func (d *ensureDiskControllerData) mustGetNextUnitNumber(controllerKey int32) int32 {
	unitNumber, ok := d.getNextUnitNumber(controllerKey)
	if !ok {
		panic(fmt.Sprintf("must get unit number for controller key %d failed", controllerKey))
	}
	return unitNumber
}

// ensureDiskControllerFind attempts to locate a controller for the provided
// disk.
//
// Please note this function is written to preserve the order in which
// controllers are located by preferring controller types in the order in which
// they are listed in this function. This prevents the following situation:
//
//   - A ConfigSpec has three controllers in the following order: PVSCSI-1,
//     NVME-1, and PVSCSI-2.
//   - The controller PVSCSI-1 is full while NVME-1 and PVSCSI-2 have free
//     slots.
//   - The *desired* behavior is to look at all, possible PVSCSI controllers
//     before moving onto SATA and then finally NVME controllers.
//   - If the function iterated over the device list in list-order, then the
//     NVME-1 controller would be located first.
//   - Instead, this function iterates over each *type* of controller first
//     before moving onto the next type.
//   - This means that even though NVME-1 has free slots, PVSCSI-2 is checked
//     first.
//
// The order of preference is as follows:
//
// * SCSI
//   - ParaVirtualSCSIController
//   - VirtualBusLogicController
//   - VirtualLsiLogicController
//   - VirtualLsiLogicSASController
//   - VirtualSCSIController
//
// * SATA
//   - VirtualSATAController
//   - VirtualAHCIController
//
// * NVME
//   - VirtualNVMEController
func ensureDiskControllerFind(
	disk *vimtypes.VirtualDisk,
	diskControllers *ensureDiskControllerData) bool {

	return false ||
		// SCSI
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.pvSCSIControllerKeys) ||
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.busLogicSCSIControllerKeys) ||
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.lsiLogicControllerKeys) ||
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.lsiLogicSASControllerKeys) ||
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.scsiControllerKeys) ||

		// SATA
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.sataControllerKeys) ||
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.ahciControllerKeys) ||

		// NVME
		ensureDiskControllerFindWith(
			disk,
			diskControllers,
			diskControllers.nvmeControllerKeys)
}

func ensureDiskControllerFindWith(
	disk *vimtypes.VirtualDisk,
	diskControllers *ensureDiskControllerData,
	controllerKeys []int32) bool {

	for i := range controllerKeys {
		controllerKey := controllerKeys[i]
		// If the controller has a unit number available then use the controller for
		// this disk.
		// TODO: Remove full controllers from their corresponding ensureDiskControllerData keys list.
		if unitNumber, ok := diskControllers.getNextUnitNumber(controllerKey); ok {
			disk.ControllerKey = controllerKey
			disk.UnitNumber = &unitNumber
			return true
		}
	}
	return false
}

func ensureDiskControllerCreate(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	pciController *vimtypes.VirtualPCIController,
	newDeviceKey int32,
	diskControllers *ensureDiskControllerData) error {

	var controller vimtypes.BaseVirtualDevice
	switch {
	case diskControllers.numSCSIControllers() < maxSCSIControllers:
		// Prefer creating a new SCSI controller.
		controller = &vimtypes.ParaVirtualSCSIController{
			VirtualSCSIController: vimtypes.VirtualSCSIController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: pciController.Key,
						Key:           newDeviceKey,
					},
					BusNumber: diskControllers.scsiBusNumbers.free(),
				},
				HotAddRemove: ptr.To(true),
				SharedBus:    vimtypes.VirtualSCSISharingNoSharing,
			},
		}
	case diskControllers.numSATAControllers() < maxSATAControllers:
		// If there are no more SCSI controllers, create a SATA
		// controller.
		controller = &vimtypes.VirtualAHCIController{
			VirtualSATAController: vimtypes.VirtualSATAController{
				VirtualController: vimtypes.VirtualController{
					VirtualDevice: vimtypes.VirtualDevice{
						ControllerKey: pciController.Key,
						Key:           newDeviceKey,
					},
					BusNumber: diskControllers.sataBusNumbers.free(),
				},
			},
		}
	case diskControllers.numNVMEControllers() < maxNVMEControllers:
		// If there are no more SATA controllers, create an NVME
		// controller.
		controller = &vimtypes.VirtualNVMEController{
			VirtualController: vimtypes.VirtualController{
				VirtualDevice: vimtypes.VirtualDevice{
					ControllerKey: pciController.Key,
					Key:           newDeviceKey,
				},
				BusNumber: diskControllers.nvmeBusNumbers.free(),
			},
			SharedBus: string(vimtypes.VirtualNVMEControllerSharingNoSharing),
		}
	default:
		return fmt.Errorf("no controllers available")
	}

	// TODO: Need to set controller.GetVirtualDevice().UnitNumber?

	// Add the new controller to the ConfigSpec.
	configSpec.DeviceChange = append(
		configSpec.DeviceChange,
		&vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    controller,
		})

	// Record the new controller.
	diskControllers.add(controller)

	return nil
}
