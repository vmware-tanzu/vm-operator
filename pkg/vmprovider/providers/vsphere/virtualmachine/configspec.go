// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"bytes"
	"reflect"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/instancestorage"
)

func CreateConfigSpec(
	name string,
	vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	minFreq uint64) *vimtypes.VirtualMachineConfigSpec {

	configSpec := &vimtypes.VirtualMachineConfigSpec{
		Name:       name,
		Annotation: constants.VCVMAnnotation,
		NumCPUs:    int32(vmClassSpec.Hardware.Cpus),
		MemoryMB:   MemoryQuantityToMb(vmClassSpec.Hardware.Memory),
		// Enable clients to differentiate the managed VMs from the regular VMs.
		ManagedBy: &vimtypes.ManagedByInfo{
			ExtensionKey: "com.vmware.vcenter.wcp",
			Type:         "VirtualMachine",
		},
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
	storageClassesToIDs map[string]string) *vimtypes.VirtualMachineConfigSpec {

	configSpec := CreateConfigSpec(vmCtx.VM.Name, vmClassSpec, minFreq)

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

	for _, dev := range CreatePCIDevices(vmClassSpec.Hardware.Devices) {
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

// MarshalConfigSpec returns a serialized XML byte array when provided with a VirtualMachineConfigSpec.
func MarshalConfigSpec(configSpec vimtypes.VirtualMachineConfigSpec) ([]byte, error) {
	start := xml.StartElement{
		Name: xml.Name{
			Local: "obj",
		},
		Attr: []xml.Attr{
			{
				Name:  xml.Name{Local: "xmlns:" + vim25.Namespace},
				Value: "urn:" + vim25.Namespace,
			},
			{
				Name:  xml.Name{Local: "xmlns:xsi"},
				Value: constants.XsiNamespace,
			},
			{
				Name:  xml.Name{Local: "xsi:type"},
				Value: vim25.Namespace + ":" + reflect.TypeOf(configSpec).Name(),
			},
		},
	}
	specWriter := new(bytes.Buffer)
	enc := xml.NewEncoder(specWriter)
	err := enc.EncodeElement(configSpec, start)
	if err != nil {
		return []byte{}, err
	}

	return specWriter.Bytes(), nil
}
