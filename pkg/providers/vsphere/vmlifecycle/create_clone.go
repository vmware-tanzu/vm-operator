// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/placement"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// CloneVMFromInventory creates a new VM by cloning the source VM. This is not reachable/used
// in production because we only really support deploying an OVF via content library.
// Maybe someday we'll use clone to speed up VM deployment so keep this code around and unit tested.
func cloneVMFromInventory(
	vmCtx pkgctx.VirtualMachineContext,
	finder *find.Finder,
	createArgs *CreateArgs) (*vimtypes.ManagedObjectReference, error) {

	srcVMName := createArgs.ProviderItemID // AKA: vmCtx.VM.Spec.Image.Name

	srcVM, err := finder.VirtualMachine(vmCtx, srcVMName)
	if err != nil {
		return nil, fmt.Errorf("failed to find clone source VM: %s: %w", srcVMName, err)
	}

	cloneSpec, err := createCloneSpec(vmCtx, createArgs, srcVM)
	if err != nil {
		return nil, fmt.Errorf("failed to create CloneSpec: %w", err)
	}

	// We always set cloneSpec.Location.Folder so use that to get the parent folder object.
	folder := object.NewFolder(srcVM.Client(), *cloneSpec.Location.Folder)

	cloneTask, err := srcVM.Clone(vmCtx, folder, cloneSpec.Config.Name, *cloneSpec)
	if err != nil {
		return nil, err
	}

	result, err := cloneTask.WaitForResult(vmCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("clone VM task failed: %w", err)
	}

	ref := result.Result.(vimtypes.ManagedObjectReference)
	return &ref, nil
}

func createCloneSpec(
	vmCtx pkgctx.VirtualMachineContext,
	createArgs *CreateArgs,
	srcVM *object.VirtualMachine) (*vimtypes.VirtualMachineCloneSpec, error) {

	cloneSpec := &vimtypes.VirtualMachineCloneSpec{
		Config: &createArgs.ConfigSpec,
		Memory: ptr.To(false), // No full memory clones.
	}

	virtualDevices, err := srcVM.Device(vmCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get clone source VM devices: %w", err)
	}

	virtualDisks := virtualDevices.SelectByType((*vimtypes.VirtualDisk)(nil))

	for _, deviceChange := range resizeBootDiskDeviceChange(vmCtx, virtualDisks) {
		if deviceChange.GetVirtualDeviceConfigSpec().Operation == vimtypes.VirtualDeviceConfigSpecOperationEdit {
			cloneSpec.Location.DeviceChange = append(cloneSpec.Location.DeviceChange, deviceChange)
		} else {
			cloneSpec.Config.DeviceChange = append(cloneSpec.Config.DeviceChange, deviceChange)
		}
	}

	if createArgs.StorageProfileID != "" {
		cloneSpec.Location.Profile = []vimtypes.BaseVirtualMachineProfileSpec{
			&vimtypes.VirtualMachineDefinedProfileSpec{ProfileId: createArgs.StorageProfileID},
		}
	} else {
		// BMV: Used to compute placement? Otherwise, always overwritten later.
		cloneSpec.Location.Datastore = &vimtypes.ManagedObjectReference{
			Type:  "Datastore",
			Value: createArgs.DatastoreMoID,
		}
	}

	cloneSpec.Location.Pool = &vimtypes.ManagedObjectReference{
		Type:  "ResourcePool",
		Value: createArgs.ResourcePoolMoID,
	}
	cloneSpec.Location.Folder = &vimtypes.ManagedObjectReference{
		Type:  "Folder",
		Value: createArgs.FolderMoID,
	}

	rpOwner, err := object.NewResourcePool(srcVM.Client(), *cloneSpec.Location.Pool).Owner(vmCtx)
	if err != nil {
		return nil, err
	}

	cluster, ok := rpOwner.(*object.ClusterComputeResource)
	if !ok {
		return nil, fmt.Errorf("owner of the ResourcePool is not a cluster but %T", rpOwner)
	}

	relocateSpec, err := placement.CloneVMRelocateSpec(vmCtx, cluster, srcVM.Reference(), cloneSpec)
	if err != nil {
		return nil, err
	}

	cloneSpec.Location.Host = relocateSpec.Host
	cloneSpec.Location.Datastore = relocateSpec.Datastore
	cloneSpec.Location.Disk = cloneVMDiskLocators(virtualDisks, createArgs, cloneSpec.Location)

	return cloneSpec, nil
}

func cloneVMDiskLocators(
	disks object.VirtualDeviceList,
	createArgs *CreateArgs,
	location vimtypes.VirtualMachineRelocateSpec) []vimtypes.VirtualMachineRelocateSpecDiskLocator {

	diskLocators := make([]vimtypes.VirtualMachineRelocateSpecDiskLocator, 0, len(disks))

	for _, disk := range disks {
		locator := vimtypes.VirtualMachineRelocateSpecDiskLocator{
			DiskId:    disk.GetVirtualDevice().Key,
			Datastore: *location.Datastore,
			Profile:   location.Profile,
			// TODO: Check if policy is encrypted and use correct DiskMoveType
			DiskMoveType: string(vimtypes.VirtualMachineRelocateDiskMoveOptionsMoveChildMostDiskBacking),
		}

		if backing, ok := disk.(*vimtypes.VirtualDisk).Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo); ok {
			switch createArgs.StorageProvisioning {
			case string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeThin):
				backing.ThinProvisioned = ptr.To(true)
			case string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeThick):
				backing.ThinProvisioned = ptr.To(false)
			case string(vimtypes.OvfCreateImportSpecParamsDiskProvisioningTypeEagerZeroedThick):
				backing.EagerlyScrub = ptr.To(true)
			}
			locator.DiskBackingInfo = backing
		}

		diskLocators = append(diskLocators, locator)
	}

	return diskLocators
}

func resizeBootDiskDeviceChange(
	vmCtx pkgctx.VirtualMachineContext,
	virtualDisks object.VirtualDeviceList) []vimtypes.BaseVirtualDeviceConfigSpec {

	advanced := vmCtx.VM.Spec.Advanced
	if advanced == nil {
		return nil
	}

	capacity := advanced.BootDiskCapacity
	if capacity == nil || capacity.IsZero() {
		return nil
	}

	// Skip resizing ISO VMs with attached CD-ROMs as their boot disks are FCDs
	// and should be managed by PVCs.
	if hw := vmCtx.VM.Spec.Hardware; hw != nil && len(hw.Cdrom) > 0 {
		return nil
	}

	// Assume the first virtual disk - if any - is the boot disk.
	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, vmDevice := range virtualDisks {
		vmDisk, ok := vmDevice.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}

		// Maybe don't allow shrink?
		if vmDisk.CapacityInBytes != capacity.Value() {
			vmDisk.CapacityInBytes = capacity.Value()
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
				Device:    vmDisk,
			})
		}

		break
	}

	return deviceChanges
}
