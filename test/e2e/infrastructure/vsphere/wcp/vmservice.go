// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package wcp

import (
	"github.com/vmware/govmomi/vim25/types"
)

// VGPUDevice contains the configuration corresponding to a vGPU device.
type VGPUDevice struct {
	ProfileName string `json:"profile_name"`
}

// DynamicDirectPathIODevice contains the configuration corresponding to a Dynamic DirectPath I/O device.
type DynamicDirectPathIODevice struct {
	VendorID    int    `json:"vendor_id"`
	DeviceID    int    `json:"device_id"`
	CustomLabel string `json:"custom_label,omitempty"`
}

// InstanceStorageVolume contains instance volume associated with VirtualMachineClass.
type InstanceStorageVolume struct {
	Size int `json:"size"`
}

// InstanceStorage contains instance storage configurations associated with VirtualMachineClass.
type InstanceStorage struct {
	StoragePolicy string                  `json:"storage_policy,omitempty"`
	Volumes       []InstanceStorageVolume `json:"volumes,omitempty"`
}

// VirtualDevices contains information about the virtual devices associated with a VirtualMachineClass.
type VirtualDevices struct {
	VGPUDevices                []VGPUDevice                `json:"vgpu_devices,omitempty"`
	DynamicDirectPathIODevices []DynamicDirectPathIODevice `json:"dynamic_direct_path_IO_devices,omitempty"`
}

// VMClassSpec represents the input structure of a VMClass required to create/update VMClass in VC.
type VMClassSpec struct {
	ID                string                          `json:"id"`
	CPUCount          *int                            `json:"cpu_count,omitempty"`
	MemoryMB          *int                            `json:"memory_mb,omitempty"`
	CPUReservation    *int                            `json:"cpu_reservation,omitempty"`
	MemoryReservation *int                            `json:"memory_reservation,omitempty"`
	Description       *string                         `json:"description,omitempty"`
	Devices           VirtualDevices                  `json:"devices,omitempty"`
	InstanceStorage   InstanceStorage                 `json:"instance_storage,omitempty"`
	ConfigSpec        *types.VirtualMachineConfigSpec `json:"config_spec,omitempty"`
}

// VMClassInfo represents the output structure.
type VMClassInfo struct {
	VMClassSpec

	VMs          []string `json:"vms"`
	Namespaces   []string `json:"namespaces"`
	ConfigStatus string   `json:"config_status"`
}

// NamespaceUpdateVMserviceSpec represent structure for only VMService specs required to update namespace instance.
type NamespaceUpdateVMserviceSpec struct {
	VMClasses        *[]string `json:"vmClasses,omitempty"`
	ContentLibraries *[]string `json:"contentLibraries,omitempty"`
}

type DeviceBacking struct {
	VMDKFile string `json:"vmdk_file"`
}

type DeviceInfo struct {
	Backing  DeviceBacking `json:"backing"`
	Capacity int64         `json:"capacity"`
}

type VirtualMachineDetails struct {
	Disks map[string]DeviceInfo `json:"disks"`
}
