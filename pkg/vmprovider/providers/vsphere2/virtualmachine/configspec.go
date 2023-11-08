// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/pointer"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/instancestorage"
)

// CreateConfigSpec returns an initial ConfigSpec that is created by overlaying the
// base ConfigSpec with VM Class spec and other arguments.
// TODO: We eventually need to de-dupe much of this with the ConfigSpec manipulation that's later done
// in the "update" pre-power on path. That operates on a ConfigInfo so we'd need to populate that from
// the config we build here.
func CreateConfigSpec(
	vmCtx context.VirtualMachineContextA2,
	vmClassConfigSpec *types.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec,
	vmImageStatus *vmopv1.VirtualMachineImageStatus,
	minFreq uint64) *types.VirtualMachineConfigSpec {

	configSpec := types.VirtualMachineConfigSpec{}

	// If there is a class ConfigSpec, then that is our initial ConfigSpec.
	if vmClassConfigSpec != nil {
		configSpec = *vmClassConfigSpec
	}

	configSpec.Name = vmCtx.VM.Name
	if configSpec.Annotation == "" {
		// If the class ConfigSpec doesn't specify any annotations, set the default one.
		configSpec.Annotation = constants.VCVMAnnotation
	}
	// CPU and Memory configurations specified in the VM Class standalone fields take
	// precedence over values in the config spec
	configSpec.NumCPUs = int32(vmClassSpec.Hardware.Cpus)
	configSpec.MemoryMB = MemoryQuantityToMb(vmClassSpec.Hardware.Memory)
	configSpec.ManagedBy = &types.ManagedByInfo{
		ExtensionKey: vmopv1.ManagedByExtensionKey,
		Type:         vmopv1.ManagedByExtensionType,
	}

	if val, ok := vmCtx.VM.Annotations[constants.FirmwareOverrideAnnotation]; ok && (val == "efi" || val == "bios") {
		configSpec.Firmware = val
	} else if vmImageStatus != nil && vmImageStatus.Firmware != "" {
		// Use the image's firmware type if present.
		// This is necessary until the vSphere UI can support creating VM Classes with
		// an empty/nil firmware type. Since VM Classes created via the vSphere UI always has
		// a non-empty firmware value set, this can cause VM boot failures.
		// TODO: Use image firmware only when the class config spec has an empty firmware type.
		configSpec.Firmware = vmImageStatus.Firmware
	}

	// TODO: Otherwise leave as-is? Our ChangeBlockTracking could be better as a *bool.
	if vmCtx.VM.Spec.Advanced.ChangeBlockTracking {
		configSpec.ChangeTrackingEnabled = pointer.Bool(true)
	}

	// Populate the CPU reservation and limits in the ConfigSpec if VAPI fields specify any.
	// VM Class VAPI does not support Limits, so they will never be non nil.
	// TODO: Remove limits: issues/56
	if res := vmClassSpec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		// TODO: Always override?
		configSpec.CpuAllocation = &types.ResourceAllocationInfo{
			Shares: &types.SharesInfo{
				Level: types.SharesLevelNormal,
			},
		}

		if !res.Requests.Cpu.IsZero() {
			rsv := CPUQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minFreq)
			configSpec.CpuAllocation.Reservation = &rsv
		}
		if !res.Limits.Cpu.IsZero() {
			lim := CPUQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minFreq)
			configSpec.CpuAllocation.Limit = &lim
		}
	}

	// Populate the memory reservation and limits in the ConfigSpec if VAPI fields specify any.
	// TODO: Remove limits: issues/56
	if res := vmClassSpec.Policies.Resources; !res.Requests.Memory.IsZero() || !res.Limits.Memory.IsZero() {
		// TODO: Always override?
		configSpec.MemoryAllocation = &types.ResourceAllocationInfo{
			Shares: &types.SharesInfo{
				Level: types.SharesLevelNormal,
			},
		}

		if !res.Requests.Memory.IsZero() {
			rsv := MemoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
			configSpec.MemoryAllocation.Reservation = &rsv
		}
		if !res.Limits.Memory.IsZero() {
			lim := MemoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
			configSpec.MemoryAllocation.Limit = &lim
		}
	}

	return &configSpec
}

// CreateConfigSpecForPlacement creates a ConfigSpec that is suitable for Placement.
// baseConfigSpec will likely be - or at least derived from - the ConfigSpec returned by CreateConfigSpec above.
func CreateConfigSpecForPlacement(
	vmCtx context.VirtualMachineContextA2,
	baseConfigSpec *types.VirtualMachineConfigSpec,
	storageClassesToIDs map[string]string) *types.VirtualMachineConfigSpec {

	// TODO: If placement chokes on EthCards w/o a backing yet (NSX-T) remove those entries here.
	deviceChangeCopy := make([]types.BaseVirtualDeviceConfigSpec, len(baseConfigSpec.DeviceChange))
	copy(deviceChangeCopy, baseConfigSpec.DeviceChange)

	configSpec := *baseConfigSpec
	configSpec.DeviceChange = deviceChangeCopy

	// Add a dummy disk for placement: PlaceVmsXCluster expects there to always be at least one disk.
	// Until we're in a position to have the OVF envelope here, add a dummy disk satisfy it.
	configSpec.DeviceChange = append(configSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
		Operation:     types.VirtualDeviceConfigSpecOperationAdd,
		FileOperation: types.VirtualDeviceConfigSpecFileOperationCreate,
		Device: &types.VirtualDisk{
			CapacityInBytes: 1024 * 1024,
			VirtualDevice: types.VirtualDevice{
				Key: -42,
				Backing: &types.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: pointer.Bool(true),
				},
			},
		},
		Profile: []types.BaseVirtualMachineProfileSpec{
			&types.VirtualMachineDefinedProfileSpec{
				ProfileId: storageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			},
		},
	})

	if lib.IsInstanceStorageFSSEnabled() {
		isVolumes := instancestorage.FilterVolumes(vmCtx.VM)

		for idx, dev := range CreateInstanceStorageDiskDevices(isVolumes) {
			configSpec.DeviceChange = append(configSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
				Operation:     types.VirtualDeviceConfigSpecOperationAdd,
				FileOperation: types.VirtualDeviceConfigSpecFileOperationCreate,
				Device:        dev,
				Profile: []types.BaseVirtualMachineProfileSpec{
					&types.VirtualMachineDefinedProfileSpec{
						ProfileId: storageClassesToIDs[isVolumes[idx].PersistentVolumeClaim.InstanceVolumeClaim.StorageClass],
						ProfileData: &types.VirtualMachineProfileRawData{
							ExtensionKey: "com.vmware.vim.sps",
						},
					},
				},
			})
		}
	}

	// TODO: Add more devices and fields
	//  - boot disks from OVA
	//  - storage profile/class
	//  - PVC volumes
	//  - Network devices (meh for now b/c of wcp constraints)
	//  - anything in ExtraConfig matter here?
	//  - any way to do the cluster modules for anti-affinity?
	//  - whatever else I'm forgetting

	return &configSpec
}

// ConfigSpecFromVMClassDevices creates a ConfigSpec that adds the standalone hardware devices from
// the VMClass if any. This ConfigSpec will be used as the class ConfigSpec to CreateConfigSpec, with
// the rest of the class fields - like CPU count - applied on top.
func ConfigSpecFromVMClassDevices(vmClassSpec *vmopv1.VirtualMachineClassSpec) *types.VirtualMachineConfigSpec {
	devsFromClass := CreatePCIDevicesFromVMClass(vmClassSpec.Hardware.Devices)
	if len(devsFromClass) == 0 {
		return nil
	}

	configSpec := &types.VirtualMachineConfigSpec{}
	for _, dev := range devsFromClass {
		configSpec.DeviceChange = append(configSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device:    dev,
		})
	}
	return configSpec
}
