// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"errors"
	"fmt"
	"strings"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// DatastoreType represents the type of datastore.
type DatastoreType string

const (
	DatastoreTypeVMFS DatastoreType = "VMFS"
	DatastoreTypeVSAN DatastoreType = "VSAN"
)

// DiskProvisioningType represents the type of disk provisioning.
type DiskProvisioningType string

const (
	DiskProvisioningTypeThin           DiskProvisioningType = "Thin"
	DiskProvisioningTypeThick          DiskProvisioningType = "Thick"
	DiskProvisioningTypeThickEagerZero DiskProvisioningType = "ThickEagerZero"
)

const (
	// guestIDOtherLinux64 is the normalized guest ID for 64-bit Linux guests.
	guestIDOtherLinux64 = "otherlinux64"
	// guestIDWindows864 is the normalized guest ID for Windows 8+ 64-bit
	// guests.
	guestIDWindows864 = "windows8_64"
	// sharingModeNone represents no sharing mode.
	sharingModeNone = "none"
)

// IsControllerDiskSupportedOption is an interface for options that can be
// applied to IsControllerDiskSupported validation.
type IsControllerDiskSupportedOption interface {
	// ApplyToIsControllerDiskSupported applies this configuration to the
	// given options.
	ApplyToIsControllerDiskSupported(*IsControllerDiskSupportedOptions)
}

// IsControllerDiskSupportedOptions contains all the options for validating
// controller/disk combinations.
type IsControllerDiskSupportedOptions struct {
	DatastoreType        DatastoreType
	ControllerType       vmopv1.VirtualControllerType
	ScsiControllerType   vmopv1.SCSIControllerType
	ControllerSharing    vmopv1.VirtualControllerSharingMode
	DiskMode             vmopv1.VolumeDiskMode
	DiskSharing          vmopv1.VolumeSharingMode
	DiskProvisioningType DiskProvisioningType
	GuestID              string
}

// WithDatastoreType specifies the datastore type.
type WithDatastoreType DatastoreType

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithDatastoreType) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.DatastoreType = DatastoreType(w)
}

// WithControllerType specifies the virtual controller type.
type WithControllerType vmopv1.VirtualControllerType

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithControllerType) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.ControllerType = vmopv1.VirtualControllerType(w)
}

// WithScsiControllerType specifies the SCSI controller type.
type WithScsiControllerType vmopv1.SCSIControllerType

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithScsiControllerType) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.ScsiControllerType = vmopv1.SCSIControllerType(w)
}

// WithControllerSharing specifies the controller sharing mode.
type WithControllerSharing vmopv1.VirtualControllerSharingMode

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithControllerSharing) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.ControllerSharing = vmopv1.VirtualControllerSharingMode(w)
}

// WithDiskMode specifies the disk mode.
type WithDiskMode vmopv1.VolumeDiskMode

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithDiskMode) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.DiskMode = vmopv1.VolumeDiskMode(w)
}

// WithDiskSharing specifies the disk sharing mode.
type WithDiskSharing vmopv1.VolumeSharingMode

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithDiskSharing) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.DiskSharing = vmopv1.VolumeSharingMode(w)
}

// WithDiskProvisioningType specifies the disk provisioning type.
type WithDiskProvisioningType DiskProvisioningType

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithDiskProvisioningType) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.DiskProvisioningType = DiskProvisioningType(w)
}

// WithGuestID specifies the guest operating system ID.
type WithGuestID string

// ApplyToIsControllerDiskSupported applies this configuration to the given
// options.
func (w WithGuestID) ApplyToIsControllerDiskSupported(
	opts *IsControllerDiskSupportedOptions) {

	opts.GuestID = string(w)
}

// IsControllerDiskSupported validates whether a given combination of
// controller type, controller sharing mode, disk mode, disk sharing mode, and
// guest ID is supported by vSphere based on empirical testing.
//
// The function recognizes the following guest IDs and validates controller
// compatibility:
//   - otherLinuxGuest (32-bit) / otherLinux64Guest (64-bit): Generic Linux
//     guests
//   - windows7Guest (32-bit) / windows7_64Guest (64-bit): Windows 7 guests
//   - windows8Guest and higher (32-bit): Windows 8+ guests (32-bit)
//   - windows8_64Guest and higher (64-bit): Windows 8+ guests (64-bit)
//
// Guest OS specific restrictions:
//   - BusLogic SCSI: NOT supported for 64-bit guests or Windows 7+ (any
//     bitness)
//   - LSI Logic SCSI: NOT supported for Windows 8+ guests (any bitness)
//
// If the provided guest ID is not recognized, it is treated as
// otherLinux64Guest. If the datastore type is not known, it defaults to VMFS.
//
// Returns nil if the combination is supported, or an error if unsupported.
func IsControllerDiskSupported(opts ...IsControllerDiskSupportedOption) error {

	// Apply all options
	options := &IsControllerDiskSupportedOptions{}
	for _, opt := range opts {
		opt.ApplyToIsControllerDiskSupported(options)
	}

	// Default to VMFS if datastore type is unknown or empty.
	if options.DatastoreType == "" {
		options.DatastoreType = DatastoreTypeVMFS
	}

	// Validate and normalize guest ID.
	guestID := getGuestID(options.GuestID)

	// Map vmopv1 types to CSV format
	controllerTypeStr := mapControllerType(options.ControllerType)
	controllerSubTypeStr := mapControllerSubType(
		options.ControllerType, options.ScsiControllerType)
	controllerSharingStr := mapControllerSharing(options.ControllerSharing)
	diskModeStr := mapDiskMode(options.DiskMode)
	diskSharingStr := mapDiskSharing(options.DiskSharing)
	provisioningStr := mapProvisioningType(options.DiskProvisioningType)

	// Create lookup key.
	key := fmt.Sprintf("%s-%s-%s-%s-%s-%s-%s",
		controllerTypeStr,
		controllerSubTypeStr,
		controllerSharingStr,
		diskModeStr,
		diskSharingStr,
		provisioningStr,
		guestID)

	// Check the lookup table based on datastore type.
	switch options.DatastoreType {
	case DatastoreTypeVMFS:
		// Check VMFS-specific failures first
		if err, ok := vmfsUnsupportedCombinations[key]; ok {
			return err
		}
	case DatastoreTypeVSAN:
		// Check VSAN-specific failures first
		if err, ok := vsanUnsupportedCombinations[key]; ok {
			return err
		}
	}

	// Check common failures that apply to all datastore types
	if err, ok := allDatastoreUnsupportedCombinations[key]; ok {
		return err
	}

	return nil
}

// getGuestID normalizes and validates a guest ID string for compatibility
// checking. It maps various guest ID formats to standardized values used in
// the validation lookup tables.
func getGuestID(guestID string) string {
	guestID = strings.ToLower(guestID)
	switch guestID {
	case "otherlinuxguest":
		return "otherguest"
	case "otherlinux64guest":
		return guestIDOtherLinux64
	case "windows7guest":
		return "windows7"
	case "windows7_64guest", "windows7-64", "windows764":
		return "windows7_64"
	case "windows8guest":
		return "windows8"
	case "windows8_64guest", "windows8-64", "windows864":
		return guestIDWindows864
	case "windows8serverguest", "windows8server64guest", "windows2012guest",
		"windows2012_64guest":
		return guestIDWindows864
	case "windows9guest", "windows9_64guest":
		return guestIDWindows864
	case "windows10guest", "windows10_64guest":
		return guestIDWindows864
	case "windows11guest", "windows11_64guest":
		return guestIDWindows864
	case "windows2016guest", "windows2016_64guest", "windows2019guest",
		"windows2019_64guest",
		"windows2022guest", "windows2022_64guest":
		return guestIDWindows864
	default:
		// Check if it starts with "windows" and contains "7", "8", "9", "10",
		// "11", "2012", "2016", "2019", "2022".
		switch {
		case strings.Contains(guestID, "windows"):
			// If it contains 7, treat as Windows 7
			switch {
			case strings.Contains(guestID, "7"):
				switch {
				case strings.Contains(guestID, "64") ||
					strings.Contains(guestID, "_64"):
					return "windows7_64"
				default:
					return "windows7"
				}
			case strings.Contains(guestID, "8") ||
				strings.Contains(guestID, "9") ||
				strings.Contains(guestID, "10") ||
				strings.Contains(guestID, "11") ||
				strings.Contains(guestID, "2012") ||
				strings.Contains(guestID, "2016") ||
				strings.Contains(guestID, "2019") ||
				strings.Contains(guestID, "2022"):
				// Windows 8+ (all map to windows8_64 for validation
				// purposes).
				return guestIDWindows864
			default:
				// Unknown Windows version, default to otherlinux64
				return guestIDOtherLinux64
			}
		default:
			// Default unknown guest IDs to otherlinux64
			return guestIDOtherLinux64
		}
	}
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
	scsiType vmopv1.SCSIControllerType,
) string {
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
		return sharingModeNone
	case vmopv1.VirtualControllerSharingModeVirtual:
		return "virtual"
	case vmopv1.VirtualControllerSharingModePhysical:
		return "physical"
	default:
		return sharingModeNone
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
		return sharingModeNone
	case vmopv1.VolumeSharingModeMultiWriter:
		return "multi-writer"
	default:
		return sharingModeNone
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
	// ErrBusLogicNot64BitGuest indicates BusLogic SCSI adapters are not
	// supported for 64-bit guests.
	ErrBusLogicNot64BitGuest = errors.New("the BusLogic SCSI adapter is not supported for 64-bit guests")

	// ErrLegacyDiskModes indicates legacy disk modes cannot be used in new
	// VMs.
	ErrLegacyDiskModes = errors.New("cannot use legacy disk modes (undoable, append, nonpersistent) in new VM")

	// ErrMultiWriterRequiresThick indicates multi-writer disks require thick
	// (eager zero) provisioning.
	ErrMultiWriterRequiresThick = errors.New("multi-writer disks or controller sharing require thick eager-zero provisioning")

	// ErrIncompatibleBackingSharingVMFS indicates incompatible device backing
	// sharing on VMFS datastores.
	ErrIncompatibleBackingSharingVMFS = errors.New("incompatible device backing specified for sharing on VMFS datastore")

	// ErrInvalidConfiguration indicates an invalid configuration that cannot
	// be created.
	ErrInvalidConfiguration = errors.New("invalid configuration")
)

// vmfsUnsupportedCombinations contains configurations that fail only on VMFS
// datastores.
var vmfsUnsupportedCombinations = map[string]error{
	"ide-ide-none-append-multi-writer-eager-otherguest":                                ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-append-multi-writer-eager-otherlinux64":                              ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-append-multi-writer-lazy-otherguest":                                 ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-append-multi-writer-lazy-otherlinux64":                               ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_nonpersistent-multi-writer-eager-otherguest":             ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_nonpersistent-multi-writer-eager-otherlinux64":           ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_nonpersistent-multi-writer-lazy-otherguest":              ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":            ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_persistent-multi-writer-eager-otherguest":                ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_persistent-multi-writer-eager-otherlinux64":              ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_persistent-multi-writer-lazy-otherguest":                 ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-independent_persistent-multi-writer-lazy-otherlinux64":               ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-nonpersistent-multi-writer-eager-otherguest":                         ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-nonpersistent-multi-writer-eager-otherlinux64":                       ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-nonpersistent-multi-writer-lazy-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-nonpersistent-multi-writer-lazy-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-persistent-multi-writer-eager-otherguest":                            ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-persistent-multi-writer-eager-otherlinux64":                          ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-persistent-multi-writer-lazy-otherguest":                             ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-persistent-multi-writer-lazy-otherlinux64":                           ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-undoable-multi-writer-eager-otherguest":                              ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-undoable-multi-writer-eager-otherlinux64":                            ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-undoable-multi-writer-lazy-otherguest":                               ErrIncompatibleBackingSharingVMFS,
	"ide-ide-none-undoable-multi-writer-lazy-otherlinux64":                             ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-append-multi-writer-eager-otherguest":                              ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-append-multi-writer-eager-otherlinux64":                            ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-append-multi-writer-lazy-otherguest":                               ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-append-multi-writer-lazy-otherlinux64":                             ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eager-otherguest":           ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eager-otherlinux64":         ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-lazy-otherguest":            ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":          ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_persistent-multi-writer-eager-otherguest":              ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_persistent-multi-writer-eager-otherlinux64":            ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_persistent-multi-writer-lazy-otherguest":               ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-independent_persistent-multi-writer-lazy-otherlinux64":             ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-nonpersistent-multi-writer-eager-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-nonpersistent-multi-writer-eager-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-nonpersistent-multi-writer-lazy-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-nonpersistent-multi-writer-lazy-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-persistent-multi-writer-eager-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-persistent-multi-writer-eager-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-persistent-multi-writer-lazy-otherguest":                           ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-persistent-multi-writer-lazy-otherlinux64":                         ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-undoable-multi-writer-eager-otherguest":                            ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-undoable-multi-writer-eager-otherlinux64":                          ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-undoable-multi-writer-lazy-otherguest":                             ErrIncompatibleBackingSharingVMFS,
	"nvme-nvme-none-undoable-multi-writer-lazy-otherlinux64":                           ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-append-multi-writer-eager-otherguest":                              ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-append-multi-writer-eager-otherlinux64":                            ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-append-multi-writer-lazy-otherguest":                               ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-append-multi-writer-lazy-otherlinux64":                             ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eager-otherguest":           ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eager-otherlinux64":         ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_nonpersistent-multi-writer-lazy-otherguest":            ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":          ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_persistent-multi-writer-eager-otherguest":              ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_persistent-multi-writer-eager-otherlinux64":            ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_persistent-multi-writer-lazy-otherguest":               ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-independent_persistent-multi-writer-lazy-otherlinux64":             ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-nonpersistent-multi-writer-eager-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-nonpersistent-multi-writer-eager-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-nonpersistent-multi-writer-lazy-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-nonpersistent-multi-writer-lazy-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-persistent-multi-writer-eager-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-persistent-multi-writer-eager-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-persistent-multi-writer-lazy-otherguest":                           ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-persistent-multi-writer-lazy-otherlinux64":                         ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-undoable-multi-writer-eager-otherguest":                            ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-undoable-multi-writer-eager-otherlinux64":                          ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-undoable-multi-writer-lazy-otherguest":                             ErrIncompatibleBackingSharingVMFS,
	"sata-ahci-none-undoable-multi-writer-lazy-otherlinux64":                           ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-append-multi-writer-eager-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-append-multi-writer-eager-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-append-multi-writer-lazy-otherguest":                           ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-append-multi-writer-lazy-otherlinux64":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-otherguest":        ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-otherguest":          ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-otherlinux64":        ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-otherguest":           ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-otherlinux64":         ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-persistent-multi-writer-eager-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-persistent-multi-writer-eager-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-persistent-multi-writer-lazy-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-persistent-multi-writer-lazy-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-undoable-multi-writer-eager-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-undoable-multi-writer-eager-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-undoable-multi-writer-lazy-otherguest":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-none-undoable-multi-writer-lazy-otherlinux64":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-append-multi-writer-eager-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-append-multi-writer-eager-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-append-multi-writer-lazy-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-append-multi-writer-lazy-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-otherguest":   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-otherguest":    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64":  ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-otherguest":      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-otherlinux64":    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-independent_persistent-none-eager-otherguest":              ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_persistent-none-lazy-otherguest":               ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-otherguest":               ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-otherlinux64":             ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-otherguest":                ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-otherlinux64":              ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-persistent-multi-writer-eager-otherguest":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-persistent-multi-writer-eager-otherlinux64":                ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-persistent-none-eager-otherguest":                          ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-persistent-none-lazy-otherguest":                           ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-undoable-multi-writer-eager-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-undoable-multi-writer-eager-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-append-multi-writer-eager-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-append-multi-writer-eager-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-append-multi-writer-lazy-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-append-multi-writer-lazy-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-otherguest":    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64":  ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-otherguest":     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64":   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-otherguest":        ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-otherlinux64":      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-independent_persistent-none-eager-otherguest":               ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_persistent-none-lazy-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-otherguest":                ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-otherlinux64":              ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-otherguest":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-otherlinux64":               ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-persistent-none-eager-otherguest":                           ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-persistent-none-lazy-otherguest":                            ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-append-multi-writer-eager-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-append-multi-writer-eager-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-append-multi-writer-lazy-otherguest":                           ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-append-multi-writer-lazy-otherlinux64":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-otherguest":        ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eager-otherguest":          ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_persistent-multi-writer-eager-otherlinux64":        ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_persistent-multi-writer-lazy-otherguest":           ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-independent_persistent-multi-writer-lazy-otherlinux64":         ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-persistent-multi-writer-eager-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-persistent-multi-writer-eager-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-persistent-multi-writer-lazy-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-persistent-multi-writer-lazy-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-undoable-multi-writer-eager-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-undoable-multi-writer-eager-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-otherguest":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-otherlinux64":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-append-multi-writer-eager-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-append-multi-writer-eager-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-append-multi-writer-lazy-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-append-multi-writer-lazy-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-otherguest":   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-otherguest":    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64":  ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eager-otherguest":      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-eager-otherlinux64":    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-lazy-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_persistent-multi-writer-lazy-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-independent_persistent-none-eager-otherguest":              ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_persistent-none-eager-otherlinux64":            ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_persistent-none-lazy-otherguest":               ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_persistent-none-lazy-otherlinux64":             ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-otherguest":               ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-otherlinux64":             ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-otherguest":                ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-otherlinux64":              ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-persistent-multi-writer-eager-otherguest":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-persistent-multi-writer-eager-otherlinux64":                ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-persistent-multi-writer-lazy-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-persistent-multi-writer-lazy-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-persistent-none-eager-otherguest":                          ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-persistent-none-eager-otherlinux64":                        ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-persistent-none-lazy-otherguest":                           ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-persistent-none-lazy-otherlinux64":                         ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-sas-physical-independent_persistent-none-eager":                     ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-independent_persistent-none-lazy":                      ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-persistent-none-eager":                                 ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-persistent-none-lazy":                                  ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-eager":                      ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-independent_persistent-none-lazy":                       ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-persistent-none-eager":                                  ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-persistent-none-lazy":                                   ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-append-multi-writer-eager-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-append-multi-writer-eager-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-otherguest":    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64":  ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-otherguest":     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64":   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eager-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-eager-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-lazy-otherguest":        ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_persistent-multi-writer-lazy-otherlinux64":      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-independent_persistent-none-eager-otherguest":               ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_persistent-none-eager-otherlinux64":             ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_persistent-none-lazy-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_persistent-none-lazy-otherlinux64":              ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-otherguest":                ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-otherlinux64":              ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-otherguest":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-otherlinux64":               ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-persistent-multi-writer-eager-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-persistent-multi-writer-eager-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-persistent-multi-writer-lazy-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-persistent-multi-writer-lazy-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-persistent-none-eager-otherguest":                           ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-persistent-none-eager-otherlinux64":                         ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-persistent-none-lazy-otherguest":                            ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-persistent-none-lazy-otherlinux64":                          ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-append-multi-writer-eager-otherguest":                            ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-append-multi-writer-eager-otherlinux64":                          ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-append-multi-writer-lazy-otherguest":                             ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-append-multi-writer-lazy-otherlinux64":                           ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eager-otherguest":         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eager-otherlinux64":       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-lazy-otherguest":          ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_persistent-multi-writer-eager-otherguest":            ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_persistent-multi-writer-eager-otherlinux64":          ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_persistent-multi-writer-lazy-otherguest":             ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-independent_persistent-multi-writer-lazy-otherlinux64":           ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eager-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eager-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-nonpersistent-multi-writer-lazy-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-nonpersistent-multi-writer-lazy-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-persistent-multi-writer-eager-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-persistent-multi-writer-eager-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-persistent-multi-writer-lazy-otherguest":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-persistent-multi-writer-lazy-otherlinux64":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-undoable-multi-writer-eager-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-undoable-multi-writer-eager-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-undoable-multi-writer-lazy-otherguest":                           ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-none-undoable-multi-writer-lazy-otherlinux64":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-append-multi-writer-eager-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-append-multi-writer-eager-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-append-multi-writer-lazy-otherguest":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-append-multi-writer-lazy-otherlinux64":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eager-otherguest":     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eager-otherlinux64":   ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-lazy-otherguest":      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64":    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-eager-otherguest":        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-eager-otherlinux64":      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-lazy-otherguest":         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_persistent-multi-writer-lazy-otherlinux64":       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-independent_persistent-none-eager-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_persistent-none-eager-otherlinux64":              ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_persistent-none-lazy-otherguest":                 ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_persistent-none-lazy-otherlinux64":               ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eager-otherguest":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eager-otherlinux64":               ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-lazy-otherguest":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-lazy-otherlinux64":                ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-persistent-multi-writer-eager-otherguest":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-persistent-multi-writer-eager-otherlinux64":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-persistent-multi-writer-lazy-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-persistent-multi-writer-lazy-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-persistent-none-eager-otherguest":                            ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-persistent-none-eager-otherlinux64":                          ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-persistent-none-lazy-otherguest":                             ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-persistent-none-lazy-otherlinux64":                           ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-undoable-multi-writer-eager-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-undoable-multi-writer-eager-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-undoable-multi-writer-lazy-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-physical-undoable-multi-writer-lazy-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-append-multi-writer-eager-otherguest":                         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-append-multi-writer-eager-otherlinux64":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-append-multi-writer-lazy-otherguest":                          ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-append-multi-writer-lazy-otherlinux64":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eager-otherguest":      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64":    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-lazy-otherguest":       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64":     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-eager-otherguest":         ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-eager-otherlinux64":       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-lazy-otherguest":          ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_persistent-multi-writer-lazy-otherlinux64":        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-independent_persistent-none-eager-otherguest":                 ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_persistent-none-eager-otherlinux64":               ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_persistent-none-lazy-otherguest":                  ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_persistent-none-lazy-otherlinux64":                ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eager-otherguest":                  ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eager-otherlinux64":                ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-lazy-otherguest":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-lazy-otherlinux64":                 ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-persistent-multi-writer-eager-otherguest":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-persistent-multi-writer-eager-otherlinux64":                   ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-persistent-multi-writer-lazy-otherguest":                      ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-persistent-multi-writer-lazy-otherlinux64":                    ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-persistent-none-eager-otherguest":                             ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-persistent-none-eager-otherlinux64":                           ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-persistent-none-lazy-otherguest":                              ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-persistent-none-lazy-otherlinux64":                            ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-undoable-multi-writer-eager-otherguest":                       ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-undoable-multi-writer-eager-otherlinux64":                     ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-undoable-multi-writer-lazy-otherguest":                        ErrIncompatibleBackingSharingVMFS,
	"scsi-pvscsi-virtual-undoable-multi-writer-lazy-otherlinux64":                      ErrIncompatibleBackingSharingVMFS,
}

// vsanUnsupportedCombinations contains configurations that fail only on VSAN
// datastores.
var vsanUnsupportedCombinations = map[string]error{
	"ide-ide-none-append-multi-writer-eager-otherguest":                                ErrLegacyDiskModes,
	"ide-ide-none-append-multi-writer-eager-otherlinux64":                              ErrLegacyDiskModes,
	"ide-ide-none-append-multi-writer-lazy-otherguest":                                 ErrLegacyDiskModes,
	"ide-ide-none-append-multi-writer-lazy-otherlinux64":                               ErrLegacyDiskModes,
	"ide-ide-none-independent_nonpersistent-multi-writer-eager-otherguest":             ErrMultiWriterRequiresThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-eager-otherlinux64":           ErrMultiWriterRequiresThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-lazy-otherguest":              ErrMultiWriterRequiresThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":            ErrMultiWriterRequiresThick,
	"ide-ide-none-nonpersistent-multi-writer-eager-otherguest":                         ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-multi-writer-eager-otherlinux64":                       ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-multi-writer-lazy-otherguest":                          ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-multi-writer-lazy-otherlinux64":                        ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-eager-otherguest":                              ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-eager-otherlinux64":                            ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-lazy-otherguest":                               ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-lazy-otherlinux64":                             ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-eager-otherguest":                              ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-eager-otherlinux64":                            ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-lazy-otherguest":                               ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-lazy-otherlinux64":                             ErrLegacyDiskModes,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eager-otherguest":           ErrMultiWriterRequiresThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eager-otherlinux64":         ErrMultiWriterRequiresThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-lazy-otherguest":            ErrMultiWriterRequiresThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":          ErrMultiWriterRequiresThick,
	"nvme-nvme-none-nonpersistent-multi-writer-eager-otherguest":                       ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-multi-writer-eager-otherlinux64":                     ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-multi-writer-lazy-otherguest":                        ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-multi-writer-lazy-otherlinux64":                      ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-eager-otherguest":                            ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-eager-otherlinux64":                          ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-lazy-otherguest":                             ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-lazy-otherlinux64":                           ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-eager-otherguest":                              ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-eager-otherlinux64":                            ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-lazy-otherguest":                               ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-lazy-otherlinux64":                             ErrLegacyDiskModes,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eager-otherguest":           ErrMultiWriterRequiresThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eager-otherlinux64":         ErrMultiWriterRequiresThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-lazy-otherguest":            ErrMultiWriterRequiresThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":          ErrMultiWriterRequiresThick,
	"sata-ahci-none-nonpersistent-multi-writer-eager-otherguest":                       ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-multi-writer-eager-otherlinux64":                     ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-multi-writer-lazy-otherguest":                        ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-multi-writer-lazy-otherlinux64":                      ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-eager-otherguest":                            ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-eager-otherlinux64":                          ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-lazy-otherguest":                             ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-lazy-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-eager-otherguest":                          ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-eager-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-lazy-otherguest":                           ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-lazy-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-otherguest":       ErrMultiWriterRequiresThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eager-otherlinux64":     ErrMultiWriterRequiresThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-otherguest":        ErrMultiWriterRequiresThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":      ErrMultiWriterRequiresThick,
	"scsi-buslogic-none-independent_persistent-multi-writer-eager-otherlinux64":        ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-multi-writer-lazy-otherlinux64":         ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-otherguest":                   ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-multi-writer-eager-otherlinux64":                 ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-otherguest":                    ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-multi-writer-lazy-otherlinux64":                  ErrLegacyDiskModes,
	"scsi-buslogic-none-persistent-multi-writer-eager-otherlinux64":                    ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-multi-writer-lazy-otherlinux64":                     ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-multi-writer-eager-otherguest":                        ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-multi-writer-eager-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-multi-writer-lazy-otherguest":                         ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-multi-writer-lazy-otherlinux64":                       ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-eager-otherguest":                      ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-eager-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-lazy-otherguest":                       ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-lazy-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-otherguest":   ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-otherguest":    ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64":  ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eager-otherlinux64":    ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-multi-writer-lazy-otherlinux64":     ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-otherguest":               ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eager-otherlinux64":             ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-otherguest":                ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-multi-writer-lazy-otherlinux64":              ErrLegacyDiskModes,
	"scsi-buslogic-physical-persistent-multi-writer-eager-otherlinux64":                ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-multi-writer-lazy-otherlinux64":                 ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-undoable-multi-writer-eager-otherguest":                    ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-multi-writer-eager-otherlinux64":                  ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-otherguest":                     ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-multi-writer-lazy-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-eager-otherguest":                       ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-eager-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-lazy-otherguest":                        ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-lazy-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-otherguest":    ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64":  ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-otherguest":     ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64":   ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eager-otherlinux64":     ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-lazy-otherlinux64":      ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-otherguest":                ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eager-otherlinux64":              ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-otherguest":                 ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-lazy-otherlinux64":               ErrLegacyDiskModes,
	"scsi-buslogic-virtual-persistent-multi-writer-eager-otherlinux64":                 ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-multi-writer-lazy-otherlinux64":                  ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-otherguest":                     ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-multi-writer-eager-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-otherguest":                      ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-multi-writer-lazy-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-eager-otherguest":                          ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-eager-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-lazy-otherguest":                           ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-lazy-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-otherguest":       ErrMultiWriterRequiresThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eager-otherlinux64":     ErrMultiWriterRequiresThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-otherguest":        ErrMultiWriterRequiresThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":      ErrMultiWriterRequiresThick,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-otherguest":                   ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eager-otherlinux64":                 ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-otherguest":                    ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-multi-writer-lazy-otherlinux64":                  ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-eager-otherguest":                        ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-eager-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-otherguest":                         ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-lazy-otherlinux64":                       ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-eager-otherguest":                      ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-eager-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-lazy-otherguest":                       ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-lazy-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-otherguest":   ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eager-otherlinux64": ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-otherguest":    ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64":  ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-otherguest":               ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eager-otherlinux64":             ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-otherguest":                ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-lazy-otherlinux64":              ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-otherguest":                    ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-eager-otherlinux64":                  ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-otherguest":                     ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-lazy-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-eager-otherguest":                       ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-eager-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-otherguest":                        ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-lazy-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-otherguest":    ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64":  ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-otherguest":     ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64":   ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-otherguest":                ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eager-otherlinux64":              ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-otherguest":                 ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-lazy-otherlinux64":               ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-otherguest":                     ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-eager-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-otherguest":                      ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-lazy-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-eager-otherguest":                            ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-eager-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-lazy-otherguest":                             ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-lazy-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eager-otherguest":         ErrMultiWriterRequiresThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eager-otherlinux64":       ErrMultiWriterRequiresThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-lazy-otherguest":          ErrMultiWriterRequiresThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-lazy-otherlinux64":        ErrMultiWriterRequiresThick,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eager-otherguest":                     ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eager-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-multi-writer-lazy-otherguest":                      ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-multi-writer-lazy-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-eager-otherguest":                          ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-eager-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-lazy-otherguest":                           ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-lazy-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-eager-otherguest":                        ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-eager-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-lazy-otherguest":                         ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-lazy-otherlinux64":                       ErrLegacyDiskModes,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eager-otherguest":     ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eager-otherlinux64":   ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-lazy-otherguest":      ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-lazy-otherlinux64":    ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eager-otherguest":                 ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eager-otherlinux64":               ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-lazy-otherguest":                  ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-lazy-otherlinux64":                ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-eager-otherguest":                      ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-eager-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-lazy-otherguest":                       ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-lazy-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-eager-otherguest":                         ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-eager-otherlinux64":                       ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-lazy-otherguest":                          ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-lazy-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eager-otherguest":      ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eager-otherlinux64":    ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-lazy-otherguest":       ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-lazy-otherlinux64":     ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eager-otherguest":                  ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eager-otherlinux64":                ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-lazy-otherguest":                   ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-lazy-otherlinux64":                 ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-eager-otherguest":                       ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-eager-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-lazy-otherguest":                        ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-lazy-otherlinux64":                      ErrLegacyDiskModes,
}

// allDatastoreUnsupportedCombinations contains configurations that fail on
// all datastore types.
var allDatastoreUnsupportedCombinations = map[string]error{
	"ide-ide-none-append-multi-writer-eagerzero-otherguest":                                ErrLegacyDiskModes,
	"ide-ide-none-append-multi-writer-eagerzero-otherlinux64":                              ErrLegacyDiskModes,
	"ide-ide-none-append-none-eager-otherguest":                                            ErrLegacyDiskModes,
	"ide-ide-none-append-none-eager-otherlinux64":                                          ErrLegacyDiskModes,
	"ide-ide-none-append-none-eagerzero-otherguest":                                        ErrLegacyDiskModes,
	"ide-ide-none-append-none-eagerzero-otherlinux64":                                      ErrLegacyDiskModes,
	"ide-ide-none-append-none-lazy-otherguest":                                             ErrLegacyDiskModes,
	"ide-ide-none-append-none-lazy-otherlinux64":                                           ErrLegacyDiskModes,
	"ide-ide-none-independent_nonpersistent-multi-writer-eagerzero-otherguest":             ErrMultiWriterRequiresThick,
	"ide-ide-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":           ErrMultiWriterRequiresThick,
	"ide-ide-none-nonpersistent-multi-writer-eagerzero-otherguest":                         ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-multi-writer-eagerzero-otherlinux64":                       ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eager-otherguest":                                     ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eager-otherlinux64":                                   ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eagerzero-otherguest":                                 ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-eagerzero-otherlinux64":                               ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-lazy-otherguest":                                      ErrLegacyDiskModes,
	"ide-ide-none-nonpersistent-none-lazy-otherlinux64":                                    ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-eagerzero-otherguest":                              ErrLegacyDiskModes,
	"ide-ide-none-undoable-multi-writer-eagerzero-otherlinux64":                            ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eager-otherguest":                                          ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eager-otherlinux64":                                        ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eagerzero-otherguest":                                      ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-eagerzero-otherlinux64":                                    ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-lazy-otherguest":                                           ErrLegacyDiskModes,
	"ide-ide-none-undoable-none-lazy-otherlinux64":                                         ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-eagerzero-otherguest":                              ErrLegacyDiskModes,
	"nvme-nvme-none-append-multi-writer-eagerzero-otherlinux64":                            ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eager-otherguest":                                          ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eager-otherlinux64":                                        ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eagerzero-otherguest":                                      ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-eagerzero-otherlinux64":                                    ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-lazy-otherguest":                                           ErrLegacyDiskModes,
	"nvme-nvme-none-append-none-lazy-otherlinux64":                                         ErrLegacyDiskModes,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eagerzero-otherguest":           ErrMultiWriterRequiresThick,
	"nvme-nvme-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":         ErrMultiWriterRequiresThick,
	"nvme-nvme-none-nonpersistent-multi-writer-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-multi-writer-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eager-otherguest":                                   ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eager-otherlinux64":                                 ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eagerzero-otherguest":                               ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-eagerzero-otherlinux64":                             ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-lazy-otherguest":                                    ErrLegacyDiskModes,
	"nvme-nvme-none-nonpersistent-none-lazy-otherlinux64":                                  ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-eagerzero-otherguest":                            ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-multi-writer-eagerzero-otherlinux64":                          ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eager-otherguest":                                        ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eager-otherlinux64":                                      ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eagerzero-otherguest":                                    ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-eagerzero-otherlinux64":                                  ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-lazy-otherguest":                                         ErrLegacyDiskModes,
	"nvme-nvme-none-undoable-none-lazy-otherlinux64":                                       ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-eagerzero-otherguest":                              ErrLegacyDiskModes,
	"sata-ahci-none-append-multi-writer-eagerzero-otherlinux64":                            ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eager-otherguest":                                          ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eager-otherlinux64":                                        ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eagerzero-otherguest":                                      ErrLegacyDiskModes,
	"sata-ahci-none-append-none-eagerzero-otherlinux64":                                    ErrLegacyDiskModes,
	"sata-ahci-none-append-none-lazy-otherguest":                                           ErrLegacyDiskModes,
	"sata-ahci-none-append-none-lazy-otherlinux64":                                         ErrLegacyDiskModes,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eagerzero-otherguest":           ErrMultiWriterRequiresThick,
	"sata-ahci-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":         ErrMultiWriterRequiresThick,
	"sata-ahci-none-nonpersistent-multi-writer-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-multi-writer-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eager-otherguest":                                   ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eager-otherlinux64":                                 ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eagerzero-otherguest":                               ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-eagerzero-otherlinux64":                             ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-lazy-otherguest":                                    ErrLegacyDiskModes,
	"sata-ahci-none-nonpersistent-none-lazy-otherlinux64":                                  ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-eagerzero-otherguest":                            ErrLegacyDiskModes,
	"sata-ahci-none-undoable-multi-writer-eagerzero-otherlinux64":                          ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eager-otherguest":                                        ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eager-otherlinux64":                                      ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eagerzero-otherguest":                                    ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-eagerzero-otherlinux64":                                  ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-lazy-otherguest":                                         ErrLegacyDiskModes,
	"sata-ahci-none-undoable-none-lazy-otherlinux64":                                       ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-eagerzero-otherguest":                          ErrLegacyDiskModes,
	"scsi-buslogic-none-append-multi-writer-eagerzero-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eager-otherguest":                                      ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eager-otherlinux64":                                    ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eagerzero-otherguest":                                  ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-eagerzero-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-lazy-otherguest":                                       ErrLegacyDiskModes,
	"scsi-buslogic-none-append-none-lazy-otherlinux64":                                     ErrLegacyDiskModes,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eagerzero-otherguest":       ErrMultiWriterRequiresThick,
	"scsi-buslogic-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":     ErrMultiWriterRequiresThick,
	"scsi-buslogic-none-independent_nonpersistent-none-eager-otherlinux64":                 ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-none-eagerzero-otherlinux64":             ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_nonpersistent-none-lazy-otherlinux64":                  ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-multi-writer-eagerzero-otherlinux64":        ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-eager-otherlinux64":                    ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-eagerzero-otherlinux64":                ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-independent_persistent-none-lazy-otherlinux64":                     ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-nonpersistent-multi-writer-eagerzero-otherguest":                   ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-multi-writer-eagerzero-otherlinux64":                 ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eager-otherguest":                               ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eager-otherlinux64":                             ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eagerzero-otherguest":                           ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-eagerzero-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-lazy-otherguest":                                ErrLegacyDiskModes,
	"scsi-buslogic-none-nonpersistent-none-lazy-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-buslogic-none-persistent-multi-writer-eagerzero-otherlinux64":                    ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-eager-otherlinux64":                                ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-eagerzero-otherlinux64":                            ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-persistent-none-lazy-otherlinux64":                                 ErrBusLogicNot64BitGuest,
	"scsi-buslogic-none-undoable-multi-writer-eagerzero-otherguest":                        ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-multi-writer-eagerzero-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eager-otherguest":                                    ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eager-otherlinux64":                                  ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eagerzero-otherguest":                                ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-eagerzero-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-lazy-otherguest":                                     ErrLegacyDiskModes,
	"scsi-buslogic-none-undoable-none-lazy-otherlinux64":                                   ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-eagerzero-otherguest":                      ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-multi-writer-eagerzero-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eager-otherguest":                                  ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eager-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eagerzero-otherguest":                              ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-eagerzero-otherlinux64":                            ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-lazy-otherguest":                                   ErrLegacyDiskModes,
	"scsi-buslogic-physical-append-none-lazy-otherlinux64":                                 ErrLegacyDiskModes,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest":   ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eager-otherguest":               ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eager-otherlinux64":             ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eagerzero-otherguest":           ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-eagerzero-otherlinux64":         ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-lazy-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_nonpersistent-none-lazy-otherlinux64":              ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_persistent-multi-writer-eagerzero-otherlinux64":    ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-none-eager-otherlinux64":                ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-independent_persistent-none-eagerzero-otherlinux64":            ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-independent_persistent-none-lazy-otherlinux64":                 ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eagerzero-otherguest":               ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-multi-writer-eagerzero-otherlinux64":             ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eager-otherguest":                           ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eager-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-lazy-otherguest":                            ErrLegacyDiskModes,
	"scsi-buslogic-physical-nonpersistent-none-lazy-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-buslogic-physical-persistent-multi-writer-eagerzero-otherlinux64":                ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-none-eager-otherlinux64":                            ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-persistent-none-eagerzero-otherlinux64":                        ErrBusLogicNot64BitGuest,
	"scsi-buslogic-physical-persistent-none-lazy-otherlinux64":                             ErrMultiWriterRequiresThick,
	"scsi-buslogic-physical-undoable-multi-writer-eagerzero-otherguest":                    ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-multi-writer-eagerzero-otherlinux64":                  ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eager-otherguest":                                ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eager-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eagerzero-otherguest":                            ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-eagerzero-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-lazy-otherguest":                                 ErrLegacyDiskModes,
	"scsi-buslogic-physical-undoable-none-lazy-otherlinux64":                               ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-multi-writer-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eager-otherguest":                                   ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eager-otherlinux64":                                 ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eagerzero-otherguest":                               ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-eagerzero-otherlinux64":                             ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-lazy-otherguest":                                    ErrLegacyDiskModes,
	"scsi-buslogic-virtual-append-none-lazy-otherlinux64":                                  ErrLegacyDiskModes,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest":    ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":  ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eager-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eager-otherlinux64":              ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eagerzero-otherguest":            ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-none-eagerzero-otherlinux64":          ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-none-lazy-otherguest":                 ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_nonpersistent-none-lazy-otherlinux64":               ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_persistent-multi-writer-eagerzero-otherlinux64":     ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-none-eager-otherlinux64":                 ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-independent_persistent-none-eagerzero-otherlinux64":             ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-independent_persistent-none-lazy-otherlinux64":                  ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eagerzero-otherguest":                ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64":              ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eager-otherguest":                            ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eager-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eagerzero-otherguest":                        ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-eagerzero-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-lazy-otherguest":                             ErrLegacyDiskModes,
	"scsi-buslogic-virtual-nonpersistent-none-lazy-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-buslogic-virtual-persistent-multi-writer-eagerzero-otherlinux64":                 ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-none-eager-otherlinux64":                             ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-persistent-none-eagerzero-otherlinux64":                         ErrBusLogicNot64BitGuest,
	"scsi-buslogic-virtual-persistent-none-lazy-otherlinux64":                              ErrMultiWriterRequiresThick,
	"scsi-buslogic-virtual-undoable-multi-writer-eagerzero-otherguest":                     ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-multi-writer-eagerzero-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eager-otherguest":                                 ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eager-otherlinux64":                               ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eagerzero-otherguest":                             ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-eagerzero-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-lazy-otherguest":                                  ErrLegacyDiskModes,
	"scsi-buslogic-virtual-undoable-none-lazy-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-eagerzero-otherguest":                          ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-multi-writer-eagerzero-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eager-otherguest":                                      ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eager-otherlinux64":                                    ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eagerzero-otherguest":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-eagerzero-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-lazy-otherguest":                                       ErrLegacyDiskModes,
	"scsi-lsilogic-none-append-none-lazy-otherlinux64":                                     ErrLegacyDiskModes,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eagerzero-otherguest":       ErrMultiWriterRequiresThick,
	"scsi-lsilogic-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":     ErrMultiWriterRequiresThick,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eagerzero-otherguest":                   ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-multi-writer-eagerzero-otherlinux64":                 ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eager-otherguest":                               ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eager-otherlinux64":                             ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eagerzero-otherguest":                           ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-eagerzero-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-lazy-otherguest":                                ErrLegacyDiskModes,
	"scsi-lsilogic-none-nonpersistent-none-lazy-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-eagerzero-otherguest":                        ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-multi-writer-eagerzero-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eager-otherguest":                                    ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eager-otherlinux64":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eagerzero-otherguest":                                ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-eagerzero-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-lazy-otherguest":                                     ErrLegacyDiskModes,
	"scsi-lsilogic-none-undoable-none-lazy-otherlinux64":                                   ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-eagerzero-otherguest":                      ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-multi-writer-eagerzero-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eager-otherguest":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eager-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eagerzero-otherguest":                              ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-eagerzero-otherlinux64":                            ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-lazy-otherguest":                                   ErrLegacyDiskModes,
	"scsi-lsilogic-physical-append-none-lazy-otherlinux64":                                 ErrLegacyDiskModes,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest":   ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64": ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eager-otherguest":               ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eager-otherlinux64":             ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eagerzero-otherguest":           ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-eagerzero-otherlinux64":         ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-lazy-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-independent_nonpersistent-none-lazy-otherlinux64":              ErrMultiWriterRequiresThick,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eagerzero-otherguest":               ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-multi-writer-eagerzero-otherlinux64":             ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eager-otherguest":                           ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eager-otherlinux64":                         ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-lazy-otherguest":                            ErrLegacyDiskModes,
	"scsi-lsilogic-physical-nonpersistent-none-lazy-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-eagerzero-otherguest":                    ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-multi-writer-eagerzero-otherlinux64":                  ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eager-otherguest":                                ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eager-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eagerzero-otherguest":                            ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-eagerzero-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-lazy-otherguest":                                 ErrLegacyDiskModes,
	"scsi-lsilogic-physical-undoable-none-lazy-otherlinux64":                               ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-multi-writer":                                           ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-eager":                                             ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-eagerzero":                                         ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-append-none-lazy":                                              ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-independent_nonpersistent-multi-writer":                        ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-none-nonpersistent-multi-writer":                                    ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-eager":                                      ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-eagerzero":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-nonpersistent-none-lazy":                                       ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-multi-writer":                                         ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-eager":                                           ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-eagerzero":                                       ErrLegacyDiskModes,
	"scsi-lsilogic-sas-none-undoable-none-lazy":                                            ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-multi-writer":                                       ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-eager":                                         ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-eagerzero":                                     ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-append-none-lazy":                                          ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-multi-writer":                    ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-eager":                      ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-eagerzero":                  ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-independent_nonpersistent-none-lazy":                       ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-physical-nonpersistent-multi-writer":                                ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-eager":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-eagerzero":                              ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-nonpersistent-none-lazy":                                   ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-multi-writer":                                     ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-eager":                                       ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-eagerzero":                                   ErrLegacyDiskModes,
	"scsi-lsilogic-sas-physical-undoable-none-lazy":                                        ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-multi-writer":                                        ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-eager":                                          ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-eagerzero":                                      ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-append-none-lazy":                                           ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-multi-writer":                     ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-eager":                       ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-eagerzero":                   ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-independent_nonpersistent-none-lazy":                        ErrMultiWriterRequiresThick,
	"scsi-lsilogic-sas-virtual-nonpersistent-multi-writer":                                 ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-eager":                                   ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-eagerzero":                               ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-nonpersistent-none-lazy":                                    ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-multi-writer":                                      ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-eager":                                        ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-eagerzero":                                    ErrLegacyDiskModes,
	"scsi-lsilogic-sas-virtual-undoable-none-lazy":                                         ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-multi-writer-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eager-otherguest":                                   ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eager-otherlinux64":                                 ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eagerzero-otherguest":                               ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-eagerzero-otherlinux64":                             ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-lazy-otherguest":                                    ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-append-none-lazy-otherlinux64":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest":    ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":  ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eager-otherguest":                ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eager-otherlinux64":              ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eagerzero-otherguest":            ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-eagerzero-otherlinux64":          ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-lazy-otherguest":                 ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-independent_nonpersistent-none-lazy-otherlinux64":               ErrMultiWriterRequiresThick,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eagerzero-otherguest":                ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64":              ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eager-otherguest":                            ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eager-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eagerzero-otherguest":                        ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-eagerzero-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-lazy-otherguest":                             ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-nonpersistent-none-lazy-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-eagerzero-otherguest":                     ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-multi-writer-eagerzero-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eager-otherguest":                                 ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eager-otherlinux64":                               ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eagerzero-otherguest":                             ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-eagerzero-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-lazy-otherguest":                                  ErrLegacyDiskModes,
	"scsi-lsilogic-virtual-undoable-none-lazy-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-eagerzero-otherguest":                            ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-multi-writer-eagerzero-otherlinux64":                          ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eager-otherguest":                                        ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eager-otherlinux64":                                      ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eagerzero-otherguest":                                    ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-eagerzero-otherlinux64":                                  ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-lazy-otherguest":                                         ErrLegacyDiskModes,
	"scsi-pvscsi-none-append-none-lazy-otherlinux64":                                       ErrLegacyDiskModes,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eagerzero-otherguest":         ErrMultiWriterRequiresThick,
	"scsi-pvscsi-none-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":       ErrMultiWriterRequiresThick,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eagerzero-otherguest":                     ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-multi-writer-eagerzero-otherlinux64":                   ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eager-otherguest":                                 ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eager-otherlinux64":                               ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eagerzero-otherguest":                             ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-eagerzero-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-lazy-otherguest":                                  ErrLegacyDiskModes,
	"scsi-pvscsi-none-nonpersistent-none-lazy-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-eagerzero-otherguest":                          ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-multi-writer-eagerzero-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eager-otherguest":                                      ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eager-otherlinux64":                                    ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eagerzero-otherguest":                                  ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-eagerzero-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-lazy-otherguest":                                       ErrLegacyDiskModes,
	"scsi-pvscsi-none-undoable-none-lazy-otherlinux64":                                     ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-eagerzero-otherguest":                        ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-multi-writer-eagerzero-otherlinux64":                      ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eager-otherguest":                                    ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eager-otherlinux64":                                  ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eagerzero-otherguest":                                ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-eagerzero-otherlinux64":                              ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-lazy-otherguest":                                     ErrLegacyDiskModes,
	"scsi-pvscsi-physical-append-none-lazy-otherlinux64":                                   ErrLegacyDiskModes,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eagerzero-otherguest":     ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":   ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eager-otherguest":                 ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eager-otherlinux64":               ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eagerzero-otherguest":             ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-eagerzero-otherlinux64":           ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-lazy-otherguest":                  ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-independent_nonpersistent-none-lazy-otherlinux64":                ErrMultiWriterRequiresThick,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eagerzero-otherguest":                 ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-multi-writer-eagerzero-otherlinux64":               ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eager-otherguest":                             ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eager-otherlinux64":                           ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eagerzero-otherguest":                         ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-eagerzero-otherlinux64":                       ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-lazy-otherguest":                              ErrLegacyDiskModes,
	"scsi-pvscsi-physical-nonpersistent-none-lazy-otherlinux64":                            ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-eagerzero-otherguest":                      ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-multi-writer-eagerzero-otherlinux64":                    ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eager-otherguest":                                  ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eager-otherlinux64":                                ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eagerzero-otherguest":                              ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-eagerzero-otherlinux64":                            ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-lazy-otherguest":                                   ErrLegacyDiskModes,
	"scsi-pvscsi-physical-undoable-none-lazy-otherlinux64":                                 ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-eagerzero-otherguest":                         ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-multi-writer-eagerzero-otherlinux64":                       ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eager-otherguest":                                     ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eager-otherlinux64":                                   ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eagerzero-otherguest":                                 ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-eagerzero-otherlinux64":                               ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-lazy-otherguest":                                      ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-append-none-lazy-otherlinux64":                                    ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eagerzero-otherguest":      ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-multi-writer-eagerzero-otherlinux64":    ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eager-otherguest":                  ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eager-otherlinux64":                ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eagerzero-otherguest":              ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-eagerzero-otherlinux64":            ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-lazy-otherguest":                   ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-independent_nonpersistent-none-lazy-otherlinux64":                 ErrMultiWriterRequiresThick,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eagerzero-otherguest":                  ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-multi-writer-eagerzero-otherlinux64":                ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eager-otherguest":                              ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eager-otherlinux64":                            ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eagerzero-otherguest":                          ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-eagerzero-otherlinux64":                        ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-lazy-otherguest":                               ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-nonpersistent-none-lazy-otherlinux64":                             ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-eagerzero-otherguest":                       ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-multi-writer-eagerzero-otherlinux64":                     ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eager-otherguest":                                   ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eager-otherlinux64":                                 ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eagerzero-otherguest":                               ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-eagerzero-otherlinux64":                             ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-lazy-otherguest":                                    ErrLegacyDiskModes,
	"scsi-pvscsi-virtual-undoable-none-lazy-otherlinux64":                                  ErrLegacyDiskModes,
}
