// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
)

// GetVirtualDiskFileNameAndUUID extracts the file name and disk UUID from a
// VirtualDisk device. Returns empty strings for unsupported
// backing types or when UUID is not available.
func GetVirtualDiskFileNameAndUUID(vd *vimtypes.VirtualDisk) (
	fileName string, diskUUID string) {
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
	return
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

// ConvertControllersGeneric is a generic helper that converts controller
// maps to sorted slices.
func ConvertControllersGeneric[T ControllerWithDevices](
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

// ParseNumericFields parses unit number and controller key from strings
func ParseNumericFields(pvcSpec cnsv1alpha1.PersistentVolumeClaimSpec) (
	int32, int32, error) {
	unitNumber, err := strconv.ParseInt(pvcSpec.UnitNumber, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid unit number %s: %w",
			pvcSpec.UnitNumber, err)
	}

	ctrlKey, err := strconv.ParseInt(pvcSpec.ControllerKey, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid controller key %s: %w",
			pvcSpec.ControllerKey, err)
	}

	return int32(unitNumber), int32(ctrlKey), nil
}
