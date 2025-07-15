// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
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
	ErrNoSnapshots            = errors.New("no snapshots for this VM")
	ErrSnapshotNotFound       = errors.New("snapshot not found")
	ErrMultipleSnapshots      = errors.New("multiple snapshots found")
	ErrParentSnapshotNotFound = errors.New("parent snapshot not found")
)

func SnapshotVirtualMachine(args SnapshotArgs) (*types.ManagedObjectReference, error) {
	obj := args.VMSnapshot
	vm := args.VcVM
	// Find snapshot by name
	snapMoRef, _ := vm.FindSnapshot(args.VMCtx, obj.Name)
	if snapMoRef != nil {
		// TODO: Handle revert to snapshot. Need a way to compare currentSnapshot's moID
		// 	with spec.currentSnap
		//
		args.VMCtx.Logger.Info("Snapshot already exists", "snapshot name", obj.Name)
		// Update vm.status with currentSnapshot
		updateVMStatusCurrentSnapshot(args.VMCtx, obj)
		// Return early, snapshot found
		return snapMoRef, nil
	}

	// If no snapshot was found, create it
	args.VMCtx.Logger.Info("Creating Snapshot of VirtualMachine", "snapshot name", obj.Name)
	snapMoRef, err := CreateSnapshot(args)
	if err != nil {
		args.VMCtx.Logger.Error(err, "failed to create snapshot for VM", "snapshot", obj.Name)
		return nil, err
	}

	// Update vm.status with currentSnapshot
	updateVMStatusCurrentSnapshot(args.VMCtx, obj)
	return snapMoRef, nil
}

func CreateSnapshot(args SnapshotArgs) (*types.ManagedObjectReference, error) {
	snapObj := args.VMSnapshot
	var quiesceSpec *types.VirtualMachineGuestQuiesceSpec
	if quiesce := snapObj.Spec.Quiesce; quiesce != nil {
		quiesceSpec = &types.VirtualMachineGuestQuiesceSpec{
			Timeout: int32(quiesce.Timeout.Round(time.Minute).Minutes()),
		}
	}

	t, err := args.VcVM.CreateSnapshotEx(args.VMCtx, snapObj.Name, snapObj.Spec.Description, snapObj.Spec.Memory, quiesceSpec)
	if err != nil {
		return nil, err
	}

	// Wait for task to finish
	taskInfo, err := t.WaitForResult(args.VMCtx)
	if err != nil {
		args.VMCtx.Logger.V(5).Error(err, "create snapshot task failed", "taskInfo", taskInfo)
		return nil, err
	}

	snapMoRef, ok := taskInfo.Result.(types.ManagedObjectReference)
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

	// Wait for task to finish
	if err := t.Wait(args.VMCtx); err != nil {
		args.VMCtx.Logger.V(5).Error(err, "delete snapshot task failed")
		return err
	}

	return nil
}

func updateVMStatusCurrentSnapshot(vmCtx pkgctx.VirtualMachineContext, vmSnapshot vmopv1.VirtualMachineSnapshot) {
	vmCtx.VM.Status.CurrentSnapshot = &vmopv1common.LocalObjectRef{
		APIVersion: vmSnapshot.APIVersion,
		Kind:       vmSnapshot.Kind,
		Name:       vmSnapshot.Name,
	}
}

// FindSnapshot returns the snapshot matching a given name from the
// snapshots present on a VM. Much of this is taken from Govmomi, but
// we maintain our version because we don't want to make another
// property collector round trip to fetch those properties again.
func FindSnapshot(
	vmCtx pkgctx.VirtualMachineContext,
	snapshotName string) (*types.ManagedObjectReference, error) {

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
type snapshotMap map[string][]types.ManagedObjectReference

func (m snapshotMap) add(parent string, tree []types.VirtualMachineSnapshotTree) {
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

// GetSnapshotSize calculates the size of a given snapshot in bytes. It include
// the memory file, the vmdk files, and the vmsn file.
// Additionally, it excludes the FCDs.
// The algorithm follows the logic from VC UI
// https://opengrok2.vdp.lvn.broadcom.net/xref/main.perforce.1666/vim-clients/applications/vsphere-client/server/h5-vm-service/src/main/java/com/vmware/vsphere/client/h5/vm/impl/DiskUsagePropertyProvider.java?r=11459544#38-54
func GetSnapshotSize(vmCtx pkgctx.VirtualMachineContext, vmSnapshot *types.ManagedObjectReference) int64 {
	if vmSnapshot == nil || vmSnapshot.Value == "" {
		vmCtx.Logger.V(5).Info("vmSnapshot is nil or empty")
		return 0
	}

	if vmCtx.MoVM.LayoutEx == nil {
		vmCtx.Logger.V(5).Info("vmCtx.MoVM.LayoutEx is nil, skip calculating snapshot size")
		return 0
	}
	vmlayout := vmCtx.MoVM.LayoutEx
	vmDevices := vmCtx.MoVM.Config.Hardware.Device

	var fileKeyList []int

	// Find the VirtualMachineFileLayoutExSnapshotLayout for current snapshot
	for _, snapshot := range vmlayout.Snapshot {
		if snapshot.Key.Value == vmSnapshot.Value {
			// Add the memoryKey if it exists.
			if snapshot.MemoryKey != -1 { // .vmem
				vmCtx.Logger.V(5).Info("Adding memoryKey", "memoryKey", snapshot.MemoryKey)
				fileKeyList = append(fileKeyList, int(snapshot.MemoryKey))
			}

			// .vmsn
			vmCtx.Logger.V(5).Info("Adding dataKey", "dataKey", snapshot.DataKey)
			fileKeyList = append(fileKeyList, int(snapshot.DataKey))

			// Add the last element of fileKeys in the disk.chain.
			// .vmdk files
			for _, disk := range snapshot.Disk {
				for _, chain := range disk.Chain {
					if len(chain.FileKey) > 0 {
						vmCtx.Logger.V(5).Info("Adding fileKey", "fileKey", chain.FileKey[len(chain.FileKey)-1])
						fileKeyList = append(fileKeyList, int(chain.FileKey[len(chain.FileKey)-1]))
					}
				}
			}
		}
	}

	// Find the keySets for FCDs from layoutEx.file
	fcdDeviceFileKeySet := getFCDDevicesFileKeySet(vmCtx, vmDevices, vmlayout.File)
	// Remove the fileKeys that are part of the FCDs
	fileKeyList = slices.DeleteFunc(fileKeyList, func(key int) bool {
		return fcdDeviceFileKeySet.Has(key)
	})

	vmCtx.Logger.V(5).Info("final fileKeyList after removing FCD related fileKeys", "fileKeyList", fileKeyList)

	fileKeyMap := make(map[int]types.VirtualMachineFileLayoutExFileInfo)
	for _, file := range vmlayout.File {
		fileKeyMap[int(file.Key)] = file
	}

	var size int64
	for _, fileKey := range fileKeyList {
		if file, ok := fileKeyMap[fileKey]; ok {
			size += file.Size
		}
		// Assume the size is 0 if it's not found in the fileKeyMap
	}

	return size
}

// getFCDDevicesFileKeySet extracts fileKeys of layoutEx.Files that are part of the FCDs.
// Here we use the file name to identify the FCDs because the backingObjectID
// is not always available. and there is no other uuid available.
// Example could be found here https://vmw-jira.broadcom.net/browse/VMSVC-2700?focusedId=20244583&page=com.atlassian.jira.plugin.system.issuetabpanels:comment-tabpanel#comment-20244583
func getFCDDevicesFileKeySet(vmCtx pkgctx.VirtualMachineContext, vmDevices []types.BaseVirtualDevice, vmlayoutFileInfos []types.VirtualMachineFileLayoutExFileInfo) sets.Set[int] {
	fcdDevicesBackingFileNamePrefixSet := sets.Set[string]{}
	for _, device := range vmDevices {
		vd, ok := device.(*types.VirtualDisk)
		if !ok || vd.VDiskId == nil || vd.VDiskId.Id == "" { // FCD has VDiskId
			continue
		}

		fileName := ""
		switch tb := vd.Backing.(type) {
		case *types.VirtualDiskFlatVer2BackingInfo:
			fileName = tb.FileName
		case *types.VirtualDiskSeSparseBackingInfo:
			fileName = tb.FileName
		case *types.VirtualDiskRawDiskMappingVer1BackingInfo:
			fileName = tb.FileName
		case *types.VirtualDiskSparseVer2BackingInfo:
			fileName = tb.FileName
		case *types.VirtualDiskRawDiskVer2BackingInfo:
			fileName = tb.DescriptorFileName
		}
		// The device backing file name might look like below:
		// "[sharedVmfs-0] fcd/c4f94aecf2cf42ba9bb66fb6c9fc3527-000002.vmdk"
		// While the file name in the layoutEx.file might be like below:
		// "[sharedVmfs-0] fcd/c4f94aecf2cf42ba9bb66fb6c9fc3527-000001.vmdk"
		// "[sharedVmfs-0] fcd/c4f94aecf2cf42ba9bb66fb6c9fc3527-000001-sesparse.vmdk"
		// "[sharedVmfs-0] fcd/c4f94aecf2cf42ba9bb66fb6c9fc3527-flat.vmdk"

		// In order to find the corresponding fcd related file name in the layoutEx.file,
		// get the uuid from fileName and check whether the layoutEx.file.Name contains the fcd/<uuid>.
		// If yes, add the file.Key of that layoutEx.file to the fcdDeviceFileKeySet

		// Use regex matcher to find the string like fcd/[a-zA-Z0-9] from the layoutEx.file.Name.
		re := regexp.MustCompile(`fcd\/[a-zA-Z0-9]+`)
		matches := re.FindStringSubmatch(fileName)
		if len(matches) > 0 {
			fcdPrefix := matches[0]
			fcdDevicesBackingFileNamePrefixSet.Insert(fcdPrefix)
		} else {
			vmCtx.Logger.Error(fmt.Errorf("no fcd/[a-zA-Z0-9] substring found"), "fileName", fileName)
		}
	}

	fcdDeviceFileKeySet := sets.Set[int]{}
	for _, file := range vmlayoutFileInfos {
		for prefix := range fcdDevicesBackingFileNamePrefixSet {
			if strings.Contains(file.Name, prefix) {
				fcdDeviceFileKeySet.Insert(int(file.Key))
				break
			}
		}
	}

	return fcdDeviceFileKeySet
}

// GetParentSnapshot finds the parent snapshot of a given snapshot name.
func GetParentSnapshot(vmCtx pkgctx.VirtualMachineContext, vmSnapshotName string) *types.VirtualMachineSnapshotTree {
	o := vmCtx.MoVM

	if o.Snapshot == nil || len(o.Snapshot.RootSnapshotList) == 0 {
		return nil
	}

	parent := getParentSnapshotHelper(nil, o.Snapshot.RootSnapshotList, vmSnapshotName)
	if parent != nil {
		return parent
	}

	return nil
}

func getParentSnapshotHelper(parent *types.VirtualMachineSnapshotTree, children []types.VirtualMachineSnapshotTree, target string) *types.VirtualMachineSnapshotTree {
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
