// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// SelectDeviceFn returns true if the provided virtual device is a match.
type SelectDeviceFn[T vimtypes.BaseVirtualDevice] func(dev vimtypes.BaseVirtualDevice) bool

// SelectDevices returns a slice of the devices that match at least one of the
// provided selector functions.
func SelectDevices[T vimtypes.BaseVirtualDevice](
	devices []vimtypes.BaseVirtualDevice,
	selectorFns ...SelectDeviceFn[T],
) []T {

	var selectedDevices []T
	for i := range devices {
		if t, ok := devices[i].(T); ok {
			for j := range selectorFns {
				if selectorFns[j](t) {
					selectedDevices = append(selectedDevices, t)
					break
				}
			}

		}
	}
	return selectedDevices
}

// SelectDevicesByType returns a slice of the devices that are of type T.
func SelectDevicesByType[T vimtypes.BaseVirtualDevice](
	devices []vimtypes.BaseVirtualDevice,
) []T {

	var selectedDevices []T
	for i := range devices {
		if t, ok := devices[i].(T); ok {
			selectedDevices = append(selectedDevices, t)
		}
	}
	return selectedDevices
}

// SelectDevicesByBackingType returns a slice of the devices that have a
// backing of type B.
func SelectDevicesByBackingType[B vimtypes.BaseVirtualDeviceBackingInfo](
	devices []vimtypes.BaseVirtualDevice,
) []vimtypes.BaseVirtualDevice {

	var selectedDevices []vimtypes.BaseVirtualDevice
	for i := range devices {
		if baseDev := devices[i]; baseDev != nil {
			if dev := baseDev.GetVirtualDevice(); dev != nil {
				if _, ok := dev.Backing.(B); ok {
					selectedDevices = append(selectedDevices, baseDev)
				}
			}
		}
	}
	return selectedDevices
}

// SelectDevicesByDeviceAndBackingType returns a slice of the devices that are
// of type T with a backing of type B.
func SelectDevicesByDeviceAndBackingType[
	T vimtypes.BaseVirtualDevice,
	B vimtypes.BaseVirtualDeviceBackingInfo,
](
	devices []vimtypes.BaseVirtualDevice,
) []T {

	var selectedDevices []T
	for i := range devices {
		if t, ok := devices[i].(T); ok {
			if dev := t.GetVirtualDevice(); dev != nil {
				if _, ok := dev.Backing.(B); ok {
					selectedDevices = append(selectedDevices, t)
				}
			}
		}
	}
	return selectedDevices
}

// SelectDevicesByTypes returns a slice of the devices that match at least one
// the provided device types.
func SelectDevicesByTypes(
	devices []vimtypes.BaseVirtualDevice,
	deviceTypes ...vimtypes.BaseVirtualDevice,
) []vimtypes.BaseVirtualDevice {

	validDeviceTypes := map[reflect.Type]struct{}{}
	for i := range deviceTypes {
		validDeviceTypes[reflect.TypeOf(deviceTypes[i])] = struct{}{}
	}
	selectorFn := func(dev vimtypes.BaseVirtualDevice) bool {
		_, ok := validDeviceTypes[reflect.TypeOf(dev)]
		return ok
	}
	return SelectDevices[vimtypes.BaseVirtualDevice](devices, selectorFn)
}

// SelectVirtualPCIPassthrough returns a slice of *VirtualPCIPassthrough
// devices.
func SelectVirtualPCIPassthrough(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {

	return SelectDevicesByType[*vimtypes.VirtualPCIPassthrough](devices)
}

// IsDeviceNvidiaVgpu returns true if the provided device is an Nvidia vGPU.
func IsDeviceNvidiaVgpu(dev vimtypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimtypes.VirtualPCIPassthrough); ok {
		_, ok := dev.Backing.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
		return ok
	}
	return false
}

// IsDeviceDynamicDirectPathIO returns true if the provided device is a
// dynamic direct path I/O device.
func IsDeviceDynamicDirectPathIO(dev vimtypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimtypes.VirtualPCIPassthrough); ok {
		_, ok := dev.Backing.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
		return ok
	}
	return false
}

// HasDeviceChangeDeviceByType returns true of one of the device change's dev is that of type T.
func HasDeviceChangeDeviceByType[T vimtypes.BaseVirtualDevice](
	deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec,
) bool {
	for i := range deviceChanges {
		if spec := deviceChanges[i].GetVirtualDeviceConfigSpec(); spec != nil {
			if dev := spec.Device; dev != nil {
				if _, ok := dev.(T); ok {
					return true
				}
			}
		}
	}
	return false
}

// HasVirtualPCIPassthroughDeviceChange returns true if any of the device changes are for a passthrough device.
func HasVirtualPCIPassthroughDeviceChange(
	devices []vimtypes.BaseVirtualDeviceConfigSpec,
) bool {
	return HasDeviceChangeDeviceByType[*vimtypes.VirtualPCIPassthrough](devices)
}

// SelectNvidiaVgpu return a slice of Nvidia vGPU devices.
func SelectNvidiaVgpu(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {
	return selectVirtualPCIPassthroughWithVmiopBacking(devices)
}

// selectVirtualPCIPassthroughWithVmiopBacking returns a slice of PCI devices with VmiopBacking.
func selectVirtualPCIPassthroughWithVmiopBacking(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {

	return SelectDevicesByDeviceAndBackingType[
		*vimtypes.VirtualPCIPassthrough,
		*vimtypes.VirtualPCIPassthroughVmiopBackingInfo,
	](devices)
}

// SelectDynamicDirectPathIO returns a slice of dynamic direct path I/O devices.
func SelectDynamicDirectPathIO(
	devices []vimtypes.BaseVirtualDevice,
) []*vimtypes.VirtualPCIPassthrough {

	return SelectDevicesByDeviceAndBackingType[
		*vimtypes.VirtualPCIPassthrough,
		*vimtypes.VirtualPCIPassthroughDynamicBackingInfo,
	](devices)
}

func IsEthernetCard(dev vimtypes.BaseVirtualDevice) bool {
	_, ok := dev.(vimtypes.BaseVirtualEthernetCard)
	return ok
}

// isNonRDMDisk returns true for all virtual disk devices excluding disks with a raw device mapping backing.
func isNonRDMDisk(dev vimtypes.BaseVirtualDevice) bool {
	if dev, ok := dev.(*vimtypes.VirtualDisk); ok {
		_, hasRDMBacking := dev.Backing.(*vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo)
		return !hasRDMBacking
	}

	return false
}

// GetPreferredDiskFormat gets the preferred disk format. This function returns
// 4k if available, native 512 if available, an empty value if no formats are
// provided, else the first available format is returned.
func GetPreferredDiskFormat[T string | vimtypes.DatastoreSectorFormat](
	diskFormats ...T) vimtypes.DatastoreSectorFormat {

	if len(diskFormats) == 0 {
		return ""
	}

	var (
		supports4k        bool
		supportsNative512 bool
	)

	for i := range diskFormats {
		switch diskFormats[i] {
		case T(vimtypes.DatastoreSectorFormatNative_4k):
			supports4k = true
		case T(vimtypes.DatastoreSectorFormatNative_512):
			supportsNative512 = true
		}
	}

	if supports4k {
		return vimtypes.DatastoreSectorFormatNative_4k
	}
	if supportsNative512 {
		return vimtypes.DatastoreSectorFormatNative_512
	}

	return vimtypes.DatastoreSectorFormat(diskFormats[0])
}

// VirtualDiskInfo holds basic disk information extracted from a VirtualDisk.
type VirtualDiskInfo struct {
	FileName        string
	Label           string
	UUID            string
	DeviceKey       int32
	CryptoKey       *vimtypes.CryptoKeyId
	Sharing         vimtypes.VirtualDiskSharing
	DiskMode        string
	ThinProvisioned *bool
	EagerlyScrub    *bool
	HasParent       bool
	ControllerKey   int32
	UnitNumber      *int32
	CapacityInBytes int64
	Device          *vimtypes.VirtualDisk
}

// Name returns the name of the disk to use in the volume status as well as
// the UnmanagedVolumeSource.ID field.
func (vdi VirtualDiskInfo) Name() string {
	return vdi.Label
}

// GetVirtualDiskInfo extracts disk information from a VirtualDisk.
func GetVirtualDiskInfo(
	vd *vimtypes.VirtualDisk) VirtualDiskInfo {

	var vdi VirtualDiskInfo

	if vd == nil {
		return vdi
	}

	vdi.Device = vd
	vdi.DeviceKey = vd.Key
	vdi.ControllerKey = vd.ControllerKey
	vdi.CapacityInBytes = vd.CapacityInBytes
	vdi.UnitNumber = vd.UnitNumber

	switch tb := vd.Backing.(type) {
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		vdi.FileName = tb.FileName
		vdi.UUID = tb.Uuid
		vdi.CryptoKey = tb.KeyId
		vdi.DiskMode = tb.DiskMode
		vdi.HasParent = tb.Parent != nil
	case *vimtypes.VirtualDiskSparseVer1BackingInfo: // No UUID or Crypto
		vdi.FileName = tb.FileName
		vdi.DiskMode = tb.DiskMode
		vdi.HasParent = tb.Parent != nil
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		vdi.FileName = tb.FileName
		vdi.UUID = tb.Uuid
		vdi.CryptoKey = tb.KeyId
		vdi.DiskMode = tb.DiskMode
		vdi.HasParent = tb.Parent != nil
	case *vimtypes.VirtualDiskFlatVer1BackingInfo: // No UUID or Crypto
		vdi.FileName = tb.FileName
		vdi.DiskMode = tb.DiskMode
		vdi.HasParent = tb.Parent != nil
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		vdi.FileName = tb.FileName
		vdi.UUID = tb.Uuid
		vdi.CryptoKey = tb.KeyId
		vdi.Sharing = vimtypes.VirtualDiskSharing(tb.Sharing)
		vdi.DiskMode = tb.DiskMode
		vdi.ThinProvisioned = tb.ThinProvisioned
		vdi.EagerlyScrub = tb.EagerlyScrub
		vdi.HasParent = tb.Parent != nil
	case *vimtypes.VirtualDiskLocalPMemBackingInfo: // No Crypto
		vdi.FileName = tb.FileName
		vdi.UUID = tb.Uuid
		vdi.DiskMode = tb.DiskMode
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo: // No Crypto, No DiskMode
		vdi.FileName = tb.FileName
		vdi.UUID = tb.Uuid
		vdi.Sharing = vimtypes.VirtualDiskSharing(tb.Sharing)
	case *vimtypes.VirtualDiskRawDiskVer2BackingInfo: // No Crypto, No DiskMode
		vdi.FileName = tb.DescriptorFileName
		vdi.UUID = tb.Uuid
		vdi.Sharing = vimtypes.VirtualDiskSharing(tb.Sharing)
	case *vimtypes.VirtualDiskPartitionedRawDiskVer2BackingInfo: // No Crypto, No DiskMode
		vdi.FileName = tb.DescriptorFileName
		vdi.UUID = tb.Uuid
		vdi.Sharing = vimtypes.VirtualDiskSharing(tb.Sharing)
	}

	if di := vd.DeviceInfo; di != nil {
		if d := di.GetDescription(); d != nil {
			vdi.Label = d.Label
		}
	}

	return vdi
}

// VirtualCdromInfo holds basic disk information extracted from a VirtualCdrom.
type VirtualCdromInfo struct {
	DeviceName    string
	FileName      string
	ControllerKey int32
	UnitNumber    *int32
	Device        *vimtypes.VirtualCdrom
}

// GetVirtualCdromInfo extracts CD-ROM information from a VirtualCdrom.
func GetVirtualCdromInfo(
	vc *vimtypes.VirtualCdrom) VirtualCdromInfo {

	var cdi VirtualCdromInfo

	if vc == nil {
		return cdi
	}

	cdi.Device = vc
	cdi.ControllerKey = vc.ControllerKey
	cdi.UnitNumber = vc.UnitNumber

	switch tb := vc.Backing.(type) {
	case *vimtypes.VirtualCdromIsoBackingInfo:
		cdi.FileName = tb.FileName
	case *vimtypes.VirtualCdromRemotePassthroughBackingInfo:
		cdi.DeviceName = tb.DeviceName
	case *vimtypes.VirtualCdromAtapiBackingInfo:
		cdi.DeviceName = tb.DeviceName
	case *vimtypes.VirtualCdromRemoteAtapiBackingInfo:
		cdi.DeviceName = tb.DeviceName
	case *vimtypes.VirtualCdromPassthroughBackingInfo:
		cdi.DeviceName = tb.DeviceName
	}

	return cdi
}

// Sortable is a type constraint that requires a Compare method for sorting.
// Types that implement Sortable[T] must have a Compare method that returns
// a negative number if a < b, 0 if a == b, or a positive number if a > b.
type Sortable[T any] interface {
	Compare(T) int
}

// ControllerID represents a unique identifier for a virtual controller in a VM.
// It combines the controller type (IDE, NVME, SCSI, or SATA) with a bus number
// to uniquely identify a specific controller instance within the virtual machine.
type ControllerID struct {
	ControllerType vmopv1.VirtualControllerType
	BusNumber      int32
}

// String returns a string representation of the ControllerID in the format
// "controllerType:BusNumber".
func (c ControllerID) String() string {
	return fmt.Sprintf("%s:%d", c.ControllerType, c.BusNumber)
}

// Compare compares two ControllerID values and returns a negative number if
// c < b, 0 if c == b, or a positive number if c > b. It first compares by
// ControllerType, then by BusNumber.
func (c ControllerID) Compare(b ControllerID) int {
	if c.ControllerType != b.ControllerType {
		return strings.Compare(string(c.ControllerType), string(b.ControllerType))
	}
	return int(c.BusNumber - b.BusNumber)
}

// DevicePlacement represents the placement information for a virtual device
// attached to a controller. It combines a unique key (such as volume name or
// backing file name) with controller type, controller bus number, and unit
// number to uniquely identify the placement of a device (such as a disk or
// CD-ROM) on a specific controller within the virtual machine.
type DevicePlacement struct {
	Key                 string
	ControllerType      vmopv1.VirtualControllerType
	ControllerBusNumber int32
	UnitNumber          int32
}

// String returns a string representation of the DevicePlacement in the format
// "Key (controllerType:controllerBusNumber:unitNumber)".
func (d DevicePlacement) String() string {
	return fmt.Sprintf("%s (%s:%d:%d)", d.Key, d.ControllerType,
		d.ControllerBusNumber, d.UnitNumber)
}

// Compare compares two DevicePlacement values and returns a negative number if
// d < b, 0 if d == b, or a positive number if d > b. It first compares by Key,
// then by ControllerType, ControllerBusNumber, and UnitNumber.
func (d DevicePlacement) Compare(b DevicePlacement) int {
	if cmp := strings.Compare(d.Key, b.Key); cmp != 0 {
		return cmp
	}
	if d.ControllerType != b.ControllerType {
		return strings.Compare(string(d.ControllerType), string(b.ControllerType))
	}
	if d.ControllerBusNumber != b.ControllerBusNumber {
		return int(d.ControllerBusNumber - b.ControllerBusNumber)
	}
	return int(d.UnitNumber - b.UnitNumber)
}

// DiffSets compares expected and actual sets and returns sorted lists of
// missing and extra items. The type T must be comparable and implement
// Sortable[T] to provide a Compare method for sorting.
func DiffSets[T interface {
	comparable
	Sortable[T]
}](expected, actual sets.Set[T]) (missing, extra []T) {

	missingSet := expected.Difference(actual)
	if missingSet.Len() > 0 {
		missing = missingSet.UnsortedList()
		slices.SortFunc(missing, func(a, b T) int {
			return a.Compare(b)
		})
	}

	extraSet := actual.Difference(expected)
	if extraSet.Len() > 0 {
		extra = extraSet.UnsortedList()
		slices.SortFunc(extra, func(a, b T) int {
			return a.Compare(b)
		})
	}

	return missing, extra
}

// HardwareInfo stores information about the actual hardware devices
// attached to a VM, including controllers, disks, and CD-ROMs.
type HardwareInfo struct {
	Controllers sets.Set[ControllerID]
	Disks       sets.Set[DevicePlacement]
	CDROMs      sets.Set[DevicePlacement]
}

// BuildHardwareInfo extracts hardware information from the VM's managed object,
// including all attached controllers, disks, and CD-ROMs.
func BuildHardwareInfo(moVM mo.VirtualMachine) HardwareInfo {

	var (
		hwInfo = HardwareInfo{
			Controllers: sets.New[ControllerID](),
			Disks:       sets.New[DevicePlacement](),
			CDROMs:      sets.New[DevicePlacement](),
		}
		controllerInfoMap = make(map[int32]ControllerID)
	)

	if moVM.Config == nil || len(moVM.Config.Hardware.Device) == 0 {
		return hwInfo
	}

	// Collect all controllers.
	for _, device := range moVM.Config.Hardware.Device {

		deviceKey := device.GetVirtualDevice().Key
		if deviceKey == 0 {
			continue
		}

		var controllerID ControllerID
		switch ctrl := device.(type) {
		case *vimtypes.VirtualIDEController:
			controllerID = ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      ctrl.BusNumber,
			}
		case vimtypes.BaseVirtualSCSIController:
			scsiCtrl := ctrl.GetVirtualSCSIController()
			controllerID = ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSCSI,
				BusNumber:      scsiCtrl.BusNumber,
			}
		case vimtypes.BaseVirtualSATAController:
			sataCtrl := ctrl.GetVirtualSATAController()
			controllerID = ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				BusNumber:      sataCtrl.BusNumber,
			}
		case *vimtypes.VirtualNVMEController:
			controllerID = ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeNVME,
				BusNumber:      ctrl.BusNumber,
			}
		default:
			continue
		}

		controllerInfoMap[deviceKey] = controllerID
		hwInfo.Controllers.Insert(controllerID)
	}

	// Collect all attached virtual disks and CD-ROMs.
	for _, device := range moVM.Config.Hardware.Device {
		var (
			controllerKey int32
			unitNumber    *int32
			placementKey  string
			deviceType    vmopv1.VirtualDeviceType
		)

		switch dev := device.(type) {
		case *vimtypes.VirtualDisk:
			deviceType = vmopv1.VirtualDeviceTypeDisk
			vdi := GetVirtualDiskInfo(dev)
			controllerKey = vdi.ControllerKey
			unitNumber = vdi.UnitNumber
			placementKey = vdi.UUID

		case *vimtypes.VirtualCdrom:
			deviceType = vmopv1.VirtualDeviceTypeCDROM
			cdi := GetVirtualCdromInfo(dev)
			controllerKey = cdi.ControllerKey
			unitNumber = cdi.UnitNumber
			placementKey = cdi.FileName

		default:
			continue
		}

		if placementKey == "" || controllerKey == 0 || unitNumber == nil {
			continue
		}

		controllerInfo, ok := controllerInfoMap[controllerKey]
		if !ok {
			continue
		}

		devicePlacement := DevicePlacement{
			Key:                 placementKey,
			ControllerType:      controllerInfo.ControllerType,
			ControllerBusNumber: controllerInfo.BusNumber,
			UnitNumber:          *unitNumber,
		}

		switch deviceType {
		case vmopv1.VirtualDeviceTypeDisk:
			hwInfo.Disks.Insert(devicePlacement)
		case vmopv1.VirtualDeviceTypeCDROM:
			hwInfo.CDROMs.Insert(devicePlacement)
		}

	}

	return hwInfo
}
