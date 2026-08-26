// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package snapshotdisk

import (
	"context"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

type SnapshotFetchResult = snapshotFetchResult
type ControllerMapFn = controllerMapFn

var (
	GetControllerMapFromMoVM         = getControllerMapFromMoVM
	NewControllerMapFn               = newControllerMapFn
	ResolveControllerMapFn           = resolveControllerMapFn
	AttachSnapshotDisks              = attachSnapshotDisks
	DetachSnapshotDisks              = detachSnapshotDisks
	GetVirtualDiskUUID               = getVirtualDiskUUID
	IsSnapshotDiskAttachedInVMMo     = isSnapshotDiskAttachedInVMMo
	FindControllerKeyForSnapshotDisk = findControllerKeyForSnapshotDisk
	AddSnapshotDiskInConfigSpec      = addSnapshotDiskInConfigSpec
	FindDiskInSnapshot               = findDiskInSnapshot
	IsSnapshotDiskVolumeAttachedInStatus = isSnapshotDiskVolumeAttachedInStatus
	IsSnapshotDiskSlotMatch              = isSnapshotDiskSlotMatch
	IsSnapshotDiskVolumeInSpec           = isSnapshotDiskVolumeInSpec
	FindSnapshotDiskInHardware       = findSnapshotDiskInHardware
	SetRetrieveSnapshotHardware      = func(fn func(ctx context.Context, vimClient *vim25.Client, snapRef vimtypes.ManagedObjectReference) (*mo.VirtualMachineSnapshot, error)) func() {
		orig := retrieveSnapshotHardware
		retrieveSnapshotHardware = fn
		return func() {
			retrieveSnapshotHardware = orig
		}
	}
)

func (i *diskBackingInfo) CreateBacking() vimtypes.BaseVirtualDeviceBackingInfo {
	if i == nil || i.createBacking == nil {
		return nil
	}
	return i.createBacking()
}
