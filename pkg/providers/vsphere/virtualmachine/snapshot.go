// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// SnapshotArgs contains the options for createSnapshot.
type SnapshotArgs struct {
	VMCtx          pkgctx.VirtualMachineContext
	VcVM           *object.VirtualMachine
	VMSnapshot     vmopv1.VirtualMachineSnapshot
	RemoveChildren bool
	Consolidate    *bool
}

// Snapshot related errors.
var (
	ErrNoSnapshots       = errors.New("no snapshots for this VM")
	ErrSnapshotNotFound  = errors.New("snapshot not found")
	ErrMultipleSnapshots = errors.New("multiple snapshots found")
)

func SnapshotVirtualMachine(args SnapshotArgs) (*vimtypes.ManagedObjectReference, error) {
	snapshotName := args.VMSnapshot.Name

	logger := args.VMCtx.Logger.WithValues("snapshotName", snapshotName)
	snapMoRef, _ := args.VcVM.FindSnapshot(args.VMCtx, snapshotName) // TODO: Use our FindSnapshot impl and check error?
	if snapMoRef != nil {
		logger.Info("Snapshot already exists")
		// Return early, snapshot found
		return snapMoRef, nil
	}

	// If no snapshot was found, create it
	logger.Info("Creating Snapshot of VirtualMachine")
	snapMoRef, err := CreateSnapshot(args)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot for VM: %w", err)
	}

	return snapMoRef, nil
}

func CreateSnapshot(args SnapshotArgs) (*vimtypes.ManagedObjectReference, error) {
	snapObj := args.VMSnapshot
	var quiesceSpec *vimtypes.VirtualMachineGuestQuiesceSpec
	if quiesce := snapObj.Spec.Quiesce; quiesce != nil {
		quiesceSpec = &vimtypes.VirtualMachineGuestQuiesceSpec{
			Timeout: int32(quiesce.Timeout.Round(time.Minute).Minutes()),
		}
	}

	t, err := args.VcVM.CreateSnapshotEx(args.VMCtx, snapObj.Name, snapObj.Spec.Description, snapObj.Spec.Memory, quiesceSpec)
	if err != nil {
		return nil, err
	}

	taskInfo, err := t.WaitForResult(args.VMCtx)
	if err != nil {
		args.VMCtx.Logger.V(5).Error(err, "create snapshot task failed", "taskInfo", taskInfo)
		return nil, err
	}

	snapMoRef, ok := taskInfo.Result.(vimtypes.ManagedObjectReference)
	if !ok {
		return nil, fmt.Errorf("create vmSnapshot task failed: %v", taskInfo.Result)
	}

	return &snapMoRef, nil
}

func DeleteSnapshot(args SnapshotArgs) error {
	t, err := args.VcVM.RemoveSnapshot(args.VMCtx, args.VMSnapshot.Name, args.RemoveChildren, args.Consolidate)
	if err != nil {
		// Catch the not found error from govmomi:
		// https://github.com/vmware/govmomi/blob/v0.52.0-alpha.0/object/virtual_machine.go#L784
		// https://github.com/vmware/govmomi/blob/v0.52.0-alpha.0/object/virtual_machine.go#L775
		if strings.Contains(err.Error(), fmt.Sprintf("snapshot %q not found", args.VMSnapshot.Name)) ||
			strings.Contains(err.Error(), "no snapshots for this VM") {
			return ErrSnapshotNotFound
		}
		return err
	}

	if err := t.Wait(args.VMCtx); err != nil {
		args.VMCtx.Logger.V(5).Error(err, "delete snapshot task failed")
		return err
	}

	return nil
}

// FindSnapshot returns the snapshot matching a given name from the
// snapshots present on a VM. Much of this is taken from Govmomi, but
// we maintain our version because we don't want to make another
// property collector round trip to fetch those properties again.
func FindSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	snapshotName string) (*vimtypes.ManagedObjectReference, error) {

	o := vmCtx.MoVM
	if o.Snapshot == nil || len(o.Snapshot.RootSnapshotList) == 0 {
		return nil, ErrNoSnapshots
	}

	m := make(snapshotMap)
	m.add("", o.Snapshot.RootSnapshotList)

	s := m[snapshotName]
	switch len(s) {
	case 0:
		return nil, fmt.Errorf("snapshot %q not found: %w", snapshotName, ErrSnapshotNotFound)
	case 1:
		return &s[0], nil
	default:
		return nil, fmt.Errorf("%q resolves to %d snapshots: %w", snapshotName, len(s), ErrMultipleSnapshots)
	}
}

// snapshotMap is a custom type that traverses over the entire snapshot tree.
type snapshotMap map[string][]vimtypes.ManagedObjectReference

func (m snapshotMap) add(parent string, tree []vimtypes.VirtualMachineSnapshotTree) {
	for i, st := range tree {
		sname := st.Name
		names := []string{sname, st.Snapshot.Value}

		if parent != "" {
			sname = path.Join(parent, sname)
			// Add full path as an option to resolve duplicate names
			names = append(names, sname)
		}

		for _, name := range names {
			m[name] = append(m[name], tree[i].Snapshot)
		}

		m.add(sname, st.ChildSnapshotList)
	}
}

// GetSnapshotSize calculates the size of a given snapshot in bytes. It includes
// the memory file, the vmdk files, and the vmsn file, and excludes FCDs.
// The algorithm use almost the same logic as how VC UI calculates the snapshot size.
// The only difference is that this function does not include the size of FCDs.
func GetSnapshotSize(vmCtx pkgctx.VirtualMachineContext, vmSnapshot *vimtypes.ManagedObjectReference) int64 {
	if vmSnapshot == nil || vmSnapshot.Value == "" {
		vmCtx.Logger.V(5).Info("vmSnapshot is nil or empty")
		return 0
	}

	vmLayout := vmCtx.MoVM.LayoutEx
	if vmLayout == nil {
		vmCtx.Logger.V(5).Info("vmCtx.MoVM.LayoutEx is nil, skip calculating snapshot size")
		return 0
	}

	var snapshot *vimtypes.VirtualMachineFileLayoutExSnapshotLayout
	for i := range vmLayout.Snapshot {
		if vmLayout.Snapshot[i].Key.Value == vmSnapshot.Value {
			snapshot = &vmLayout.Snapshot[i]
			break
		}
	}

	if snapshot == nil {
		// TODO: Error if snapshot not found?
		return 0
	}

	fcdDeviceKeySet := getFCDDeviceKeySet(vmCtx)
	fileKeyMap := make(map[int32]int64)
	for _, file := range vmLayout.File {
		fileKeyMap[file.Key] = file.Size
	}

	var total int64

	// Add the file key for the snapshot memory (.vmem) file if present.
	if snapshot.MemoryKey != -1 {
		vmCtx.Logger.V(5).Info("Adding memoryKey", "memoryKey", snapshot.MemoryKey)
		total += fileKeyMap[snapshot.MemoryKey]
	}

	vmCtx.Logger.V(5).Info("Adding the file key for snapshot (.vmsn) file", "dataKey", snapshot.DataKey)
	total += fileKeyMap[snapshot.DataKey]

	// Add the disk files for the child most delta disk which is the last item in the disk chain.
	for _, disk := range snapshot.Disk {
		if fcdDeviceKeySet.Has(disk.Key) {
			// A VM snapshot creates a delta disk for all disks of a VM -- including PVCs that are
			// backed by FCDs. Since the usage of the delta disks created on PVCs will already be
			// reported by the VolumeSnapshot SPU, we skip those here to avoid double counting.
			vmCtx.Logger.V(5).Info("Skipping the disk file of the FCD", "diskKey", disk.Key)
			continue
		}

		if len(disk.Chain) == 0 {
			vmCtx.Logger.V(5).Info("Skipping the disk since its chain is empty", "diskKey", disk.Key)
			continue
		}

		// File keys for the child most delta disk
		childMostFileKeys := disk.Chain[len(disk.Chain)-1].FileKey
		if len(childMostFileKeys) > 0 {
			vmCtx.Logger.V(5).Info("Adding file key for the child most delta disk of the snapshot",
				"fileKey", childMostFileKeys)
			for _, fileKey := range childMostFileKeys {
				total += fileKeyMap[fileKey]
			}
		}
	}

	return total
}

// getFCDDeviceKeySet returns a set of device keys of disk devices that are FCDs.
func getFCDDeviceKeySet(vmCtx pkgctx.VirtualMachineContext) sets.Set[int32] {
	deviceKeysSet := sets.Set[int32]{}
	device := object.VirtualDeviceList(vmCtx.MoVM.Config.Hardware.Device)
	for _, d := range device.SelectByType(&vimtypes.VirtualDisk{}) {
		disk := d.(*vimtypes.VirtualDisk)
		if disk.VDiskId == nil || disk.VDiskId.Id == "" { // FCDs have VDiskId.
			continue
		}

		deviceKeysSet.Insert(disk.Key)
	}

	return deviceKeysSet
}

// GetParentSnapshot finds the parent snapshot of a given snapshot name.
func GetParentSnapshot(vmCtx pkgctx.VirtualMachineContext, vcVM *object.VirtualMachine, vmSnapshotName string) (*vimtypes.VirtualMachineSnapshotTree, error) {
	var o mo.VirtualMachine

	err := vcVM.Properties(vmCtx, vcVM.Reference(), []string{"snapshot"}, &o)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	if o.Snapshot == nil || len(o.Snapshot.RootSnapshotList) == 0 {
		return nil, nil
	}

	parent := getParentSnapshotHelper(nil, o.Snapshot.RootSnapshotList, vmSnapshotName)
	if parent != nil {
		return parent, nil
	}

	return nil, nil
}

func getParentSnapshotHelper(
	parent *vimtypes.VirtualMachineSnapshotTree,
	children []vimtypes.VirtualMachineSnapshotTree,
	target string) *vimtypes.VirtualMachineSnapshotTree {
	for _, child := range children {
		if child.Name == target {
			return parent
		}
		parent := getParentSnapshotHelper(&child, child.ChildSnapshotList, target)
		if parent != nil {
			return parent
		}
	}
	return nil
}
