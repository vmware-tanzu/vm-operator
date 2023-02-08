// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
)

const (
	// A negative device range is traditionally used.
	pciDevicesStartDeviceKey      = int32(-200)
	instanceStorageStartDeviceKey = int32(-300)
)

func CreatePCIPassThroughDevice(deviceKey int32, backingInfo vimTypes.BaseVirtualDeviceBackingInfo) vimTypes.BaseVirtualDevice {
	device := &vimTypes.VirtualPCIPassthrough{
		VirtualDevice: vimTypes.VirtualDevice{
			Key:     deviceKey,
			Backing: backingInfo,
		},
	}
	return device
}

// CreatePCIDevicesFromConfigSpec creates vim25 VirtualDevices from the specified list of PCI devices from the VM Class ConfigSpec.
func CreatePCIDevicesFromConfigSpec(pciDevsFromConfigSpec []*vimTypes.VirtualPCIPassthrough) []vimTypes.BaseVirtualDevice {
	devices := make([]vimTypes.BaseVirtualDevice, 0, len(pciDevsFromConfigSpec))

	deviceKey := pciDevicesStartDeviceKey

	for i := range pciDevsFromConfigSpec {
		dev := pciDevsFromConfigSpec[i]
		dev.Key = deviceKey
		devices = append(devices, dev)
		deviceKey--
	}

	return devices
}

// CreatePCIDevicesFromVMClass creates vim25 VirtualDevices from the specified list of PCI devices from VM Class spec.
func CreatePCIDevicesFromVMClass(pciDevicesFromVMClass v1alpha1.VirtualDevices) []vimTypes.BaseVirtualDevice {
	devices := make([]vimTypes.BaseVirtualDevice, 0, len(pciDevicesFromVMClass.VGPUDevices)+len(pciDevicesFromVMClass.DynamicDirectPathIODevices))

	deviceKey := pciDevicesStartDeviceKey

	for _, vGPU := range pciDevicesFromVMClass.VGPUDevices {
		backingInfo := &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{
			Vgpu: vGPU.ProfileName,
		}
		dev := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		devices = append(devices, dev)
		deviceKey--
	}

	for _, dynamicDirectPath := range pciDevicesFromVMClass.DynamicDirectPathIODevices {
		allowedDev := vimTypes.VirtualPCIPassthroughAllowedDevice{
			VendorId: int32(dynamicDirectPath.VendorID),
			DeviceId: int32(dynamicDirectPath.DeviceID),
		}
		backingInfo := &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{
			AllowedDevice: []vimTypes.VirtualPCIPassthroughAllowedDevice{allowedDev},
			CustomLabel:   dynamicDirectPath.CustomLabel,
		}
		dev := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		devices = append(devices, dev)
		deviceKey--
	}

	return devices
}

func CreateInstanceStorageDiskDevices(isVolumes []v1alpha1.VirtualMachineVolume) []vimTypes.BaseVirtualDevice {
	devices := make([]vimTypes.BaseVirtualDevice, 0, len(isVolumes))
	deviceKey := instanceStorageStartDeviceKey

	for _, volume := range isVolumes {
		device := &vimTypes.VirtualDisk{
			CapacityInBytes: volume.PersistentVolumeClaim.InstanceVolumeClaim.Size.Value(),
			VirtualDevice: vimTypes.VirtualDevice{
				Key: deviceKey,
				Backing: &vimTypes.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: pointer.Bool(false),
				},
			},
			VDiskId: &vimTypes.ID{
				Id: constants.InstanceStorageVDiskID,
			},
		}
		devices = append(devices, device)
		deviceKey--
	}

	return devices
}
