// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
)

func CreateConfigSpec(
	name string,
	vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	minFreq uint64,
	vmClassConfigSpec *vimtypes.VirtualMachineConfigSpec) *vimtypes.VirtualMachineConfigSpec {

	var configSpec *vimtypes.VirtualMachineConfigSpec
	if vmClassConfigSpec != nil {
		// Use VMClass ConfigSpec as the initial ConfigSpec.
		t := *vmClassConfigSpec
		configSpec = &t
	} else {
		configSpec = &vimtypes.VirtualMachineConfigSpec{}
	}

	configSpec.Name = name
	configSpec.Annotation = constants.VCVMAnnotation
	// CPU and Memory configurations specified in the VM Class spec.hardware
	// takes precedence over values in the config spec
	// TODO: might need revisiting to conform on a single way of consuming cpu and memory.
	//  we prefer the vm class CR to have consistent values to reduce confusion.
	configSpec.NumCPUs = int32(vmClassSpec.Hardware.Cpus)
	configSpec.MemoryMB = MemoryQuantityToMb(vmClassSpec.Hardware.Memory)

	// TODO: add constants for this key.
	configSpec.ManagedBy = &vimtypes.ManagedByInfo{
		ExtensionKey: "com.vmware.vcenter.wcp",
		Type:         "VirtualMachine",
	}

	configSpec.CpuAllocation = &vimtypes.ResourceAllocationInfo{
		Shares: &vimtypes.SharesInfo{
			Level: vimtypes.SharesLevelNormal,
		},
	}

	if minFreq != 0 {
		if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
			rsv := CPUQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minFreq)
			configSpec.CpuAllocation.Reservation = &rsv
		}

		if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
			lim := CPUQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minFreq)
			configSpec.CpuAllocation.Limit = &lim
		}
	}

	configSpec.MemoryAllocation = &vimtypes.ResourceAllocationInfo{
		Shares: &vimtypes.SharesInfo{
			Level: vimtypes.SharesLevelNormal,
		},
	}

	if !vmClassSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := MemoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
		configSpec.MemoryAllocation.Reservation = &rsv
	}

	if !vmClassSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := MemoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
		configSpec.MemoryAllocation.Limit = &lim
	}

	return configSpec
}

// CreateConfigSpecForPlacement creates a ConfigSpec to use for placement. Once CL deploy can accept
// a ConfigSpec, this should largely - or ideally entirely - be folded into CreateConfigSpec() above.
func CreateConfigSpecForPlacement(
	vmCtx context.VirtualMachineContext,
	vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	minFreq uint64,
	storageClassesToIDs map[string]string,
	vmClassConfigSpec *vimtypes.VirtualMachineConfigSpec) *vimtypes.VirtualMachineConfigSpec {

	configSpec := CreateConfigSpec(vmCtx.VM.Name, vmClassSpec, minFreq, vmClassConfigSpec)

	// Add a dummy disk for placement: PlaceVmsXCluster expects there to always be at least one disk.
	// Until we're in a position to have the OVF envelope here, add a dummy disk satisfy it.
	configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
		Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
		FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
		Device: &vimtypes.VirtualDisk{
			CapacityInBytes: 1024 * 1024,
			VirtualDevice: vimtypes.VirtualDevice{
				Key: -42,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: pointer.Bool(true),
				},
			},
		},
		Profile: []vimtypes.BaseVirtualMachineProfileSpec{
			&vimtypes.VirtualMachineDefinedProfileSpec{
				ProfileId: storageClassesToIDs[vmCtx.VM.Spec.StorageClass],
			},
		},
	})

	for _, dev := range CreatePCIDevices(vmClassSpec.Hardware.Devices, nil) {
		configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    dev,
		})
	}

	if lib.IsInstanceStorageFSSEnabled() {
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
