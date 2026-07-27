// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	// A negative device range is traditionally used.
	pciDevicesStartDeviceKey      = int32(-200)
	instanceStorageStartDeviceKey = int32(-300)
	hostLocalStartDeviceKey       = int32(-1000)
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
			VendorId: int32(dynamicDirectPath.VendorID), //nolint:gosec // disable G115
			DeviceId: int32(dynamicDirectPath.DeviceID), //nolint:gosec // disable G115
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

// CreateHostLocalPlacementDiskDevices returns placement-only phantom disks,
// one per given PVC, sized to the PVC's requested storage capacity. These
// disks are never actually created — they exist only so DRS/SPBM's
// placement math, driven by the disk's Profile, is forced to consider each
// PVC's host-local storage policy when scoring candidate hosts. Unlike
// instance storage disks, no magic VDiskId marker is needed.
func CreateHostLocalPlacementDiskDevices(pvcs []corev1.PersistentVolumeClaim) []vimtypes.BaseVirtualDevice {
	devices := make([]vimtypes.BaseVirtualDevice, 0, len(pvcs))
	deviceKey := hostLocalStartDeviceKey

	for _, pvc := range pvcs {
		capacity := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		device := &vimtypes.VirtualDisk{
			CapacityInBytes: capacity.Value(),
			VirtualDevice: vimtypes.VirtualDevice{
				Key: deviceKey,
				Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
					ThinProvisioned: ptr.To(false),
				},
			},
		}
		devices = append(devices, device)
		deviceKey--
	}

	return devices
}
