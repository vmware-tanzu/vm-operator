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

type DiskBackingInfo = diskBackingInfo
type SnapshotFetchResult = snapshotFetchResult
type VolumePlacement = volumePlacement

var (
	CreateSnapshotDiskBacking        = createSnapshotDiskBacking
	RemoveObsoleteSnapshotDisks      = removeObsoleteSnapshotDisks
	GetVirtualDiskUUID               = getVirtualDiskUUID
	GetDiskBackingInfo               = getDiskBackingInfo
	IsDiskDerivedFromSnapshotDisk    = isDiskDerivedFromSnapshotDisk
	IsSnapshotDiskAttached           = isSnapshotDiskAttached
	FindControllerKeyForSnapshotDisk = findControllerKeyForSnapshotDisk
	EnsureSnapshotDiskAttached       = ensureSnapshotDiskAttached
	GetVolumeStatusPlacement         = getVolumeStatusPlacement
	GetEffectivePlacement            = getEffectivePlacement
	SetRetrieveSnapshotHardware      = func(fn func(ctx context.Context, vimClient *vim25.Client, snapRef vimtypes.ManagedObjectReference) (*mo.VirtualMachineSnapshot, error)) func() {
		orig := retrieveSnapshotHardware
		retrieveSnapshotHardware = fn
		return func() {
			retrieveSnapshotHardware = orig
		}
	}
)
