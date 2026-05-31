// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	res "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
)

// GetVirtualDiskByUUID returns the VirtualDiskInfo for the first disk whose
// backing UUID matches diskUUID. Returns nil when no matching disk is found.
func (vs *vSphereVMProvider) GetVirtualDiskByUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	diskUUID string) (*providers.VirtualDiskInfo, error) {

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "getDiskByUUID"),
		vm,
	)

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vCenter client: %w", err)
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM %q: %w", vm.Name, err)
	}

	var moVM mo.VirtualMachine
	if err := vcVM.Properties(vmCtx, vcVM.Reference(),
		[]string{"config.hardware.device"}, &moVM); err != nil {
		return nil, fmt.Errorf("failed to fetch VM hardware devices for %q: %w",
			vm.Name, err)
	}

	for _, device := range moVM.Config.Hardware.Device {
		disk, ok := device.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}
		backing, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
		if !ok {
			continue
		}
		if backing.Uuid == diskUUID {
			unitNum := int32(0)
			if disk.UnitNumber != nil {
				unitNum = *disk.UnitNumber
			}
			return &providers.VirtualDiskInfo{
				DiskUUID:      backing.Uuid,
				DiskPath:      backing.FileName,
				ControllerKey: disk.ControllerKey,
				UnitNumber:    unitNum,
			}, nil
		}
	}
	return nil, nil
}

// AddExistingDiskToVM attaches an already-existing VMDK to the VM using a
// ReconfigVM_Task. The FileOperation is left unset so vSphere does not try
// to create a new backing file — the VMDK already exists on the datastore.
func (vs *vSphereVMProvider) AddExistingDiskToVM(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	diskPath string,
	controllerKey, unitNumber int32,
	diskMode string) error {

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "addExistingDisk"),
		vm,
	)
	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.Info("Adding existing disk to VM",
		"diskPath", diskPath,
		"controllerKey", controllerKey,
		"unitNumber", unitNumber,
		"diskMode", diskMode)

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return fmt.Errorf("failed to get vCenter client: %w", err)
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return fmt.Errorf("failed to get VM %q: %w", vm.Name, err)
	}

	disk := &vimtypes.VirtualDisk{
		VirtualDevice: vimtypes.VirtualDevice{
			ControllerKey: controllerKey,
			UnitNumber:    &unitNumber,
			Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
				VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
					FileName: diskPath,
				},
				DiskMode: diskMode,
			},
		},
	}

	configSpec := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				// FileOperation is intentionally omitted: the disk file
				// already exists on the datastore.
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    disk,
			},
		},
	}

	resVM := res.NewVMFromObject(vcVM)
	if _, err := resVM.Reconfigure(vmCtx, &configSpec); err != nil {
		return fmt.Errorf("ReconfigVM to add disk %q to VM %q failed: %w",
			diskPath, vm.Name, err)
	}

	logger.Info("Successfully added disk to VM",
		"diskPath", diskPath,
		"vmName", vm.Name)
	return nil
}

// RemoveDiskFromVM removes the disk identified by diskUUID from the VM using
// a ReconfigVM_Task. FileOperation is intentionally omitted so that the
// backing VMDK file is preserved on the datastore.
func (vs *vSphereVMProvider) RemoveDiskFromVM(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	diskUUID string) error {

	vmCtx := pkgctx.NewVirtualMachineContext(
		pkgctx.WithVCOpID(ctx, vm, "removeDisk"),
		vm,
	)
	logger := pkglog.FromContextOrDefault(vmCtx)
	logger.Info("Removing disk from VM", "diskUUID", diskUUID, "vmName", vm.Name)

	diskInfo, err := vs.GetVirtualDiskByUUID(ctx, vm, diskUUID)
	if err != nil {
		return fmt.Errorf("failed to look up disk %q on VM %q: %w",
			diskUUID, vm.Name, err)
	}
	if diskInfo == nil {
		// Disk is already absent — idempotent success.
		logger.Info("Disk not found on VM, treating removal as success",
			"diskUUID", diskUUID, "vmName", vm.Name)
		return nil
	}

	client, err := vs.getVcClient(vmCtx)
	if err != nil {
		return fmt.Errorf("failed to get vCenter client: %w", err)
	}

	vcVM, err := vs.getVM(vmCtx, client, true)
	if err != nil {
		return fmt.Errorf("failed to get VM %q: %w", vm.Name, err)
	}

	// Fetch the full device object by re-reading hardware to get the proper
	// device object reference needed for ReconfigVM remove.
	var moVM mo.VirtualMachine
	if err := vcVM.Properties(vmCtx, vcVM.Reference(),
		[]string{"config.hardware.device"}, &moVM); err != nil {
		return fmt.Errorf("failed to fetch VM hardware devices for %q: %w",
			vm.Name, err)
	}

	var targetDisk vimtypes.BaseVirtualDevice
	for _, device := range moVM.Config.Hardware.Device {
		disk, ok := device.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}
		backing, ok := disk.Backing.(*vimtypes.VirtualDiskFlatVer2BackingInfo)
		if !ok {
			continue
		}
		if backing.Uuid == diskUUID {
			targetDisk = disk
			break
		}
	}

	if targetDisk == nil {
		// Re-check: disk disappeared between the first lookup and now.
		logger.Info("Disk not found on VM during removal, treating as success",
			"diskUUID", diskUUID, "vmName", vm.Name)
		return nil
	}

	configSpec := vimtypes.VirtualMachineConfigSpec{
		DeviceChange: []vimtypes.BaseVirtualDeviceConfigSpec{
			&vimtypes.VirtualDeviceConfigSpec{
				// FileOperation is intentionally omitted: we preserve the
				// VMDK file on the datastore.
				Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
				Device:    targetDisk,
			},
		},
	}

	resVM := res.NewVMFromObject(vcVM)
	if _, err := resVM.Reconfigure(vmCtx, &configSpec); err != nil {
		return fmt.Errorf("ReconfigVM to remove disk %q from VM %q failed: %w",
			diskUUID, vm.Name, err)
	}

	logger.Info("Successfully removed disk from VM",
		"diskUUID", diskUUID, "vmName", vm.Name)
	return nil
}
