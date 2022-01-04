// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/pointer"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

const (
	// A negative device range is traditionally used.
	pciDevicesStartDeviceKey      = int32(-200)
	instanceStorageStartDeviceKey = int32(-300)
)

func CreatePCIPassThroughDevice(deviceKey int32, backingInfo vimtypes.BaseVirtualDeviceBackingInfo) vimtypes.BaseVirtualDevice {
	device := &vimtypes.VirtualPCIPassthrough{
		VirtualDevice: vimtypes.VirtualDevice{
			Key:     deviceKey,
			Backing: backingInfo,
		},
	}
	return device
}

func CreatePCIDevices(pciDevices vmopv1alpha1.VirtualDevices) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, 0,
		len(pciDevices.VGPUDevices)+len(pciDevices.DynamicDirectPathIODevices))
	deviceKey := pciDevicesStartDeviceKey

	for _, vGPU := range pciDevices.VGPUDevices {
		backingInfo := &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
			Vgpu: vGPU.ProfileName,
		}
		vGPUDevice := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		devices = append(devices, vGPUDevice)
		deviceKey--
	}

	for _, dynamicDirectPath := range pciDevices.DynamicDirectPathIODevices {
		allowedDev := vimtypes.VirtualPCIPassthroughAllowedDevice{
			VendorId: int32(dynamicDirectPath.VendorID),
			DeviceId: int32(dynamicDirectPath.DeviceID),
		}
		backingInfo := &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
			AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{allowedDev},
			CustomLabel:   dynamicDirectPath.CustomLabel,
		}
		dynamicDirectPathDevice := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		devices = append(devices, dynamicDirectPathDevice)
		deviceKey--
	}

	return devices
}

func CreateInstanceStorageDiskDevices(isVolumes []vmopv1alpha1.VirtualMachineVolume) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, 0, len(isVolumes))
	deviceKey := instanceStorageStartDeviceKey

	for _, volume := range isVolumes {
		device := &vimtypes.VirtualDisk{
			CapacityInBytes: volume.PersistentVolumeClaim.InstanceVolumeClaim.Size.Value(),
			VirtualDevice: vimtypes.VirtualDevice{
				Key: deviceKey,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: pointer.Bool(false),
				},
			},
			VDiskId: &vimtypes.ID{
				Id: constants.InstanceStorageVDiskID,
			},
		}
		devices = append(devices, device)
		deviceKey--
	}

	return devices
}
