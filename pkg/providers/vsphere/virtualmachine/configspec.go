// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/ptr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// CreateConfigSpec returns an initial ConfigSpec that is created by overlaying the
// base ConfigSpec with VM Class spec and other arguments.
// TODO: We eventually need to de-dupe much of this with the ConfigSpec manipulation that's later done
// in the "update" pre-power on path. That operates on a ConfigInfo so we'd need to populate that from
// the config we build here.
func CreateConfigSpec(
	vmCtx context.VirtualMachineContext,
	configSpec types.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec,
	vmImageStatus *vmopv1.VirtualMachineImageStatus,
	minFreq uint64) types.VirtualMachineConfigSpec {

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

	hardwareVersion := determineHardwareVersion(vmCtx.VM, &configSpec, vmImageStatus)
	if hardwareVersion.IsValid() {
		configSpec.Version = hardwareVersion.String()
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

	if advanced := vmCtx.VM.Spec.Advanced; advanced != nil && advanced.ChangeBlockTracking {
		configSpec.ChangeTrackingEnabled = ptr.To(true)
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

	return configSpec
}

func determineHardwareVersion(
	vm *vmopv1.VirtualMachine,
	configSpec *types.VirtualMachineConfigSpec,
	vmImageStatus *vmopv1.VirtualMachineImageStatus) types.HardwareVersion {

	vmMinVersion := types.HardwareVersion(vm.Spec.MinHardwareVersion)

	var configSpecVersion types.HardwareVersion
	if configSpec.Version != "" {
		configSpecVersion, _ = types.ParseHardwareVersion(configSpec.Version)
	}

	if configSpecVersion.IsValid() {
		if vmMinVersion <= configSpecVersion {
			// No update needed.
			return 0
		}

		return vmMinVersion
	}

	// A VM Class with an embedded ConfigSpec should have the version set, so
	// this is a ConfigSpec we created from the HW devices in the class. If the
	// image's version is too old to support passthrough devices or PVCs if
	// configured, bump the version so those devices will work.
	var imageVersion types.HardwareVersion
	if vmImageStatus != nil && vmImageStatus.HardwareVersion != nil {
		imageVersion = types.HardwareVersion(*vmImageStatus.HardwareVersion)
	}

	var minVerFromDevs types.HardwareVersion
	if util.HasVirtualPCIPassthroughDeviceChange(configSpec.DeviceChange) {
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForPCIPassthruDevices)
	} else if hasPVC(vm) {
		// This only catches volumes set at VM create time.
		minVerFromDevs = max(imageVersion, constants.MinSupportedHWVersionForPVC)
	}

	// If both are zero, VC will use the cluster's default version.
	return max(vmMinVersion, minVerFromDevs)
}

// CreateConfigSpecForPlacement creates a ConfigSpec that is suitable for
// Placement. configSpec will likely be - or at least derived from - the
// ConfigSpec returned by CreateConfigSpec above.
func CreateConfigSpecForPlacement(
	vmCtx context.VirtualMachineContext,
	configSpec types.VirtualMachineConfigSpec,
	storageClassesToIDs map[string]string) types.VirtualMachineConfigSpec {

	deviceChangeCopy := make([]types.BaseVirtualDeviceConfigSpec, 0, len(configSpec.DeviceChange))
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
	configSpec.DeviceChange = append(configSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
		Operation:     types.VirtualDeviceConfigSpecOperationAdd,
		FileOperation: types.VirtualDeviceConfigSpecFileOperationCreate,
		Device: &types.VirtualDisk{
			CapacityInBytes: 1024 * 1024,
			VirtualDevice: types.VirtualDevice{
				Key: -42,
				Backing: &types.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: ptr.To(true),
				},
			},
		},
		Profile: []types.BaseVirtualMachineProfileSpec{
			&types.VirtualMachineDefinedProfileSpec{
				ProfileId: storageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			},
		},
	})

	if pkgconfig.FromContext(vmCtx).Features.InstanceStorage {
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

	return configSpec
}

// ConfigSpecFromVMClassDevices creates a ConfigSpec that adds the standalone hardware devices from
// the VMClass if any. This ConfigSpec will be used as the class ConfigSpec to CreateConfigSpec, with
// the rest of the class fields - like CPU count - applied on top.
func ConfigSpecFromVMClassDevices(vmClassSpec *vmopv1.VirtualMachineClassSpec) types.VirtualMachineConfigSpec {
	devsFromClass := CreatePCIDevicesFromVMClass(vmClassSpec.Hardware.Devices)
	if len(devsFromClass) == 0 {
		return types.VirtualMachineConfigSpec{}
	}

	var configSpec types.VirtualMachineConfigSpec
	for _, dev := range devsFromClass {
		configSpec.DeviceChange = append(configSpec.DeviceChange, &types.VirtualDeviceConfigSpec{
			Operation: types.VirtualDeviceConfigSpecOperationAdd,
			Device:    dev,
		})
	}
	return configSpec
}

func hasPVC(vm *vmopv1.VirtualMachine) bool {
	for i := range vm.Spec.Volumes {
		if vm.Spec.Volumes[i].PersistentVolumeClaim != nil {
			return true
		}
	}
	return false
}
