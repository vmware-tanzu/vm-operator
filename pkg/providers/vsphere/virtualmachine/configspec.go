// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/ptr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/instancestorage"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

// CreateConfigSpec returns an initial ConfigSpec that is created by overlaying the
// base ConfigSpec with VM Class spec and other arguments.
// TODO: We eventually need to de-dupe much of this with the ConfigSpec manipulation that's later done
// in the "update" pre-power on path. That operates on a ConfigInfo so we'd need to populate that from
// the config we build here.
func CreateConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec) vimtypes.VirtualMachineConfigSpec {

	// spec.biosUUID is only set when creating a VM and is immutable.
	// These two fields should not be updated for existing VMs.
	if id := vmCtx.VM.Spec.BiosUUID; id != "" {
		configSpec.Uuid = id
		configSpec.InstanceUuid = string(vmCtx.VM.UID)
	}

	if advanced := vmCtx.VM.Spec.Advanced; advanced != nil && advanced.ChangeBlockTracking != nil {
		configSpec.ChangeTrackingEnabled = advanced.ChangeBlockTracking
	}

	return configSpec
}

// CreateConfigSpecForPlacement creates a ConfigSpec that is suitable for
// Placement. configSpec will likely be - or at least derived from - the
// ConfigSpec returned by CreateConfigSpec above.
func CreateConfigSpecForPlacement(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec,
	storageClassesToIDs map[string]string) vimtypes.VirtualMachineConfigSpec {

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
	configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
		Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
		FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
		Device: &vimtypes.VirtualDisk{
			CapacityInBytes: 1024 * 1024,
			VirtualDevice: vimtypes.VirtualDevice{
				Key: -42,
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
func ConfigSpecFromVMClassDevices(vmClassSpec vmopv1.VirtualMachineClassSpec) vimtypes.VirtualMachineConfigSpec {
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
