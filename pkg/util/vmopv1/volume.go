// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"path"
	"strings"

	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"
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

// GetVirtualDiskName returns a human-readable name for a VirtualDisk device.
// It extracts the base filename without extension from the disk's backing
// store.
func GetVirtualDiskName(vd *vimtypes.VirtualDisk) (name string) {
	fileName, _ := GetVirtualDiskFileNameAndUUID(vd)
	name = fileNameToName(fileName)
	return
}
