// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimTypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/utils/pointer"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/util"
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

func CreatePCIDevices(
	pciDevicesFromVMClass v1alpha1.VirtualDevices,
	pciDevsFromConfigSpec []*vimTypes.VirtualPCIPassthrough) []vimTypes.BaseVirtualDevice {

	devices := make([]vimTypes.BaseVirtualDevice, 0,
		len(pciDevicesFromVMClass.VGPUDevices)+
			len(pciDevicesFromVMClass.DynamicDirectPathIODevices)+
			len(pciDevsFromConfigSpec))

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

	// Append PCI devices from the VM Class ConfigSpec.
	for i := range pciDevsFromConfigSpec {
		dev := pciDevsFromConfigSpec[i]

		// TODO(akutz) This check will be removed at some point once we
		//             more than just vGPU devices via the ConfigSpec.
		if util.IsDeviceVGPU(dev) {
			dev.Key = deviceKey
			devices = append(devices, dev)
			deviceKey--
		}
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
