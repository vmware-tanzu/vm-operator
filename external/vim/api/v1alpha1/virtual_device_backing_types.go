// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "k8s.io/apimachinery/pkg/api/resource"

// VirtualDeviceBackingInfoType identifies the concrete runtime type of a
// virtual device's backing information. The values correspond to vSphere
// API class names (e.g. vim.vm.device.VirtualDisk.FlatVer2BackingInfo).
type VirtualDeviceBackingInfoType string

const (
	// CD-ROM backing types
	VirtualDeviceBackingInfoTypeCdromATAPI             VirtualDeviceBackingInfoType = "VirtualCdromAtapiBackingInfo"
	VirtualDeviceBackingInfoTypeCdromISO               VirtualDeviceBackingInfoType = "VirtualCdromIsoBackingInfo"
	VirtualDeviceBackingInfoTypeCdromPassthrough       VirtualDeviceBackingInfoType = "VirtualCdromPassthroughBackingInfo"
	VirtualDeviceBackingInfoTypeCdromRemoteATAPI       VirtualDeviceBackingInfoType = "VirtualCdromRemoteAtapiBackingInfo"
	VirtualDeviceBackingInfoTypeCdromRemotePassthrough VirtualDeviceBackingInfoType = "VirtualCdromRemotePassthroughBackingInfo"

	// Disk backing types
	VirtualDeviceBackingInfoTypeDiskFlatVer1           VirtualDeviceBackingInfoType = "VirtualDiskFlatVer1BackingInfo"
	VirtualDeviceBackingInfoTypeDiskFlatVer2           VirtualDeviceBackingInfoType = "VirtualDiskFlatVer2BackingInfo"
	VirtualDeviceBackingInfoTypeDiskLocalPMem          VirtualDeviceBackingInfoType = "VirtualDiskLocalPMemBackingInfo"
	VirtualDeviceBackingInfoTypeDiskPartitionedRawVer2 VirtualDeviceBackingInfoType = "VirtualDiskPartitionedRawDiskVer2BackingInfo"
	VirtualDeviceBackingInfoTypeDiskRawMappingVer1     VirtualDeviceBackingInfoType = "VirtualDiskRawDiskMappingVer1BackingInfo"
	VirtualDeviceBackingInfoTypeDiskRawVer2            VirtualDeviceBackingInfoType = "VirtualDiskRawDiskVer2BackingInfo"
	VirtualDeviceBackingInfoTypeDiskSeSparse           VirtualDeviceBackingInfoType = "VirtualDiskSeSparseBackingInfo"
	VirtualDeviceBackingInfoTypeDiskSparseVer1         VirtualDeviceBackingInfoType = "VirtualDiskSparseVer1BackingInfo"
	VirtualDeviceBackingInfoTypeDiskSparseVer2         VirtualDeviceBackingInfoType = "VirtualDiskSparseVer2BackingInfo"

	// Ethernet card backing types
	VirtualDeviceBackingInfoTypeEthernetCardDistributedVirtualPort VirtualDeviceBackingInfoType = "VirtualEthernetCardDistributedVirtualPortBackingInfo"
	VirtualDeviceBackingInfoTypeEthernetCardLegacyNetwork          VirtualDeviceBackingInfoType = "VirtualEthernetCardLegacyNetworkBackingInfo"
	VirtualDeviceBackingInfoTypeEthernetCardNetwork                VirtualDeviceBackingInfoType = "VirtualEthernetCardNetworkBackingInfo"
	VirtualDeviceBackingInfoTypeEthernetCardOpaqueNetwork          VirtualDeviceBackingInfoType = "VirtualEthernetCardOpaqueNetworkBackingInfo"

	// Floppy backing types
	VirtualDeviceBackingInfoTypeFloppyDevice       VirtualDeviceBackingInfoType = "VirtualFloppyDeviceBackingInfo"
	VirtualDeviceBackingInfoTypeFloppyImage        VirtualDeviceBackingInfoType = "VirtualFloppyImageBackingInfo"
	VirtualDeviceBackingInfoTypeFloppyRemoteDevice VirtualDeviceBackingInfoType = "VirtualFloppyRemoteDeviceBackingInfo"

	// NVDIMM backing type
	VirtualDeviceBackingInfoTypeNVDIMM VirtualDeviceBackingInfoType = "VirtualNVDIMMBackingInfo"

	// Parallel port backing types
	VirtualDeviceBackingInfoTypeParallelPortDevice VirtualDeviceBackingInfoType = "VirtualParallelPortDeviceBackingInfo"
	VirtualDeviceBackingInfoTypeParallelPortFile   VirtualDeviceBackingInfoType = "VirtualParallelPortFileBackingInfo"

	// PCI passthrough backing types
	VirtualDeviceBackingInfoTypePCIPassthroughDevice VirtualDeviceBackingInfoType = "VirtualPCIPassthroughDeviceBackingInfo"

	// TODO(akutz) While this type does extend VirtualDeviceBackingInfo, it is
	//             not used as a value for a VirtualDevice's backing field.
	//             Rather it is used with the VirtualSriovEthernetCard's
	//             dvxBacking field.
	// VirtualDeviceBackingInfoTypePCIPassthroughDvx     VirtualDeviceBackingInfoType = "VirtualPCIPassthroughDvxBackingInfo"

	VirtualDeviceBackingInfoTypePCIPassthroughDynamic VirtualDeviceBackingInfoType = "VirtualPCIPassthroughDynamicBackingInfo"
	VirtualDeviceBackingInfoTypePCIPassthroughPlugin  VirtualDeviceBackingInfoType = "VirtualPCIPassthroughPluginBackingInfo"
	VirtualDeviceBackingInfoTypePCIPassthroughVmiop   VirtualDeviceBackingInfoType = "VirtualPCIPassthroughVmiopBackingInfo"

	// Pointing device backing type
	VirtualDeviceBackingInfoTypePointingDeviceDevice VirtualDeviceBackingInfoType = "VirtualPointingDeviceDeviceBackingInfo"

	// Precision clock backing type
	VirtualDeviceBackingInfoTypePrecisionClockSystemClock VirtualDeviceBackingInfoType = "VirtualPrecisionClockSystemClockBackingInfo"

	// SCSI passthrough backing type
	VirtualDeviceBackingInfoTypeSCSIPassthroughDevice VirtualDeviceBackingInfoType = "VirtualSCSIPassthroughDeviceBackingInfo"

	// Serial port backing types
	VirtualDeviceBackingInfoTypeSerialPortDevice    VirtualDeviceBackingInfoType = "VirtualSerialPortDeviceBackingInfo"
	VirtualDeviceBackingInfoTypeSerialPortFile      VirtualDeviceBackingInfoType = "VirtualSerialPortFileBackingInfo"
	VirtualDeviceBackingInfoTypeSerialPortPipe      VirtualDeviceBackingInfoType = "VirtualSerialPortPipeBackingInfo"
	VirtualDeviceBackingInfoTypeSerialPortThinPrint VirtualDeviceBackingInfoType = "VirtualSerialPortThinPrintBackingInfo"
	VirtualDeviceBackingInfoTypeSerialPortURI       VirtualDeviceBackingInfoType = "VirtualSerialPortURIBackingInfo"

	// Sound card backing type
	VirtualDeviceBackingInfoTypeSoundCardDevice VirtualDeviceBackingInfoType = "VirtualSoundCardDeviceBackingInfo"

	// SR-IOV Ethernet card backing type
	// TODO(akutz) While this type does extend VirtualDeviceBackingInfo, it is
	//             not used as a value for a VirtualDevice's backing field.
	//             Rather it is used with the VirtualSriovEthernetCard's
	//             sriovBacking field.
	//VirtualDeviceBackingInfoTypeSriovEthernetCardSriov VirtualDeviceBackingInfoType = "VirtualSriovEthernetCardSriovBackingInfo"

	// USB backing types
	VirtualDeviceBackingInfoTypeUSBRemoteClient VirtualDeviceBackingInfoType = "VirtualUSBRemoteClientBackingInfo"
	VirtualDeviceBackingInfoTypeUSBRemoteHost   VirtualDeviceBackingInfoType = "VirtualUSBRemoteHostBackingInfo"
	VirtualDeviceBackingInfoTypeUSBUSB          VirtualDeviceBackingInfoType = "VirtualUSBUSBBackingInfo"
)

// +kubebuilder:validation:XValidation:rule="[has(self.virtualCdromAtapiBackingInfo), has(self.virtualCdromIsoBackingInfo), has(self.virtualCdromPassthroughBackingInfo), has(self.virtualCdromRemoteAtapiBackingInfo), has(self.virtualCdromRemotePassthroughBackingInfo), has(self.virtualDiskFlatVer1BackingInfo), has(self.virtualDiskFlatVer2BackingInfo), has(self.virtualDiskLocalPMemBackingInfo), has(self.virtualDiskPartitionedRawDiskVer2BackingInfo), has(self.virtualDiskRawDiskMappingVer1BackingInfo), has(self.virtualDiskRawDiskVer2BackingInfo), has(self.virtualDiskSeSparseBackingInfo), has(self.virtualDiskSparseVer1BackingInfo), has(self.virtualDiskSparseVer2BackingInfo), has(self.virtualEthernetCardDistributedVirtualPortBackingInfo), has(self.virtualEthernetCardLegacyNetworkBackingInfo), has(self.virtualEthernetCardNetworkBackingInfo), has(self.virtualEthernetCardOpaqueNetworkBackingInfo), has(self.virtualFloppyDeviceBackingInfo), has(self.virtualFloppyImageBackingInfo), has(self.virtualFloppyRemoteDeviceBackingInfo), has(self.virtualNVDIMMBackingInfo), has(self.virtualParallelPortDeviceBackingInfo), has(self.virtualParallelPortFileBackingInfo), has(self.virtualPCIPassthroughDeviceBackingInfo), has(self.virtualPCIPassthroughDynamicBackingInfo), has(self.virtualPCIPassthroughPluginBackingInfo), has(self.virtualPCIPassthroughVmiopBackingInfo), has(self.virtualPointingDeviceDeviceBackingInfo), has(self.virtualPrecisionClockSystemClockBackingInfo), has(self.virtualSCSIPassthroughDeviceBackingInfo), has(self.virtualSerialPortDeviceBackingInfo), has(self.virtualSerialPortFileBackingInfo), has(self.virtualSerialPortPipeBackingInfo), has(self.virtualSerialPortThinPrintBackingInfo), has(self.virtualSerialPortURIBackingInfo), has(self.virtualSoundCardDeviceBackingInfo), has(self.virtualUSBRemoteClientBackingInfo), has(self.virtualUSBRemoteHostBackingInfo), has(self.virtualUSBUSBBackingInfo)].filter(x, x).size() <= 1",message="at most one backing-specific data field may be specified"

// VirtualDeviceBackingInfo maps the polymorphic virtual device backing
// information hierarchy to a Kubernetes-compatible flat structure.
// It corresponds to vim.vm.device.VirtualDevice.BackingInfo.
type VirtualDeviceBackingInfo struct {

	// Type is the type of the virtual device backing information.
	Type VirtualDeviceBackingInfoType `json:"type"`

	// CD-ROM backing types
	VirtualCdromAtapiBackingInfo             *VirtualCdromAtapiBackingInfo             `json:"virtualCdromAtapiBackingInfo,omitempty"`
	VirtualCdromIsoBackingInfo               *VirtualCdromIsoBackingInfo               `json:"virtualCdromIsoBackingInfo,omitempty"`
	VirtualCdromPassthroughBackingInfo       *VirtualCdromPassthroughBackingInfo       `json:"virtualCdromPassthroughBackingInfo,omitempty"`
	VirtualCdromRemoteAtapiBackingInfo       *VirtualCdromRemoteAtapiBackingInfo       `json:"virtualCdromRemoteAtapiBackingInfo,omitempty"`
	VirtualCdromRemotePassthroughBackingInfo *VirtualCdromRemotePassthroughBackingInfo `json:"virtualCdromRemotePassthroughBackingInfo,omitempty"`

	// Disk backing types
	VirtualDiskFlatVer1BackingInfo               *VirtualDiskFlatVer1BackingInfo               `json:"virtualDiskFlatVer1BackingInfo,omitempty"`
	VirtualDiskFlatVer2BackingInfo               *VirtualDiskFlatVer2BackingInfo               `json:"virtualDiskFlatVer2BackingInfo,omitempty"`
	VirtualDiskLocalPMemBackingInfo              *VirtualDiskLocalPMemBackingInfo              `json:"virtualDiskLocalPMemBackingInfo,omitempty"`
	VirtualDiskPartitionedRawDiskVer2BackingInfo *VirtualDiskPartitionedRawDiskVer2BackingInfo `json:"virtualDiskPartitionedRawDiskVer2BackingInfo,omitempty"`
	VirtualDiskRawDiskMappingVer1BackingInfo     *VirtualDiskRawDiskMappingVer1BackingInfo     `json:"virtualDiskRawDiskMappingVer1BackingInfo,omitempty"`
	VirtualDiskRawDiskVer2BackingInfo            *VirtualDiskRawDiskVer2BackingInfo            `json:"virtualDiskRawDiskVer2BackingInfo,omitempty"`
	VirtualDiskSeSparseBackingInfo               *VirtualDiskSeSparseBackingInfo               `json:"virtualDiskSeSparseBackingInfo,omitempty"`
	VirtualDiskSparseVer1BackingInfo             *VirtualDiskSparseVer1BackingInfo             `json:"virtualDiskSparseVer1BackingInfo,omitempty"`
	VirtualDiskSparseVer2BackingInfo             *VirtualDiskSparseVer2BackingInfo             `json:"virtualDiskSparseVer2BackingInfo,omitempty"`

	// Ethernet card backing types
	VirtualEthernetCardDistributedVirtualPortBackingInfo *VirtualEthernetCardDistributedVirtualPortBackingInfo `json:"virtualEthernetCardDistributedVirtualPortBackingInfo,omitempty"`
	VirtualEthernetCardLegacyNetworkBackingInfo          *VirtualEthernetCardLegacyNetworkBackingInfo          `json:"virtualEthernetCardLegacyNetworkBackingInfo,omitempty"`
	VirtualEthernetCardNetworkBackingInfo                *VirtualEthernetCardNetworkBackingInfo                `json:"virtualEthernetCardNetworkBackingInfo,omitempty"`
	VirtualEthernetCardOpaqueNetworkBackingInfo          *VirtualEthernetCardOpaqueNetworkBackingInfo          `json:"virtualEthernetCardOpaqueNetworkBackingInfo,omitempty"`

	// Floppy backing types
	VirtualFloppyDeviceBackingInfo       *VirtualFloppyDeviceBackingInfo       `json:"virtualFloppyDeviceBackingInfo,omitempty"`
	VirtualFloppyImageBackingInfo        *VirtualFloppyImageBackingInfo        `json:"virtualFloppyImageBackingInfo,omitempty"`
	VirtualFloppyRemoteDeviceBackingInfo *VirtualFloppyRemoteDeviceBackingInfo `json:"virtualFloppyRemoteDeviceBackingInfo,omitempty"`

	// NVDIMM backing type
	VirtualNVDIMMBackingInfo *VirtualNVDIMMBackingInfo `json:"virtualNVDIMMBackingInfo,omitempty"`

	// Parallel port backing types
	VirtualParallelPortDeviceBackingInfo *VirtualParallelPortDeviceBackingInfo `json:"virtualParallelPortDeviceBackingInfo,omitempty"`
	VirtualParallelPortFileBackingInfo   *VirtualParallelPortFileBackingInfo   `json:"virtualParallelPortFileBackingInfo,omitempty"`

	// PCI passthrough backing types
	VirtualPCIPassthroughDeviceBackingInfo  *VirtualPCIPassthroughDeviceBackingInfo  `json:"virtualPCIPassthroughDeviceBackingInfo,omitempty"`
	VirtualPCIPassthroughDynamicBackingInfo *VirtualPCIPassthroughDynamicBackingInfo `json:"virtualPCIPassthroughDynamicBackingInfo,omitempty"`
	VirtualPCIPassthroughPluginBackingInfo  *VirtualPCIPassthroughPluginBackingInfo  `json:"virtualPCIPassthroughPluginBackingInfo,omitempty"`
	VirtualPCIPassthroughVmiopBackingInfo   *VirtualPCIPassthroughVmiopBackingInfo   `json:"virtualPCIPassthroughVmiopBackingInfo,omitempty"`

	// Pointing device backing type
	VirtualPointingDeviceDeviceBackingInfo *VirtualPointingDeviceDeviceBackingInfo `json:"virtualPointingDeviceDeviceBackingInfo,omitempty"`

	// Precision clock backing type
	VirtualPrecisionClockSystemClockBackingInfo *VirtualPrecisionClockSystemClockBackingInfo `json:"virtualPrecisionClockSystemClockBackingInfo,omitempty"`

	// SCSI passthrough backing type
	VirtualSCSIPassthroughDeviceBackingInfo *VirtualSCSIPassthroughDeviceBackingInfo `json:"virtualSCSIPassthroughDeviceBackingInfo,omitempty"`

	// Serial port backing types
	VirtualSerialPortDeviceBackingInfo    *VirtualSerialPortDeviceBackingInfo    `json:"virtualSerialPortDeviceBackingInfo,omitempty"`
	VirtualSerialPortFileBackingInfo      *VirtualSerialPortFileBackingInfo      `json:"virtualSerialPortFileBackingInfo,omitempty"`
	VirtualSerialPortPipeBackingInfo      *VirtualSerialPortPipeBackingInfo      `json:"virtualSerialPortPipeBackingInfo,omitempty"`
	VirtualSerialPortThinPrintBackingInfo *VirtualSerialPortThinPrintBackingInfo `json:"virtualSerialPortThinPrintBackingInfo,omitempty"`
	VirtualSerialPortURIBackingInfo       *VirtualSerialPortURIBackingInfo       `json:"virtualSerialPortURIBackingInfo,omitempty"`

	// Sound card backing type
	VirtualSoundCardDeviceBackingInfo *VirtualSoundCardDeviceBackingInfo `json:"virtualSoundCardDeviceBackingInfo,omitempty"`

	// USB backing types
	VirtualUSBRemoteClientBackingInfo *VirtualUSBRemoteClientBackingInfo `json:"virtualUSBRemoteClientBackingInfo,omitempty"`
	VirtualUSBRemoteHostBackingInfo   *VirtualUSBRemoteHostBackingInfo   `json:"virtualUSBRemoteHostBackingInfo,omitempty"`
	VirtualUSBUSBBackingInfo          *VirtualUSBUSBBackingInfo          `json:"virtualUSBUSBBackingInfo,omitempty"`
}

// VirtualDeviceDeviceBackingInfo contains information about a host device that
// backs a virtual device.
// It corresponds to vim.vm.device.VirtualDevice.DeviceBackingInfo.
type VirtualDeviceDeviceBackingInfo struct {
	// +optional

	// DeviceName is the name of the device on the host system.
	DeviceName string `json:"deviceName,omitempty"`

	// +optional

	// UseAutoDetect indicates whether the device should be auto-detected
	// instead of directly specified. When true, DeviceName is ignored.
	UseAutoDetect *bool `json:"useAutoDetect,omitempty"`
}

// VirtualDeviceRemoteDeviceBackingInfo contains information about a remote
// device that backs a virtual device.
// It corresponds to vim.vm.device.VirtualDevice.RemoteDeviceBackingInfo.
type VirtualDeviceRemoteDeviceBackingInfo struct {
	// DeviceName is the name of the device on the remote system.
	DeviceName string `json:"deviceName"`

	// +optional

	// UseAutoDetect indicates whether the device should be auto-detected
	// instead of directly specified. When true, DeviceName is ignored.
	UseAutoDetect *bool `json:"useAutoDetect,omitempty"`
}

// VirtualDeviceFileBackingInfo contains information about a host file that
// backs a virtual device.
// It corresponds to vim.vm.device.VirtualDevice.FileBackingInfo.
type VirtualDeviceFileBackingInfo struct {
	// FileName is the filename for the host file used in this backing.
	FileName string `json:"fileName"`

	// +optional

	// BackingObjectId is the backing object's durable and immutable
	// identifier.
	BackingObjectId *string `json:"backingObjectId,omitempty"`
}

// VirtualDevicePipeBackingInfo contains information about a named pipe that
// backs a virtual device.
// It corresponds to vim.vm.device.VirtualDevice.PipeBackingInfo.
type VirtualDevicePipeBackingInfo struct {
	// PipeName is the pipe name for the host pipe associated with this
	// backing.
	PipeName string `json:"pipeName"`
}

// VirtualDeviceURIBackingInfo contains information about a network URI that
// backs a virtual device.
// It corresponds to vim.vm.device.VirtualDevice.URIBackingInfo.
type VirtualDeviceURIBackingInfo struct {
	// ServiceURI identifies the local host or a remote system on the network.
	ServiceURI string `json:"serviceURI"`

	// Direction is the connection direction.
	Direction string `json:"direction"`

	// +optional

	// ProxyURI identifies a proxy service providing network access to
	// ServiceURI.
	ProxyURI string `json:"proxyURI,omitempty"`

	// +optional

	// +optional

	// IsSerialPort indicates whether the URI is a serial port.
	IsSerialPort bool `json:"isSerialPort,omitempty"`
}

// VirtualCdromPassthroughBackingInfo defines device pass-through backing for a
// virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.PassthroughBackingInfo.
type VirtualCdromPassthroughBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`

	// Exclusive indicates whether the virtual machine has exclusive CD-ROM
	// device access.
	Exclusive bool `json:"exclusive"`
}

// VirtualCdromRemotePassthroughBackingInfo defines remote pass-through device
// backing for a virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.RemotePassthroughBackingInfo.
type VirtualCdromRemotePassthroughBackingInfo struct {
	VirtualDeviceRemoteDeviceBackingInfo `json:",inline"`

	// Exclusive indicates whether the virtual machine has exclusive CD-ROM
	// device access.
	Exclusive bool `json:"exclusive"`
}

// VirtualPCIPassthroughDeviceBackingInfo contains information about the host
// PCI device backing for a PCI passthrough device.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.DeviceBackingInfo.
type VirtualPCIPassthroughDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`

	// Id is the PCI name ID, composed of "bus:slot.function".
	Id string `json:"id"`

	// DeviceId is the PCI device ID.
	DeviceId string `json:"deviceId"`

	// SystemId is the ID of the system the PCI device is attached to.
	SystemId string `json:"systemId"`

	// VendorId is the PCI vendor ID.
	VendorId int32 `json:"vendorId"`
}

// VirtualPCIPassthroughDynamicBackingInfo contains information about the
// Dynamic DirectPath PCI device backing.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.DynamicBackingInfo.
type VirtualPCIPassthroughDynamicBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`

	// +optional

	// AllowedDevice lists the allowed PCI devices for this Dynamic DirectPath
	// device.
	AllowedDevice []VirtualPCIPassthroughAllowedDevice `json:"allowedDevice,omitempty"`

	// +optional

	// CustomLabel is an optional label that the device must also have set.
	CustomLabel string `json:"customLabel,omitempty"`

	// +optional

	// AssignedId is the ID of the device assigned when the VM is powered on.
	AssignedId string `json:"assignedId,omitempty"`
}

// VirtualPCIPassthroughDvxBackingInfo defines DVX (Device Virtualization
// Extensions) backing for a PCI passthrough device.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.DvxBackingInfo.
type VirtualPCIPassthroughDvxBackingInfo struct {
	// +optional

	// DeviceClass is the device class that backs this DVX device.
	DeviceClass string `json:"deviceClass,omitempty"`

	// +optional

	// ConfigParams contains configuration parameters for this device class.
	ConfigParams []OptionValue `json:"configParams,omitempty"`
}

// VirtualPCIPassthroughPluginBackingInfo is a base backing type for
// plugin-based PCI passthrough devices.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.PluginBackingInfo,
// which carries no data beyond the base device backing.
type VirtualPCIPassthroughPluginBackingInfo struct{}

// VirtualPCIPassthroughVmiopBackingInfo contains information about a VMIOP
// plugin-based PCI passthrough device (typically vGPU).
// It corresponds to vim.vm.device.VirtualPCIPassthrough.VmiopBackingInfo.
type VirtualPCIPassthroughVmiopBackingInfo struct {
	VirtualPCIPassthroughPluginBackingInfo `json:",inline"`

	// +optional

	// Vgpu is the vGPU configuration type exposed by the VMIOP plugin.
	Vgpu string `json:"vgpu,omitempty"`

	// +optional

	// VgpuMigrateDataSize is the expected size of the vGPU device state
	// during migration.
	VgpuMigrateDataSize *resource.Quantity `json:"vgpuMigrateDataSize,omitempty"`

	// +optional

	// MigrateSupported indicates whether the vGPU device supports migration.
	MigrateSupported *bool `json:"migrateSupported,omitempty"`

	// +optional

	// EnhancedMigrateCapability indicates whether the vGPU has enhanced
	// migration features for sub-second downtime.
	EnhancedMigrateCapability *bool `json:"enhancedMigrateCapability,omitempty"`
}

// VirtualPointingDeviceDeviceBackingInfo defines host mouse device backing for
// a virtual pointing device.
// It corresponds to vim.vm.device.VirtualPointingDevice.DeviceBackingInfo.
type VirtualPointingDeviceDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`

	// HostPointingDevice defines the mouse type used to interact with the
	// host mouse.
	HostPointingDevice string `json:"hostPointingDevice"`
}

// VirtualSerialPortPipeBackingInfo defines named pipe backing for a virtual
// serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.PipeBackingInfo.
type VirtualSerialPortPipeBackingInfo struct {
	VirtualDevicePipeBackingInfo `json:",inline"`

	// Endpoint is the role of the virtual machine as an endpoint for the
	// pipe ("client" or "server").
	Endpoint string `json:"endpoint"`

	// +optional

	// NoRxLoss enables optimized data transfer over the pipe to prevent data
	// overrun.
	NoRxLoss *bool `json:"noRxLoss,omitempty"`
}

// VirtualSerialPortThinPrintBackingInfo defines ThinPrint device backing for
// a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.ThinPrintBackingInfo.
type VirtualSerialPortThinPrintBackingInfo struct{}

// VirtualSerialPortURIBackingInfo defines network URI backing for a virtual
// serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.URIBackingInfo.
type VirtualSerialPortURIBackingInfo struct {
	VirtualDeviceURIBackingInfo `json:",inline"`
}

// VirtualUSBRemoteClientBackingInfo identifies a USB device on a remote client
// host.
// It corresponds to vim.vm.device.VirtualUSB.RemoteClientBackingInfo.
type VirtualUSBRemoteClientBackingInfo struct {
	VirtualDeviceRemoteDeviceBackingInfo `json:",inline"`

	// Hostname is the name of the remote client host where the physical USB
	// device resides.
	Hostname string `json:"hostname"`
}

// VirtualUSBRemoteHostBackingInfo identifies a USB device on a specific ESX
// host, supporting persistent access across vMotion.
// It corresponds to vim.vm.device.VirtualUSB.RemoteHostBackingInfo.
type VirtualUSBRemoteHostBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`

	// Hostname is the name of the ESX host to which the physical USB device
	// is attached.
	Hostname string `json:"hostname"`
}

// VirtualEthernetCardNetworkBackingInfo defines standard network backing for
// a virtual Ethernet card.
// It corresponds to vim.vm.device.VirtualEthernetCard.NetworkBackingInfo.
type VirtualEthernetCardNetworkBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`

	// +optional

	Network *ManagedObjectReference `json:"network,omitempty"`
}

// ManagementNetwork describes a management network accessible to virtual
// machines on a host.
type ManagementNetwork struct {
	// +optional

	// Name is the name of the network.
	Name string `json:"name,omitempty"`

	// +optional

	// Type is the type of the network.
	Type string `json:"type,omitempty"`
}

// DistributedVirtualSwitchPortConnection describes a connection to a
// distributed virtual switch port or portgroup.
// It corresponds to vim.dvs.PortConnection.
type DistributedVirtualSwitchPortConnection struct {
	// SwitchUuid is the UUID of the distributed virtual switch.
	SwitchUuid string `json:"switchUuid"`

	// +optional

	// PortgroupKey is the key of the portgroup. Specify this to connect to a
	// portgroup rather than a specific port.
	PortgroupKey string `json:"portgroupKey,omitempty"`

	// +optional

	// PortKey is the key of the specific port. Specify this to connect to a
	// particular port rather than a portgroup.
	PortKey string `json:"portKey,omitempty"`

	// +optional

	// ConnectionCookie is a unique identifier for this port connection
	// instance, assigned by the server.
	ConnectionCookie *int32 `json:"connectionCookie,omitempty"`
}

// VirtualEthernetCardDistributedVirtualPortBackingInfo defines backing for a
// virtual Ethernet card connected to a distributed virtual switch port or
// portgroup.
// It corresponds to vim.vm.device.VirtualEthernetCard.DistributedVirtualPortBackingInfo.
type VirtualEthernetCardDistributedVirtualPortBackingInfo struct {
	// Port is the distributed virtual switch port or portgroup connection.
	Port DistributedVirtualSwitchPortConnection `json:"port"`
}

// VirtualEthernetCardOpaqueNetworkBackingInfo defines backing for a virtual
// Ethernet card connected to an opaque network.
// It corresponds to vim.vm.device.VirtualEthernetCard.OpaqueNetworkBackingInfo.
type VirtualEthernetCardOpaqueNetworkBackingInfo struct {
	// OpaqueNetworkId is the opaque network identifier.
	OpaqueNetworkId string `json:"opaqueNetworkId"`

	// OpaqueNetworkType is the opaque network type.
	OpaqueNetworkType string `json:"opaqueNetworkType"`
}

// VirtualSriovEthernetCardSriovBackingInfo contains information about the
// SR-IOV physical function and virtual function backing for a passthrough NIC.
// It corresponds to vim.vm.device.VirtualSriovEthernetCard.SriovBackingInfo.
type VirtualSriovEthernetCardSriovBackingInfo struct {
	// +optional

	// PhysicalFunctionBacking is the physical function backing for this
	// device.
	PhysicalFunctionBacking *VirtualPCIPassthroughDeviceBackingInfo `json:"physicalFunctionBacking,omitempty"`

	// +optional

	// VirtualFunctionBacking is the virtual function backing for this
	// device.
	VirtualFunctionBacking *VirtualPCIPassthroughDeviceBackingInfo `json:"virtualFunctionBacking,omitempty"`

	// +optional

	// VirtualFunctionIndex is the index of the assigned virtual function.
	VirtualFunctionIndex int32 `json:"virtualFunctionIndex,omitempty"`
}

// VirtualNVDIMMBackingInfo contains information about the file backing for a
// virtual NVDIMM device.
// It corresponds to vim.vm.device.VirtualNVDIMM.BackingInfo.
type VirtualNVDIMMBackingInfo struct {
	VirtualDeviceFileBackingInfo `json:",inline"`

	// +optional

	// ChangeId is the change ID of the virtual NVDIMM for the corresponding
	// snapshot, used to track incremental changes.
	ChangeId string `json:"changeId,omitempty"`

	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:validation:Type=object
	// +kubebuilder:pruning:PreserveUnknownFields

	Parent *VirtualNVDIMMBackingInfo `json:"parent,omitempty"`
}

// VirtualPrecisionClockSystemClockBackingInfo contains information about using
// the host system clock as the backing reference clock for a virtual precision
// clock device.
// It corresponds to vim.vm.device.VirtualPrecisionClock.SystemClockBackingInfo.
type VirtualPrecisionClockSystemClockBackingInfo struct {
	// +optional

	// Protocol is the time synchronization protocol used to discipline the
	// system clock (e.g. "ptp", "ntp").
	Protocol string `json:"protocol,omitempty"`
}

// VirtualCdromAtapiBackingInfo defines ATAPI device backing for a virtual
// CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.AtapiBackingInfo, which carries
// no data beyond the base device backing.
type VirtualCdromAtapiBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualCdromIsoBackingInfo defines ISO image file backing for a virtual
// CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.IsoBackingInfo, which carries
// no data beyond the base file backing.
type VirtualCdromIsoBackingInfo struct {
	VirtualDeviceFileBackingInfo `json:",inline"`
}

// VirtualCdromRemoteAtapiBackingInfo defines remote ATAPI device backing for a
// virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.RemoteAtapiBackingInfo, which
// carries no data beyond the base remote device backing.
type VirtualCdromRemoteAtapiBackingInfo struct {
	VirtualDeviceRemoteDeviceBackingInfo `json:",inline"`
}

// VirtualEthernetCardLegacyNetworkBackingInfo defines legacy network backing
// for a virtual Ethernet card.
// It corresponds to vim.vm.device.VirtualEthernetCard.LegacyNetworkBackingInfo,
// which carries no data beyond the base device backing.
type VirtualEthernetCardLegacyNetworkBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualFloppyDeviceBackingInfo defines host device backing for a virtual
// floppy drive.
// It corresponds to vim.vm.device.VirtualFloppy.DeviceBackingInfo, which
// carries no data beyond the base device backing.
type VirtualFloppyDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualFloppyImageBackingInfo defines image file backing for a virtual
// floppy drive.
// It corresponds to vim.vm.device.VirtualFloppy.ImageBackingInfo, which
// carries no data beyond the base file backing.
type VirtualFloppyImageBackingInfo struct {
	VirtualDeviceFileBackingInfo `json:",inline"`
}

// VirtualFloppyRemoteDeviceBackingInfo defines remote device backing for a
// virtual floppy drive.
// It corresponds to vim.vm.device.VirtualFloppy.RemoteDeviceBackingInfo, which
// carries no data beyond the base remote device backing.
type VirtualFloppyRemoteDeviceBackingInfo struct {
	VirtualDeviceRemoteDeviceBackingInfo `json:",inline"`
}

// VirtualParallelPortDeviceBackingInfo defines host device backing for a
// virtual parallel port.
// It corresponds to vim.vm.device.VirtualParallelPort.DeviceBackingInfo, which
// carries no data beyond the base device backing.
type VirtualParallelPortDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualParallelPortFileBackingInfo defines host file backing for a virtual
// parallel port.
// It corresponds to vim.vm.device.VirtualParallelPort.FileBackingInfo, which
// carries no data beyond the base file backing.
type VirtualParallelPortFileBackingInfo struct {
	VirtualDeviceFileBackingInfo `json:",inline"`
}

// VirtualSCSIPassthroughDeviceBackingInfo defines host device backing for a
// virtual SCSI passthrough device.
// It corresponds to vim.vm.device.VirtualSCSIPassthrough.DeviceBackingInfo,
// which carries no data beyond the base device backing.
type VirtualSCSIPassthroughDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualSerialPortDeviceBackingInfo defines host device backing for a virtual
// serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.DeviceBackingInfo, which
// carries no data beyond the base device backing.
type VirtualSerialPortDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualSerialPortFileBackingInfo defines host file backing for a virtual
// serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.FileBackingInfo, which
// carries no data beyond the base file backing.
type VirtualSerialPortFileBackingInfo struct {
	VirtualDeviceFileBackingInfo `json:",inline"`
}

// VirtualSoundCardDeviceBackingInfo defines host device backing for a virtual
// sound card.
// It corresponds to vim.vm.device.VirtualSoundCard.DeviceBackingInfo, which
// carries no data beyond the base device backing.
type VirtualSoundCardDeviceBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}

// VirtualUSBUSBBackingInfo defines host USB device backing for a virtual USB
// device.
// It corresponds to vim.vm.device.VirtualUSB.USBBackingInfo, which carries no
// data beyond the base device backing.
type VirtualUSBUSBBackingInfo struct {
	VirtualDeviceDeviceBackingInfo `json:",inline"`
}
