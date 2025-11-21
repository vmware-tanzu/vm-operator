// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Code generated from permutation test results. DO NOT EDIT.

package vmopv1

import (
	"errors"
	"fmt"
	"strings"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

type DiskProvisioningType string

const (
	DiskProvisioningTypeThin           DiskProvisioningType = "Thin"
	DiskProvisioningTypeThick          DiskProvisioningType = "Thick"
	DiskProvisioningTypeThickEagerZero DiskProvisioningType = "ThickEagerZero"
)

// IsControllerDiskSupported validates whether a given combination of controller
// type, controller sharing mode, disk mode, disk sharing mode, and guest ID is
// supported by vSphere based on empirical testing.
//
// The function recognizes the following guest IDs and validates controller 
// compatibility:
//
//   - otherLinuxGuest (32-bit)
//   - otherLinux64Guest (64-bit)
//   - windows7Guest (32-bit)
//   - windows7_64Guest (64-bit)
//   - windows8Guest and higher (32-bit)
//   - windows8_64Guest and higher (64-bit)
//
// Guest OS specific restrictions:
//
//   - BusLogic SCSI is not supported for 64-bit guests or Windows 7+
//    (any bitness)
//   - LSI Logic SCSI is not supported for Windows 8+ guests (any bitness)
//
// If the provided guest ID is not recognized, it is treated as
// otherLinux64Guest.
//
// Returns nil if the combination is supported, or an error if unsupported.
func IsControllerDiskSupported(
	controllerType vmopv1.VirtualControllerType,
	scsiControllerType vmopv1.SCSIControllerType,
	controllerSharing vmopv1.VirtualControllerSharingMode,
	diskMode vmopv1.VolumeDiskMode,
	diskSharing vmopv1.VolumeSharingMode,
	diskProvisioningType DiskProvisioningType,
	guestID string,
) error {

	// Validate and normalize guest ID.
	guestID = getGuestID(guestID)

	// Map vmopv1 types to CSV format
	controllerTypeStr := mapControllerType(controllerType)
	controllerSubTypeStr := mapControllerSubType(controllerType, scsiControllerType)
	controllerSharingStr := mapControllerSharing(controllerSharing)
	diskModeStr := mapDiskMode(diskMode)
	diskSharingStr := mapDiskSharing(diskSharing)
	provisioningStr := mapProvisioningType(diskProvisioningType)

	// Create lookup key.
	key := fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s",
		controllerTypeStr,
		controllerSubTypeStr,
		controllerSharingStr,
		diskModeStr,
		diskSharingStr,
		provisioningStr,
		guestID)

	// Check the lookup table.
	if err, ok := unsupportedCombinations[key]; ok {
		return err
	}

	return nil
}

func getGuestID(guestID string) string {

	guestID = strings.ToLower(guestID)
	switch guestID {
	case "otherlinuxguest":
	
		guestID = "otherguest"
	
	case "otherlinux64guest":
	
		guestID = "otherlinux64"
	
	case "windows7guest":
	
		guestID = "windows7"
	
	case "windows7_64guest", "windows7-64", "windows764":
	
		guestID = "windows7_64"
	
	case "windows8guest":
	
		guestID = "windows8"
	
	case "windows8_64guest", "windows8-64", "windows864":
	
		guestID = "windows8_64"
	
	case "windows8serverguest", 
		"windows8server64guest", 
		"windows2012guest",
		"windows2012_64guest":
	
		guestID = "windows8_64"

	case "windows9guest", "windows9_64guest":

		guestID = "windows8_64"
	
	case "windows10guest", "windows10_64guest":

		guestID = "windows8_64"
	
	case "windows11guest", "windows11_64guest":

		guestID = "windows8_64"
	
		case "windows2016guest", 
		"windows2016_64guest", 
		"windows2019guest", 
		"windows2019_64guest",
		"windows2022guest", 
		"windows2022_64guest":

		guestID = "windows8_64"

	default:
		
		// Check if it starts with "windows" and contains "7", "8", "9", "10", 
		// "11", "2012", "2016", "2019", "2022"
		if strings.Contains(guestID, "windows") {
			
			// If it contains 7, treat as Windows 7
			if strings.Contains(guestID, "7") {
				if strings.Contains(guestID, "64") || 
					strings.Contains(guestID, "_64") {
						
					guestID = "windows7_64"
				} else {
					guestID = "windows7"
				}
			} else if strings.Contains(guestID, "8") || 
				strings.Contains(guestID, "9") ||
				strings.Contains(guestID, "10") || 
				strings.Contains(guestID, "11") ||
				strings.Contains(guestID, "2012") || 
				strings.Contains(guestID, "2016") ||
				strings.Contains(guestID, "2019") || 
				strings.Contains(guestID, "2022") {
				
					// Windows 8+ (all map to windows8_64 for validation purposes)
				guestID = "windows8_64"
			} else {
				
				// Unknown Windows version, default to otherlinux64
				guestID = "otherlinux64"
			}
		} else {
			
			// Default unknown guest IDs to otherlinux64
			guestID = "otherlinux64"
		}
	}

	return guestID
}

func mapControllerType(ct vmopv1.VirtualControllerType) string {
	switch ct {
	case vmopv1.VirtualControllerTypeSCSI:
		return "scsi"
	case vmopv1.VirtualControllerTypeIDE:
		return "ide"
	case vmopv1.VirtualControllerTypeSATA:
		return "sata"
	case vmopv1.VirtualControllerTypeNVME:
		return "nvme"
	default:
		return string(ct)
	}
}

func mapControllerSubType(
	ct vmopv1.VirtualControllerType, 
	scsiType vmopv1.SCSIControllerType) string {

	switch ct {
	case vmopv1.VirtualControllerTypeSCSI:
		switch scsiType {
		case vmopv1.SCSIControllerTypeParaVirtualSCSI:
			return "pvscsi"
		case vmopv1.SCSIControllerTypeBusLogic:
			return "buslogic"
		case vmopv1.SCSIControllerTypeLsiLogic:
			return "lsilogic"
		case vmopv1.SCSIControllerTypeLsiLogicSAS:
			return "lsilogic-sas"
		default:
			return string(scsiType)
		}
	case vmopv1.VirtualControllerTypeIDE:
		return "ide"
	case vmopv1.VirtualControllerTypeSATA:
		return "ahci"
	case vmopv1.VirtualControllerTypeNVME:
		return "nvme"
	default:
		return mapControllerType(ct)
	}
}

func mapControllerSharing(cs vmopv1.VirtualControllerSharingMode) string {
	switch cs {
	case vmopv1.VirtualControllerSharingModeNone, "":
		return "none"
	case vmopv1.VirtualControllerSharingModeVirtual:
		return "virtual"
	case vmopv1.VirtualControllerSharingModePhysical:
		return "physical"
	default:
		return "none"
	}
}

func mapDiskMode(dm vmopv1.VolumeDiskMode) string {
	switch dm {
	case vmopv1.VolumeDiskModePersistent:
		return "persistent"
	case vmopv1.VolumeDiskModeIndependentPersistent:
		return "independent_persistent"
	case vmopv1.VolumeDiskModeIndependentNonPersistent:
		return "independent_nonpersistent"
	case vmopv1.VolumeDiskModeNonPersistent:
		return "nonpersistent"
	case vmopv1.VolumeDiskMode("Append"):
		return "append"
	case vmopv1.VolumeDiskMode("Undoable"):
		return "undoable"
	default:
		return string(dm)
	}
}

func mapDiskSharing(ds vmopv1.VolumeSharingMode) string {
	switch ds {
	case vmopv1.VolumeSharingModeNone, "":
		return "none"
	case vmopv1.VolumeSharingModeMultiWriter:
		return "multi-writer"
	default:
		return "none"
	}
}

func mapProvisioningType(pt DiskProvisioningType) string {
	switch pt {
	case DiskProvisioningTypeThin:
		return "lazy"
	case DiskProvisioningTypeThick:
		return "eager"
	case DiskProvisioningTypeThickEagerZero:
		return "eagerzero"
	default:
		return "lazy"
	}
}

var (
	// ErrBusLogicNot64BitGuest indicates BusLogic SCSI adapters are not supported for 64-bit guests
	ErrBusLogicNot64BitGuest = errors.New("the BusLogic SCSI adapter is not supported for 64-bit guests")

	// ErrBusLogicNotWindows7Plus indicates BusLogic SCSI adapters are not supported for Windows 7 or later (including 32-bit)
	ErrBusLogicNotWindows7Plus = errors.New("the BusLogic SCSI adapter is not supported for Windows 7 or later guests")

	// ErrLsiLogicNotWindows8Plus indicates LSI Logic SCSI adapters are not supported for Windows 8 or higher
	ErrLsiLogicNotWindows8Plus = errors.New("the LSI Logic SCSI adapter is not supported for Windows 8 or higher guests")

	// ErrLegacyDiskModes indicates legacy disk modes cannot be used in new VMs
	ErrLegacyDiskModes = errors.New("cannot use legacy disk modes (undoable, append, nonpersistent) in new VM")

	// ErrVirtualSharingPersistentThick indicates virtual sharing with persistent disk requires thick provisioning
	ErrVirtualSharingPersistentThick = errors.New("virtual sharing with persistent disk (non-multi-writer) requires thick provisioning")

	// ErrVirtualSharingIndependentPersistentThick indicates virtual sharing with independent_persistent disk requires thick provisioning
	ErrVirtualSharingIndependentPersistentThick = errors.New("virtual sharing with independent_persistent disk (non-multi-writer) requires thick provisioning")

	// ErrVirtualSharingIndependentNonPersistentThick indicates virtual sharing with independent_nonpersistent disk requires thick provisioning
	ErrVirtualSharingIndependentNonPersistentThick = errors.New("virtual sharing with independent_nonpersistent disk requires thick provisioning")

	// ErrPhysicalSharingPersistentThick indicates physical sharing with persistent disk requires thick provisioning
	ErrPhysicalSharingPersistentThick = errors.New("physical sharing with persistent disk (non-multi-writer) requires thick provisioning")

	// ErrPhysicalSharingIndependentPersistentThick indicates physical sharing with independent_persistent disk requires thick provisioning
	ErrPhysicalSharingIndependentPersistentThick = errors.New("physical sharing with independent_persistent disk (non-multi-writer) requires thick provisioning")

	// ErrPhysicalSharingIndependentNonPersistentThick indicates physical sharing with independent_nonpersistent disk requires thick provisioning
	ErrPhysicalSharingIndependentNonPersistentThick = errors.New("physical sharing with independent_nonpersistent disk requires thick provisioning")

	// ErrMultiWriterIndependentNonPersistentThick indicates multi-writer disks with independent_nonpersistent mode require thick provisioning
	ErrMultiWriterIndependentNonPersistentThick = errors.New("multi-writer disks with independent_nonpersistent mode require thick provisioning")

	// ErrInvalidConfiguration indicates an invalid configuration that cannot be created
	ErrInvalidConfiguration = errors.New("invalid configuration")
)

var unsupportedCombinations = map[string]error{
	// ============================================================================
	// Unsupported combinations
	// ============================================================================

	"scsi-buslogic-none-persistent-none-lazy-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-eager-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-eagerzero-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-persistent-multi-writer-eagerzero-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-lazy-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-eager-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_nonpersistent-none-lazy-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-persistent-none-lazy-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-buslogic-virtual-persistent-none-lazy-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-buslogic-virtual-persistent-none-eager-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-buslogic-virtual-persistent-none-eager-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-buslogic-virtual-persistent-none-eagerzero-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-persistent-multi-writer-eagerzero-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-none-lazy-otherlinux64": ErrVirtualSharingIndependentPersistentThick,
	"scsi-buslogic-virtual-independent_persistent-none-lazy-otherguest": ErrVirtualSharingIndependentPersistentThick,
	"scsi-buslogic-virtual-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-none-eager-otherguest": ErrVirtualSharingIndependentPersistentThick,
	"scsi-buslogic-virtual-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-virtual-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-persistent-none-lazy-otherlinux64": ErrPhysicalSharingPersistentThick,
	"scsi-buslogic-physical-persistent-none-lazy-otherguest": ErrPhysicalSharingPersistentThick,
	"scsi-buslogic-physical-persistent-none-eager-otherlinux64": ErrPhysicalSharingPersistentThick,
	"scsi-buslogic-physical-persistent-none-eager-otherguest": ErrPhysicalSharingPersistentThick,
	"scsi-buslogic-physical-persistent-none-eagerzero-otherlinux64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-none-lazy-otherguest": ErrPhysicalSharingIndependentPersistentThick,
	"scsi-buslogic-physical-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_nonpersistent-none-lazy-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-lazy-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eager-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eager-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-undoable-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-buslogic-physical-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-buslogic-physical-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_nonpersistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-persistent-none-lazy-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-virtual-persistent-none-lazy-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-virtual-persistent-none-eager-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-virtual-persistent-none-eager-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-virtual-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-none-lazy-otherguest": ErrVirtualSharingIndependentPersistentThick,
	"scsi-lsilogic-virtual-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-none-eager-otherguest": ErrVirtualSharingIndependentPersistentThick,
	"scsi-lsilogic-virtual-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-virtual-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-persistent-none-lazy-otherlinux64": ErrPhysicalSharingPersistentThick,
	"scsi-lsilogic-physical-persistent-none-lazy-otherguest": ErrPhysicalSharingPersistentThick,
	"scsi-lsilogic-physical-persistent-none-eager-otherlinux64": ErrPhysicalSharingPersistentThick,
	"scsi-lsilogic-physical-persistent-none-eager-otherguest": ErrPhysicalSharingPersistentThick,
	"scsi-lsilogic-physical-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-none-lazy-otherguest": ErrPhysicalSharingIndependentPersistentThick,
	"scsi-lsilogic-physical-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_nonpersistent-none-lazy-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-lazy-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eager-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eager-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-undoable-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-physical-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-undoable-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-persistent-none-lazy-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-sas-virtual-persistent-none-lazy-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-sas-virtual-persistent-none-eager-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-sas-virtual-persistent-none-eager-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-lsilogic-sas-virtual-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-append-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-virtual-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-none-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-lazy-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-lazy-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-eager-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-eager-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-undoable-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-undoable-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-append-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-lsilogic-sas-physical-append-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-persistent-none-lazy-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-pvscsi-virtual-persistent-none-lazy-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-pvscsi-virtual-persistent-none-eager-otherlinux64": ErrVirtualSharingPersistentThick,
	"scsi-pvscsi-virtual-persistent-none-eager-otherguest": ErrVirtualSharingPersistentThick,
	"scsi-pvscsi-virtual-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-none-lazy-otherlinux64": ErrVirtualSharingIndependentPersistentThick,
	"scsi-pvscsi-virtual-independent_persistent-none-lazy-otherguest": ErrVirtualSharingIndependentPersistentThick,
	"scsi-pvscsi-virtual-independent_persistent-none-eager-otherlinux64": ErrVirtualSharingIndependentPersistentThick,
	"scsi-pvscsi-virtual-independent_persistent-none-eager-otherguest": ErrVirtualSharingIndependentPersistentThick,
	"scsi-pvscsi-virtual-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-lazy-otherguest": ErrVirtualSharingIndependentNonPersistentThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-virtual-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-persistent-none-lazy-otherlinux64": ErrPhysicalSharingPersistentThick,
	"scsi-pvscsi-physical-persistent-none-lazy-otherguest": ErrPhysicalSharingPersistentThick,
	"scsi-pvscsi-physical-persistent-none-eager-otherlinux64": ErrPhysicalSharingPersistentThick,
	"scsi-pvscsi-physical-persistent-none-eager-otherguest": ErrPhysicalSharingPersistentThick,
	"scsi-pvscsi-physical-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-none-lazy-otherlinux64": ErrPhysicalSharingIndependentPersistentThick,
	"scsi-pvscsi-physical-independent_persistent-none-lazy-otherguest": ErrPhysicalSharingIndependentPersistentThick,
	"scsi-pvscsi-physical-independent_persistent-none-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-none-eager-otherguest": ErrPhysicalSharingIndependentPersistentThick,
	"scsi-pvscsi-physical-independent_persistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_nonpersistent-none-lazy-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-lazy-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eager-otherlinux64": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eager-otherguest": ErrPhysicalSharingIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eager-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"scsi-pvscsi-physical-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"ide-ide-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"ide-ide-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"ide-ide-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"ide-ide-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"sata-ahci-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"sata-ahci-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_persistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_persistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_persistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_persistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_persistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_persistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_nonpersistent-none-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_nonpersistent-none-eagerzero-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-lazy-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-lazy-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterIndependentNonPersistentThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eager-otherguest": ErrMultiWriterIndependentNonPersistentThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eagerzero-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-nonpersistent-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-lazy-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eager-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eager-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-nonpersistent-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-nonpersistent-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-nonpersistent-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-nonpersistent-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-lazy-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eager-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eager-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-undoable-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-undoable-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-undoable-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-undoable-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-lazy-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-lazy-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eager-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eager-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eagerzero-otherguest": ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-lazy-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-append-multi-writer-lazy-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-append-multi-writer-eager-otherlinux64": ErrInvalidConfiguration,
	"nvme-nvme-none-append-multi-writer-eager-otherguest": ErrInvalidConfiguration,
	"nvme-nvme-none-append-multi-writer-eagerzero-otherlinux64": ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-eagerzero-otherguest": ErrLegacyDiskModes,

	// Total unsupported: 957

	// ============================================================================
	// Supported combinations (commented out to indicate support)
	// ============================================================================

	// "scsi-buslogic-none-persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-buslogic-none-persistent-none-eager-otherguest": nil, // Supported
	// "scsi-buslogic-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-buslogic-none-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "scsi-buslogic-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-buslogic-none-independent_persistent-none-eager-otherguest": nil, // Supported
	// "scsi-buslogic-none-independent_persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-buslogic-none-independent_nonpersistent-none-lazy-otherguest": nil, // Supported
	// "scsi-buslogic-none-independent_nonpersistent-none-eager-otherguest": nil, // Supported
	// "scsi-buslogic-virtual-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-buslogic-virtual-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "scsi-buslogic-physical-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-none-persistent-none-lazy-otherlinux64": nil, // Supported
	// "scsi-lsilogic-none-persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-lsilogic-none-persistent-none-eager-otherlinux64": nil, // Supported
	// "scsi-lsilogic-none-persistent-none-eager-otherguest": nil, // Supported
	// "scsi-lsilogic-none-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-none-persistent-multi-writer-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-none-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-none-independent_persistent-none-lazy-otherlinux64": nil, // Supported
	// "scsi-lsilogic-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-lsilogic-none-independent_persistent-none-eager-otherlinux64": nil, // Supported
	// "scsi-lsilogic-none-independent_persistent-none-eager-otherguest": nil, // Supported
	// "scsi-lsilogic-none-independent_nonpersistent-none-lazy-otherguest": nil, // Supported
	// "scsi-lsilogic-none-independent_nonpersistent-none-eager-otherguest": nil, // Supported
	// "scsi-lsilogic-virtual-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-virtual-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-virtual-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-physical-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-physical-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-sas-none-persistent-none-lazy-otherlinux64": nil, // Supported
	// "scsi-lsilogic-sas-none-persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-lsilogic-sas-none-persistent-none-eager-otherlinux64": nil, // Supported
	// "scsi-lsilogic-sas-none-persistent-none-eager-otherguest": nil, // Supported
	// "scsi-lsilogic-sas-none-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-sas-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-sas-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-lsilogic-sas-virtual-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-sas-virtual-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-lsilogic-sas-physical-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-lsilogic-sas-physical-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-none-persistent-none-lazy-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-pvscsi-none-persistent-none-eager-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-persistent-none-eager-otherguest": nil, // Supported
	// "scsi-pvscsi-none-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-none-persistent-multi-writer-eagerzero-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-none-independent_persistent-none-lazy-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "scsi-pvscsi-none-independent_persistent-none-eager-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-independent_persistent-none-eager-otherguest": nil, // Supported
	// "scsi-pvscsi-none-independent_persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-none-independent_nonpersistent-none-lazy-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-independent_nonpersistent-none-lazy-otherguest": nil, // Supported
	// "scsi-pvscsi-none-independent_nonpersistent-none-eager-otherlinux64": nil, // Supported
	// "scsi-pvscsi-none-independent_nonpersistent-none-eager-otherguest": nil, // Supported
	// "scsi-pvscsi-virtual-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-pvscsi-virtual-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-virtual-persistent-multi-writer-eagerzero-otherlinux64": nil, // Supported
	// "scsi-pvscsi-virtual-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-physical-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "scsi-pvscsi-physical-persistent-none-eagerzero-otherguest": nil, // Supported
	// "scsi-pvscsi-physical-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "ide-ide-none-persistent-none-lazy-otherlinux64": nil, // Supported
	// "ide-ide-none-persistent-none-lazy-otherguest": nil, // Supported
	// "ide-ide-none-persistent-none-eager-otherlinux64": nil, // Supported
	// "ide-ide-none-persistent-none-eager-otherguest": nil, // Supported
	// "ide-ide-none-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "ide-ide-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "ide-ide-none-persistent-multi-writer-eagerzero-otherlinux64": nil, // Supported
	// "ide-ide-none-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "ide-ide-none-independent_persistent-none-lazy-otherlinux64": nil, // Supported
	// "ide-ide-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "ide-ide-none-independent_persistent-none-eager-otherlinux64": nil, // Supported
	// "ide-ide-none-independent_persistent-none-eager-otherguest": nil, // Supported
	// "ide-ide-none-independent_persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "ide-ide-none-independent_persistent-none-eagerzero-otherguest": nil, // Supported
	// "ide-ide-none-independent_nonpersistent-none-lazy-otherlinux64": nil, // Supported
	// "ide-ide-none-independent_nonpersistent-none-lazy-otherguest": nil, // Supported
	// "ide-ide-none-independent_nonpersistent-none-eager-otherlinux64": nil, // Supported
	// "ide-ide-none-independent_nonpersistent-none-eager-otherguest": nil, // Supported
	// "ide-ide-none-independent_nonpersistent-none-eagerzero-otherlinux64": nil, // Supported
	// "ide-ide-none-independent_nonpersistent-none-eagerzero-otherguest": nil, // Supported
	// "sata-ahci-none-persistent-none-lazy-otherlinux64": nil, // Supported
	// "sata-ahci-none-persistent-none-lazy-otherguest": nil, // Supported
	// "sata-ahci-none-persistent-none-eager-otherlinux64": nil, // Supported
	// "sata-ahci-none-persistent-none-eager-otherguest": nil, // Supported
	// "sata-ahci-none-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "sata-ahci-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "sata-ahci-none-persistent-multi-writer-eagerzero-otherlinux64": nil, // Supported
	// "sata-ahci-none-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "sata-ahci-none-independent_persistent-none-lazy-otherlinux64": nil, // Supported
	// "sata-ahci-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "sata-ahci-none-independent_persistent-none-eager-otherlinux64": nil, // Supported
	// "sata-ahci-none-independent_persistent-none-eager-otherguest": nil, // Supported
	// "sata-ahci-none-independent_persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "sata-ahci-none-independent_persistent-none-eagerzero-otherguest": nil, // Supported
	// "sata-ahci-none-independent_nonpersistent-none-lazy-otherlinux64": nil, // Supported
	// "sata-ahci-none-independent_nonpersistent-none-lazy-otherguest": nil, // Supported
	// "sata-ahci-none-independent_nonpersistent-none-eager-otherlinux64": nil, // Supported
	// "sata-ahci-none-independent_nonpersistent-none-eager-otherguest": nil, // Supported
	// "sata-ahci-none-independent_nonpersistent-none-eagerzero-otherguest": nil, // Supported
	// "nvme-nvme-none-persistent-none-lazy-otherlinux64": nil, // Supported
	// "nvme-nvme-none-persistent-none-lazy-otherguest": nil, // Supported
	// "nvme-nvme-none-persistent-none-eager-otherlinux64": nil, // Supported
	// "nvme-nvme-none-persistent-none-eager-otherguest": nil, // Supported
	// "nvme-nvme-none-persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "nvme-nvme-none-persistent-none-eagerzero-otherguest": nil, // Supported
	// "nvme-nvme-none-persistent-multi-writer-eagerzero-otherlinux64": nil, // Supported
	// "nvme-nvme-none-persistent-multi-writer-eagerzero-otherguest": nil, // Supported
	// "nvme-nvme-none-independent_persistent-none-lazy-otherlinux64": nil, // Supported
	// "nvme-nvme-none-independent_persistent-none-lazy-otherguest": nil, // Supported
	// "nvme-nvme-none-independent_persistent-none-eager-otherlinux64": nil, // Supported
	// "nvme-nvme-none-independent_persistent-none-eager-otherguest": nil, // Supported
	// "nvme-nvme-none-independent_persistent-none-eagerzero-otherlinux64": nil, // Supported
	// "nvme-nvme-none-independent_persistent-none-eagerzero-otherguest": nil, // Supported
	// "nvme-nvme-none-independent_nonpersistent-none-lazy-otherlinux64": nil, // Supported
	// "nvme-nvme-none-independent_nonpersistent-none-lazy-otherguest": nil, // Supported
	// "nvme-nvme-none-independent_nonpersistent-none-eager-otherlinux64": nil, // Supported
	// "nvme-nvme-none-independent_nonpersistent-none-eager-otherguest": nil, // Supported

	// ============================================================================
	// BusLogic SCSI - Windows 7 (32-bit) restrictions
	// BusLogic is NOT supported for Windows 7 or later (any bitness)
	// ============================================================================

	"scsi-buslogic-none-append-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-append-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-append-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-append-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-append-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-append-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_nonpersistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_nonpersistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_nonpersistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_persistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_persistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_persistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-independent_persistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-nonpersistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-nonpersistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-nonpersistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-nonpersistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-persistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-persistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-persistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-persistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-persistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-persistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-undoable-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-undoable-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-undoable-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-undoable-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-undoable-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-none-undoable-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-append-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-append-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-append-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-append-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-append-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-append-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_nonpersistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_nonpersistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_nonpersistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_persistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_persistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-independent_persistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-nonpersistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-nonpersistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-nonpersistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-persistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-persistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-persistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-persistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-persistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-undoable-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-undoable-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-undoable-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-undoable-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-physical-undoable-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-append-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-append-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-append-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-append-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-append-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-append-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_nonpersistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_persistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_persistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-independent_persistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-nonpersistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-nonpersistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-nonpersistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-persistent-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-persistent-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-persistent-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-persistent-none-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-undoable-multi-writer-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-undoable-none-eager-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-undoable-none-eagerzero-windows7": ErrBusLogicNotWindows7Plus,
	"scsi-buslogic-virtual-undoable-none-lazy-windows7": ErrBusLogicNotWindows7Plus,

	// ============================================================================
	// BusLogic SCSI - Windows 7 (64-bit) restrictions
	// BusLogic is NOT supported for 64-bit guests
	// ============================================================================

	"scsi-buslogic-none-append-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-append-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-append-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-append-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-append-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-append-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-append-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-append-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-append-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-append-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-append-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-append-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_nonpersistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_nonpersistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_nonpersistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-append-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-append-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-append-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-append-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-append-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-append-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_nonpersistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-multi-writer-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-none-eager-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-none-eagerzero-windows7_64": ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-none-lazy-windows7_64": ErrBusLogicNot64BitGuest,

	// ============================================================================
	// LSI Logic SCSI - Windows 8+ restrictions
	// LSI Logic is NOT supported for Windows 8 or higher (any bitness)
	// ============================================================================

	"scsi-lsilogic-none-append-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-append-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-append-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-append-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-append-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-append-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_nonpersistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_nonpersistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_nonpersistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_persistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_persistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_persistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-independent_persistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-nonpersistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-nonpersistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-nonpersistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-persistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-persistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-persistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-persistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-persistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-persistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-undoable-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-undoable-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-undoable-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-undoable-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-none-undoable-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-append-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-append-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-append-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-append-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-append-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-append-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_nonpersistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_persistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_persistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-independent_persistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-nonpersistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-nonpersistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-nonpersistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-persistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-persistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-persistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-persistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-persistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-persistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-undoable-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-undoable-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-undoable-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-physical-undoable-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-append-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-append-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-append-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-append-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-append-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_persistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_persistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-independent_persistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-nonpersistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-nonpersistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-nonpersistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-persistent-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-persistent-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-persistent-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-persistent-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-persistent-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-persistent-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-undoable-multi-writer-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-undoable-none-eager-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-undoable-none-eagerzero-windows8_64": ErrLsiLogicNotWindows8Plus,
	"scsi-lsilogic-virtual-undoable-none-lazy-windows8_64": ErrLsiLogicNotWindows8Plus,

	// ============================================================================
	// Summary:
	// - Total unsupported combinations: 1281
	//   - Original entries (otherguest/otherlinux64): 957
	//   - Windows 7 (32-bit) entries: 108
	//   - Windows 7 (64-bit) entries: 108
	//   - Windows 8+ entries: 108
	// - Total supported combinations (commented out): 123
	// ============================================================================
}

