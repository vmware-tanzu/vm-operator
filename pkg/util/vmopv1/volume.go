// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
)

// These constants define the maximum number of devices that can be attached
// to each controller type. Unit number 7 is reserved for SCSI controllers
// as it's used internally by the SCSI controller and cannot be assigned
// to user devices.
const (
	IDEControllerMaxUnitNumber   = 1
	NVMeControllerMaxUnitNumber  = 63
	SATAControllerMaxUnitNumber  = 29
	SCSIControllerMaxUnitNumber  = 15
	SCSIParaVirtualMaxUnitNumber = 63
	SCSIReservedUnitNumber       = 7
)

// Error message constants.
const (
	// General validation errors.
	ErrControllerNotFound   = "unable to find a controller with type %q and bus number %d"
	ErrHardwareOutOfSync    = "vm.status.hardware is out of sync with batch volume status"
	ErrUnitNumberNegative   = "unit number cannot be negative"
	ErrVolumeDuplicatedUnit = "volume %q contains a duplicated unit number"

	// Application type validation errors.
	ErrControllerMustBeSCSI                = "controller must be SCSI with %q applicationType"
	ErrDiskModeMustBeIndependentPersistent = "diskMode must be IndependentPersistent with %q applicationType"
	ErrPVCSharingModeMustBeMultiWriter     = "PVC sharingMode must be MultiWriter with OracleRAC applicationType"
	ErrControllerSharingModeMustBeNone     = "controller sharingMode must be None with OracleRAC applicationType"
	ErrPVCSharingModeMustBeNone            = "PVC sharingMode must be None with MicrosoftWSFC applicationType"
	ErrControllerSharingModeMustBePhysical = "controller sharingMode must be Physical with MicrosoftWSFC applicationType"

	// Unit number validation errors.
	ErrIDEControllerUnitNumberTooHigh   = "IDE controller unit number cannot be greater than %d"
	ErrNVMEControllerUnitNumberTooHigh  = "NVME controller unit number cannot be greater than %d"
	ErrSATAControllerUnitNumberTooHigh  = "SATA controller unit number cannot be greater than %d"
	ErrSCSIParaVirtualUnitNumberTooHigh = "SCSI ParaVirtual controller unit number cannot be greater than %d"
	ErrSCSIControllerUnitNumberTooHigh  = "SCSI %s controller unit number cannot be greater than %d"
	ErrSCSIControllerUnitNumberReserved = "SCSI controller unit number %d is reserved"
)

// GetVirtualDiskFileNameAndUUID extracts the file name and disk UUID from a
// VirtualDisk device. Returns empty strings for unsupported backing types or
// when UUID is not available.
func GetVirtualDiskFileNameAndUUID(vd *vimtypes.VirtualDisk) (
	fileName string, diskUUID string) {
	if vd == nil {
		return "", ""
	}
	switch tb := vd.Backing.(type) {
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		fileName = tb.FileName
		diskUUID = tb.Uuid
	case *vimtypes.VirtualDiskSparseVer1BackingInfo:
		fileName = tb.FileName
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		fileName = tb.FileName
		diskUUID = tb.Uuid
	case *vimtypes.VirtualDiskFlatVer1BackingInfo:
		fileName = tb.FileName
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		fileName = tb.FileName
		diskUUID = tb.Uuid
	case *vimtypes.VirtualDiskLocalPMemBackingInfo:
		fileName = tb.FileName
		diskUUID = tb.Uuid
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		fileName = tb.FileName
		diskUUID = tb.Uuid
	case *vimtypes.VirtualDiskRawDiskVer2BackingInfo:
		fileName = tb.DescriptorFileName
		diskUUID = tb.Uuid
	case *vimtypes.VirtualDiskPartitionedRawDiskVer2BackingInfo:
		fileName = tb.DescriptorFileName
		diskUUID = tb.Uuid
	}
	return fileName, diskUUID
}

// fileNameToName converts a vSphere datastore path to a simple name.
// For example, "[datastore1] vm/my-disk.vmdk" becomes "my-disk".
func fileNameToName(fileName string) (name string) {
	var diskPath object.DatastorePath
	if !diskPath.FromString(fileName) {
		return
	}

	dp := diskPath.Path
	return strings.TrimSuffix(path.Base(dp), path.Ext(dp))
}

// GetVirtualCdromName returns a human-readable name for a VirtualCdrom device.
// For physical devices, it returns the device name. For ISO files, it returns
// the filename without extension.
func GetVirtualCdromName(vd *vimtypes.VirtualCdrom) (name string) {
	if vd == nil {
		return ""
	}
	switch tb := vd.Backing.(type) {
	case *vimtypes.VirtualCdromAtapiBackingInfo:
		name = tb.DeviceName
	case *vimtypes.VirtualCdromPassthroughBackingInfo:
		name = tb.DeviceName
	case *vimtypes.VirtualCdromIsoBackingInfo:
		fileName := tb.FileName
		name = fileNameToName(fileName)
	case *vimtypes.VirtualCdromRemoteAtapiBackingInfo:
		name = tb.DeviceName
	case *vimtypes.VirtualCdromRemotePassthroughBackingInfo:
		name = tb.DeviceName
	}
	return
}

// GetVirtualDiskNameAndUUID returns a human-readable name and UUID for a
// VirtualDisk device. It extracts the base filename without extension from
// the disk's backing store.
func GetVirtualDiskNameAndUUID(vd *vimtypes.VirtualDisk) (string, string) {
	fileName, uuid := GetVirtualDiskFileNameAndUUID(vd)
	name := fileNameToName(fileName)
	return name, uuid
}

// ControllerWithDevices interface for controllers that have devices.
type ControllerWithDevices interface {
	GetDevices() []vmopv1.VirtualDeviceStatus
	GetBusNumber() int32
}

// SortControllerWithDevices is a generic helper that converts controller
// maps to sorted slices.
func SortControllerWithDevices[T ControllerWithDevices](
	controllerMap map[int32]*T) []T {
	controllers := make([]T, 0, len(controllerMap))
	for _, ctrl := range controllerMap {
		// Sort devices by type and unit number within each controller
		devices := (*ctrl).GetDevices()
		slices.SortFunc(devices, func(a, b vmopv1.VirtualDeviceStatus) int {
			if a.Type != b.Type {
				return strings.Compare(string(a.Type), string(b.Type))
			}
			return int(a.UnitNumber - b.UnitNumber)
		})
		controllers = append(controllers, *ctrl)
	}

	// Sort controllers by BusNumber
	slices.SortFunc(controllers, func(a, b T) int {
		return int(a.GetBusNumber() - b.GetBusNumber())
	})

	return controllers
}

// VolumeKey uniquely identifies a volume using both VolumeName and PVCName.
// This dual-key approach handles cases where CNS batchAttachment status may
// not be synchronized with VM.spec.volumes.
type VolumeKey struct {
	VolumeName string
	PVCName    string
}

type CtrlKey struct {
	CtrlType  vmopv1.VirtualControllerType
	BusNumber int32
}

type DeviceInfo struct {
	CtrlKey
	UnitNumber int32
}

type CtrlInfo struct {
	ControllerKey int32
	SharingMode   vmopv1.VirtualControllerSharingMode
	SCSIType      vmopv1.SCSIControllerType
}

// getExistingVolumeToDiskUUIDMap constructs a volume name and pvc name to
// underlying attached disk UUID map based on current batch volume's status.
func getExistingVolumeToDiskUUIDMap(
	volStatus []cnsv1alpha1.VolumeStatus) map[VolumeKey]string {
	m := make(map[VolumeKey]string)
	for _, s := range volStatus {
		p := s.PersistentVolumeClaim
		if s.Name != "" && p.ClaimName != "" && p.Attached && p.Diskuuid != "" {
			m[VolumeKey{VolumeName: s.Name, PVCName: p.ClaimName}] = p.Diskuuid
		}
	}
	return m
}

// getExistingUnmanagedDiskUUIDSet returns a set with all the underlying disk
// UUIDs for the unmanaged volumes.
func getExistingUnmanagedDiskUUIDSet(
	volStatus []vmopv1.VirtualMachineVolumeStatus) sets.Set[string] {
	s := sets.Set[string]{}
	for _, vs := range volStatus {
		if vs.Type == vmopv1.VolumeTypeClassic &&
			vs.Attached && vs.DiskUUID != "" {
			s.Insert(vs.DiskUUID)
		}
	}
	return s
}

// processControllerDevices processes devices for a given controller and populates
// the controller info map and device info map. The devices parameter contains
// the list of devices already attached to the controller from
// vm.status.hardware.
func processControllerDevices(
	ctrlType vmopv1.VirtualControllerType,
	busNumber int32,
	deviceKey int32,
	sharingMode vmopv1.VirtualControllerSharingMode,
	scsiType vmopv1.SCSIControllerType,
	devices []vmopv1.VirtualDeviceStatus,
	ctrlInfoMap map[CtrlKey]CtrlInfo,
	vmDiskUUIDToDeviceInfoMap map[string]DeviceInfo) {

	ctrlKey := CtrlKey{CtrlType: ctrlType, BusNumber: busNumber}
	ctrlInfo := CtrlInfo{ControllerKey: deviceKey}

	// Set sharing mode and SCSI type for applicable controllers
	if ctrlType == vmopv1.VirtualControllerTypeNVME ||
		ctrlType == vmopv1.VirtualControllerTypeSCSI {
		ctrlInfo.SharingMode = sharingMode
	}
	if ctrlType == vmopv1.VirtualControllerTypeSCSI {
		ctrlInfo.SCSIType = scsiType
	}

	ctrlInfoMap[ctrlKey] = ctrlInfo

	for _, d := range devices {
		if d.DiskUUID != "" {
			vmDiskUUIDToDeviceInfoMap[d.DiskUUID] = DeviceInfo{
				CtrlKey:    ctrlKey,
				UnitNumber: d.UnitNumber,
			}
		}
	}
}

// GetExistingControllersAndDevices extracts controller and device information
// from the VM's hardware status. Returns a map of controller information keyed
// by controller type and bus number, and a map of device information keyed by
// disk UUID for devices with valid UUIDs. Controller values contain device key,
// sharing mode, and SCSI type details, while device values contain controller
// key, unit number, and device type.
func GetExistingControllersAndDevices(
	hwStatus *vmopv1.VirtualMachineHardwareStatus) (
	map[CtrlKey]CtrlInfo, map[string]DeviceInfo) {
	if hwStatus == nil {
		return nil, nil
	}

	ctrlInfoMap := make(map[CtrlKey]CtrlInfo)
	vmDiskUUIDToDeviceInfoMap := make(map[string]DeviceInfo)

	// Process IDE controllers
	for _, c := range hwStatus.IDEControllers {
		processControllerDevices(
			vmopv1.VirtualControllerTypeIDE,
			c.BusNumber,
			c.DeviceKey,
			"", // IDE doesn't have sharing mode
			"", // IDE doesn't have SCSI type
			c.Devices,
			ctrlInfoMap,
			vmDiskUUIDToDeviceInfoMap)
	}

	// Process NVME controllers
	for _, c := range hwStatus.NVMEControllers {
		processControllerDevices(
			vmopv1.VirtualControllerTypeNVME,
			c.BusNumber,
			c.DeviceKey,
			c.SharingMode,
			"", // NVME doesn't have SCSI type
			c.Devices,
			ctrlInfoMap,
			vmDiskUUIDToDeviceInfoMap)
	}

	// Process SATA controllers
	for _, c := range hwStatus.SATAControllers {
		processControllerDevices(
			vmopv1.VirtualControllerTypeSATA,
			c.BusNumber,
			c.DeviceKey,
			"", // SATA doesn't have sharing mode
			"", // SATA doesn't have SCSI type
			c.Devices,
			ctrlInfoMap,
			vmDiskUUIDToDeviceInfoMap)
	}

	// Process SCSI controllers
	for _, c := range hwStatus.SCSIControllers {
		processControllerDevices(
			vmopv1.VirtualControllerTypeSCSI,
			c.BusNumber,
			c.DeviceKey,
			c.SharingMode,
			c.Type,
			c.Devices,
			ctrlInfoMap,
			vmDiskUUIDToDeviceInfoMap)
	}
	return ctrlInfoMap, vmDiskUUIDToDeviceInfoMap
}

// GetVolumeValidationInfo constructs controller and device information from the
// current VM hardware status and volume mappings for volume validation.
// It processes all controller types (IDE, NVME, SATA, SCSI) and builds maps
// containing controller details, managed device info, and unmanaged disk info.
func GetVolumeValidationInfo(
	existingAttachmentStatus []cnsv1alpha1.VolumeStatus,
	volStatus []vmopv1.VirtualMachineVolumeStatus,
	hwStatus *vmopv1.VirtualMachineHardwareStatus) (
	map[CtrlKey]CtrlInfo,
	map[VolumeKey]DeviceInfo,
	sets.Set[DeviceInfo],
	error) {
	if hwStatus == nil {
		return nil, nil, nil, nil
	}
	cnsVolumeToDiskUUIDMap := getExistingVolumeToDiskUUIDMap(
		existingAttachmentStatus)
	unmanagedDiskUUIDSet := getExistingUnmanagedDiskUUIDSet(
		volStatus)
	ctrlInfoMap, vmDiskUUIDToDeviceInfoMap := GetExistingControllersAndDevices(
		hwStatus)

	managedDeviceInfoMap := make(map[VolumeKey]DeviceInfo)
	for volumeKey, diskUUID := range cnsVolumeToDiskUUIDMap {
		deviceInfo, ok := vmDiskUUIDToDeviceInfoMap[diskUUID]
		if !ok {
			// Device out of sync: VM status reporting paused, has not been
			// updated yet after spec reconciliation, or CNS batchAttachment
			// status has not been updated by its operator.
			return nil, nil, nil, errors.New(ErrHardwareOutOfSync)
		}
		managedDeviceInfoMap[volumeKey] = deviceInfo
	}

	unmanagedDiskDeviceInfoSet := sets.Set[DeviceInfo]{}
	for diskUUID, deviceInfo := range vmDiskUUIDToDeviceInfoMap {
		if unmanagedDiskUUIDSet.Has(diskUUID) {
			unmanagedDiskDeviceInfoSet.Insert(deviceInfo)
		}
	}

	return ctrlInfoMap, managedDeviceInfoMap, unmanagedDiskDeviceInfoSet, nil
}

// ValidateVMSpecVolumesApplicationType validates volume application types
// and their associated controller and sharing mode requirements.
func ValidateVMSpecVolumesApplicationType(
	vmSpecVolumes []vmopv1.VirtualMachineVolume,
	ctrlInfoMap map[CtrlKey]CtrlInfo) error {

	for _, vol := range vmSpecVolumes {
		var (
			pvc       = vol.PersistentVolumeClaim
			ctrlInfo  CtrlInfo
			ctrlExist bool
		)

		if pvc.ApplicationType == "" {
			continue
		}

		if pvc.ControllerType != vmopv1.VirtualControllerTypeSCSI {
			return fmt.Errorf(ErrControllerMustBeSCSI, pvc.ApplicationType)
		}
		if pvc.DiskMode != vmopv1.VolumeDiskModeIndependentPersistent {
			return fmt.Errorf(ErrDiskModeMustBeIndependentPersistent,
				pvc.ApplicationType)
		}

		if pvc.ControllerType == "" || pvc.ControllerBusNumber == nil {
			continue
		}

		ctrlInfo, ctrlExist = ctrlInfoMap[CtrlKey{
			CtrlType:  pvc.ControllerType,
			BusNumber: *pvc.ControllerBusNumber,
		}]
		if !ctrlExist {
			return fmt.Errorf(ErrControllerNotFound, pvc.ControllerType,
				*pvc.ControllerBusNumber)
		}

		switch pvc.ApplicationType {
		case vmopv1.VolumeApplicationTypeOracleRAC:
			if pvc.SharingMode != vmopv1.VolumeSharingModeMultiWriter {
				return errors.New(ErrPVCSharingModeMustBeMultiWriter)
			}
			if ctrlInfo.SharingMode != vmopv1.VirtualControllerSharingModeNone {
				return errors.New(ErrControllerSharingModeMustBeNone)
			}
		case vmopv1.VolumeApplicationTypeMicrosoftWSFC:
			if pvc.SharingMode != vmopv1.VolumeSharingModeNone {
				return errors.New(ErrPVCSharingModeMustBeNone)
			}
			if ctrlInfo.SharingMode !=
				vmopv1.VirtualControllerSharingModePhysical {
				return errors.New(ErrControllerSharingModeMustBePhysical)
			}
		}
	}

	return nil
}

// validateUnitNumberRange validates unit numbers against controller-specific
// limits and reserved numbers (e.g., SCSI unit 7).
func validateUnitNumberRange(
	unitNumber int32,
	ctrlType vmopv1.VirtualControllerType,
	ctrlInfo CtrlInfo) error {
	if unitNumber < 0 {
		return errors.New(ErrUnitNumberNegative)
	}

	switch ctrlType {
	case vmopv1.VirtualControllerTypeIDE:
		if unitNumber > IDEControllerMaxUnitNumber {
			return fmt.Errorf(ErrIDEControllerUnitNumberTooHigh,
				IDEControllerMaxUnitNumber)
		}
	case vmopv1.VirtualControllerTypeNVME:
		if unitNumber > NVMeControllerMaxUnitNumber {
			return fmt.Errorf(ErrNVMEControllerUnitNumberTooHigh,
				NVMeControllerMaxUnitNumber)
		}
	case vmopv1.VirtualControllerTypeSATA:
		if unitNumber > SATAControllerMaxUnitNumber {
			return fmt.Errorf(ErrSATAControllerUnitNumberTooHigh,
				SATAControllerMaxUnitNumber)
		}
	case vmopv1.VirtualControllerTypeSCSI:
		switch ctrlInfo.SCSIType {
		case vmopv1.SCSIControllerTypeParaVirtualSCSI:
			if unitNumber > SCSIParaVirtualMaxUnitNumber {
				return fmt.Errorf(ErrSCSIParaVirtualUnitNumberTooHigh,
					SCSIParaVirtualMaxUnitNumber)
			}
		default:
			if unitNumber > SCSIControllerMaxUnitNumber {
				return fmt.Errorf(ErrSCSIControllerUnitNumberTooHigh,
					ctrlInfo.SCSIType, SCSIControllerMaxUnitNumber)
			}
		}
		if unitNumber == SCSIReservedUnitNumber {
			return fmt.Errorf(ErrSCSIControllerUnitNumberReserved,
				SCSIReservedUnitNumber)
		}
	}
	return nil
}

// ValidateVMSpecVolumesUnitNumber validates unit numbers for VM spec volumes
// to ensure they are within valid ranges and do not conflict with existing
// devices.
func ValidateVMSpecVolumesUnitNumber(
	vmSpecVolumes []vmopv1.VirtualMachineVolume,
	managedDeviceInfoMap map[VolumeKey]DeviceInfo,
	unmanagedDiskDeviceInfoSet sets.Set[DeviceInfo],
	ctrlInfoMap map[CtrlKey]CtrlInfo) error {

	expectedSet := unmanagedDiskDeviceInfoSet.Clone()
	for _, vol := range vmSpecVolumes {
		var (
			pvc     = vol.PersistentVolumeClaim
			devInfo *DeviceInfo
		)

		// when all information are provided to form a device information
		// for a newly added/updated volume
		if pvc.ControllerType != "" && pvc.ControllerBusNumber != nil &&
			pvc.UnitNumber != nil {
			ctrlKey := CtrlKey{
				CtrlType:  pvc.ControllerType,
				BusNumber: *pvc.ControllerBusNumber,
			}
			devInfo = &DeviceInfo{
				UnitNumber: *pvc.UnitNumber,
				CtrlKey:    ctrlKey,
			}

			ctrlInfo, ctrlExist := ctrlInfoMap[ctrlKey]
			if !ctrlExist {
				return fmt.Errorf(ErrControllerNotFound, pvc.ControllerType,
					*pvc.ControllerBusNumber)
			}

			// unit number range checking
			if err := validateUnitNumberRange(
				*pvc.UnitNumber, pvc.ControllerType, ctrlInfo); err != nil {
				return err
			}
		}

		// when no information is provided, try to find an existing match
		if pvc.ControllerType == "" && pvc.ControllerBusNumber == nil &&
			pvc.UnitNumber == nil {
			dv, exist := managedDeviceInfoMap[VolumeKey{
				VolumeName: vol.Name,
				PVCName:    pvc.ClaimName,
			}]
			if exist {
				devInfo = &dv
			}
		}

		if devInfo != nil {
			if expectedSet.Has(*devInfo) {
				return fmt.Errorf(ErrVolumeDuplicatedUnit, vol.Name)
			}
			expectedSet.Insert(*devInfo)
		}
	}

	return nil
}
