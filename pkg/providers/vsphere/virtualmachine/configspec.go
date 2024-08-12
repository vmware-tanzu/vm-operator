// Copyright (c) 2022-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// CreateConfigSpec returns an initial ConfigSpec that is created by overlaying the
// base ConfigSpec with VM Class spec and other arguments.
// TODO: We eventually need to de-dupe much of this with the ConfigSpec manipulation that's later done
// in the "update" pre-power on path. That operates on a ConfigInfo so we'd need to populate that from
// the config we build here.
func CreateConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec,
	vmClassSpec vmopv1.VirtualMachineClassSpec,
	vmImageStatus vmopv1.VirtualMachineImageStatus,
	minFreq uint64) vimtypes.VirtualMachineConfigSpec {

	configSpec.Name = vmCtx.VM.Name
	if configSpec.Annotation == "" {
		// If the class ConfigSpec doesn't specify any annotations, set the default one.
		configSpec.Annotation = constants.VCVMAnnotation
	}
	// CPU and Memory configurations specified in the VM Class standalone fields take
	// precedence over values in the config spec
	configSpec.NumCPUs = int32(vmClassSpec.Hardware.Cpus)
	configSpec.MemoryMB = MemoryQuantityToMb(vmClassSpec.Hardware.Memory)
	configSpec.ManagedBy = &vimtypes.ManagedByInfo{
		ExtensionKey: vmopv1.ManagedByExtensionKey,
		Type:         vmopv1.ManagedByExtensionType,
	}

	// Ensure ExtraConfig contains the name/namespace of the VM's Kubernetes
	// resource.
	configSpec.ExtraConfig = util.OptionValues(configSpec.ExtraConfig).Merge(
		&vimtypes.OptionValue{
			Key:   constants.ExtraConfigVMServiceNamespacedName,
			Value: vmCtx.VM.NamespacedName(),
		},
	)

	// spec.biosUUID is only set when creating a VM and is immutable.
	// This field should not be updated for existing VMs.
	if id := vmCtx.VM.Spec.BiosUUID; id != "" {
		configSpec.Uuid = id
	}
	// spec.instanceUUID is only set when creating a VM and is immutable.
	// This field should not be updated for existing VMs.
	if id := vmCtx.VM.Spec.InstanceUUID; id != "" {
		configSpec.InstanceUuid = id
	}

	hardwareVersion := vmopv1util.DetermineHardwareVersion(
		*vmCtx.VM, configSpec, vmImageStatus)
	if hardwareVersion.IsValid() {
		configSpec.Version = hardwareVersion.String()
	}

	if val, ok := vmCtx.VM.Annotations[constants.FirmwareOverrideAnnotation]; ok && (val == "efi" || val == "bios") {
		configSpec.Firmware = val
	} else if vmImageStatus.Firmware != "" {
		// Use the image's firmware type if present.
		// This is necessary until the vSphere UI can support creating VM Classes with
		// an empty/nil firmware type. Since VM Classes created via the vSphere UI always has
		// a non-empty firmware value set, this can cause VM boot failures.
		// TODO: Use image firmware only when the class config spec has an empty firmware type.
		configSpec.Firmware = vmImageStatus.Firmware
	}

	if advanced := vmCtx.VM.Spec.Advanced; advanced != nil && advanced.ChangeBlockTracking != nil {
		configSpec.ChangeTrackingEnabled = advanced.ChangeBlockTracking
	}

	// Populate the CPU reservation and limits in the ConfigSpec if VAPI fields specify any.
	// VM Class VAPI does not support Limits, so they will never be non nil.
	// TODO: Remove limits: issues/56
	if res := vmClassSpec.Policies.Resources; !res.Requests.Cpu.IsZero() || !res.Limits.Cpu.IsZero() {
		// TODO: Always override?
		configSpec.CpuAllocation = &vimtypes.ResourceAllocationInfo{
			Shares: &vimtypes.SharesInfo{
				Level: vimtypes.SharesLevelNormal,
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
	} else if configSpec.CpuAllocation == nil {
		// Default to best effort.
		configSpec.CpuAllocation = &vimtypes.ResourceAllocationInfo{
			Shares: &vimtypes.SharesInfo{
				Level: vimtypes.SharesLevelNormal,
			},
			Reservation: ptr.To[int64](0),
			Limit:       ptr.To[int64](-1),
		}
	}

	// Populate the memory reservation and limits in the ConfigSpec if VAPI fields specify any.
	// TODO: Remove limits: issues/56
	if res := vmClassSpec.Policies.Resources; !res.Requests.Memory.IsZero() || !res.Limits.Memory.IsZero() {
		// TODO: Always override?
		configSpec.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
			Shares: &vimtypes.SharesInfo{
				Level: vimtypes.SharesLevelNormal,
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
	} else if configSpec.MemoryAllocation == nil {
		// Default to best effort.
		configSpec.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
			Shares: &vimtypes.SharesInfo{
				Level: vimtypes.SharesLevelNormal,
			},
			Reservation: ptr.To[int64](0),
			Limit:       ptr.To[int64](-1),
		}
	}

	// If VM Spec guestID is specified, initially set the guest ID in ConfigSpec to ensure VM is created with the expected guest ID.
	// Afterwards, only update it if the VM spec guest ID differs from the VM's existing ConfigInfo.
	if guestID := vmCtx.VM.Spec.GuestID; guestID != "" {
		configSpec.GuestId = guestID
	}

	return configSpec
}

// CreateConfigSpecForPlacement creates a ConfigSpec that is suitable for
// Placement. configSpec will likely be - or at least derived from - the
// ConfigSpec returned by CreateConfigSpec above.
func CreateConfigSpecForPlacement(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec,
	storageClassesToIDs map[string]string) (vimtypes.VirtualMachineConfigSpec, error) {

	deviceChangeCopy := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(configSpec.DeviceChange))
	for _, devChange := range configSpec.DeviceChange {
		if spec := devChange.GetVirtualDeviceConfigSpec(); spec != nil {
			// VC PlaceVmsXCluster() has issues when the ConfigSpec has EthCards so return to the
			// prior status quo until those issues get sorted out.
			if util.IsEthernetCard(spec.Device) {
				continue
			}
		}
		deviceChangeCopy = append(deviceChangeCopy, devChange)
	}

	configSpec.DeviceChange = deviceChangeCopy

	// Add a dummy disk for placement: PlaceVmsXCluster expects there to always be at least one disk.
	// Until we're in a position to have the OVF envelope here, add a dummy disk satisfy it.
	// Note: UnitNumber is required for PlaceVmsXCluster w/ VpxdVmxGeneration fss enabled.
	// PlaceVmsXCluster is not used with Instance Storage, so UnitNumber is not required for volumes below.
	configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
		Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
		FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
		Device: &vimtypes.VirtualDisk{
			CapacityInBytes: 1024 * 1024,
			VirtualDevice: vimtypes.VirtualDevice{
				Key:        -42,
				UnitNumber: ptr.To[int32](0),
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: ptr.To(true),
				},
			},
		},
		Profile: []vimtypes.BaseVirtualMachineProfileSpec{
			&vimtypes.VirtualMachineDefinedProfileSpec{
				ProfileId: storageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			},
		},
	})

	if pkgcfg.FromContext(vmCtx).Features.InstanceStorage {
		isVolumes := instancestorage.FilterVolumes(vmCtx.VM)

		for idx, dev := range CreateInstanceStorageDiskDevices(isVolumes) {
			configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
				Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
				FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
				Device:        dev,
				Profile: []vimtypes.BaseVirtualMachineProfileSpec{
					&vimtypes.VirtualMachineDefinedProfileSpec{
						ProfileId: storageClassesToIDs[isVolumes[idx].PersistentVolumeClaim.InstanceVolumeClaim.StorageClass],
						ProfileData: &vimtypes.VirtualMachineProfileRawData{
							ExtensionKey: "com.vmware.vim.sps",
						},
					},
				},
			})
		}
	}

	if err := util.EnsureDisksHaveControllers(&configSpec); err != nil {
		return vimtypes.VirtualMachineConfigSpec{}, err
	}

	// TODO: Add more devices and fields
	//  - boot disks from OVA
	//  - storage profile/class
	//  - PVC volumes
	//  - Network devices (meh for now b/c of wcp constraints)
	//  - anything in ExtraConfig matter here?
	//  - any way to do the cluster modules for anti-affinity?
	//  - whatever else I'm forgetting

	return configSpec, nil
}

// ConfigSpecFromVMClassDevices creates a ConfigSpec that adds the standalone hardware devices from
// the VMClass if any. This ConfigSpec will be used as the class ConfigSpec to CreateConfigSpec, with
// the rest of the class fields - like CPU count - applied on top.
func ConfigSpecFromVMClassDevices(vmClassSpec *vmopv1.VirtualMachineClassSpec) vimtypes.VirtualMachineConfigSpec {
	devsFromClass := CreatePCIDevicesFromVMClass(vmClassSpec.Hardware.Devices)
	if len(devsFromClass) == 0 {
		return vimtypes.VirtualMachineConfigSpec{}
	}

	var configSpec vimtypes.VirtualMachineConfigSpec
	for _, dev := range devsFromClass {
		configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    dev,
		})
	}
	return configSpec
}
