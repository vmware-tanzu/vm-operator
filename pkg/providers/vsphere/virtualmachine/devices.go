// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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

// CreatePCIDevicesFromConfigSpec creates vim25 VirtualDevices from the specified list of PCI devices from the VM Class ConfigSpec.
func CreatePCIDevicesFromConfigSpec(pciDevsFromConfigSpec []*vimtypes.VirtualPCIPassthrough) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, 0, len(pciDevsFromConfigSpec))

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
func CreatePCIDevicesFromVMClass(pciDevicesFromVMClass vmopv1.VirtualDevices) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, 0, len(pciDevicesFromVMClass.VGPUDevices)+len(pciDevicesFromVMClass.DynamicDirectPathIODevices))

	deviceKey := pciDevicesStartDeviceKey

	for _, vGPU := range pciDevicesFromVMClass.VGPUDevices {
		backingInfo := &vimtypes.VirtualPCIPassthroughVmiopBackingInfo{
			Vgpu: vGPU.ProfileName,
		}
		dev := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		devices = append(devices, dev)
		deviceKey--
	}

	for _, dynamicDirectPath := range pciDevicesFromVMClass.DynamicDirectPathIODevices {
		allowedDev := vimtypes.VirtualPCIPassthroughAllowedDevice{
			VendorId: int32(dynamicDirectPath.VendorID),
			DeviceId: int32(dynamicDirectPath.DeviceID),
		}
		backingInfo := &vimtypes.VirtualPCIPassthroughDynamicBackingInfo{
			AllowedDevice: []vimtypes.VirtualPCIPassthroughAllowedDevice{allowedDev},
			CustomLabel:   dynamicDirectPath.CustomLabel,
		}
		dev := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		devices = append(devices, dev)
		deviceKey--
	}

	return devices
}

func CreateInstanceStorageDiskDevices(isVolumes []vmopv1.VirtualMachineVolume) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, 0, len(isVolumes))
	deviceKey := instanceStorageStartDeviceKey

	for _, volume := range isVolumes {
		device := &vimtypes.VirtualDisk{
			CapacityInBytes: volume.PersistentVolumeClaim.InstanceVolumeClaim.Size.Value(),
			VirtualDevice: vimtypes.VirtualDevice{
				Key: deviceKey,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: ptr.To(false),
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
