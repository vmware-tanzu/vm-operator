// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// VirtualDeviceBackingOptionType identifies the concrete runtime type of a
// virtual device's backing option. The values correspond to vSphere API class
// names (e.g. vim.vm.device.VirtualDisk.FlatVer2BackingOption).
type VirtualDeviceBackingOptionType string

const (
	// CD-ROM backing option types
	VirtualDeviceBackingOptionTypeCdromAtapi             VirtualDeviceBackingOptionType = "VirtualCdromAtapiBackingOption"
	VirtualDeviceBackingOptionTypeCdromIso               VirtualDeviceBackingOptionType = "VirtualCdromIsoBackingOption"
	VirtualDeviceBackingOptionTypeCdromPassthrough       VirtualDeviceBackingOptionType = "VirtualCdromPassthroughBackingOption"
	VirtualDeviceBackingOptionTypeCdromRemoteAtapi       VirtualDeviceBackingOptionType = "VirtualCdromRemoteAtapiBackingOption"
	VirtualDeviceBackingOptionTypeCdromRemotePassthrough VirtualDeviceBackingOptionType = "VirtualCdromRemotePassthroughBackingOption"

	// Disk backing option types
	VirtualDeviceBackingOptionTypeDiskFlatVer1           VirtualDeviceBackingOptionType = "VirtualDiskFlatVer1BackingOption"
	VirtualDeviceBackingOptionTypeDiskFlatVer2           VirtualDeviceBackingOptionType = "VirtualDiskFlatVer2BackingOption"
	VirtualDeviceBackingOptionTypeDiskLocalPMem          VirtualDeviceBackingOptionType = "VirtualDiskLocalPMemBackingOption"
	VirtualDeviceBackingOptionTypeDiskPartitionedRawVer2 VirtualDeviceBackingOptionType = "VirtualDiskPartitionedRawDiskVer2BackingOption"
	VirtualDeviceBackingOptionTypeDiskRawMappingVer1     VirtualDeviceBackingOptionType = "VirtualDiskRawDiskMappingVer1BackingOption"
	VirtualDeviceBackingOptionTypeDiskRawVer2            VirtualDeviceBackingOptionType = "VirtualDiskRawDiskVer2BackingOption"
	VirtualDeviceBackingOptionTypeDiskSeSparse           VirtualDeviceBackingOptionType = "VirtualDiskSeSparseBackingOption"
	VirtualDeviceBackingOptionTypeDiskSparseVer1         VirtualDeviceBackingOptionType = "VirtualDiskSparseVer1BackingOption"
	VirtualDeviceBackingOptionTypeDiskSparseVer2         VirtualDeviceBackingOptionType = "VirtualDiskSparseVer2BackingOption"

	// Ethernet card backing option types
	VirtualDeviceBackingOptionTypeEthernetCardDVPort        VirtualDeviceBackingOptionType = "VirtualEthernetCardDVPortBackingOption"
	VirtualDeviceBackingOptionTypeEthernetCardLegacyNetwork VirtualDeviceBackingOptionType = "VirtualEthernetCardLegacyNetworkBackingOption"
	VirtualDeviceBackingOptionTypeEthernetCardNetwork       VirtualDeviceBackingOptionType = "VirtualEthernetCardNetworkBackingOption"
	VirtualDeviceBackingOptionTypeEthernetCardOpaqueNetwork VirtualDeviceBackingOptionType = "VirtualEthernetCardOpaqueNetworkBackingOption"

	// Floppy backing option types
	VirtualDeviceBackingOptionTypeFloppyDevice       VirtualDeviceBackingOptionType = "VirtualFloppyDeviceBackingOption"
	VirtualDeviceBackingOptionTypeFloppyImage        VirtualDeviceBackingOptionType = "VirtualFloppyImageBackingOption"
	VirtualDeviceBackingOptionTypeFloppyRemoteDevice VirtualDeviceBackingOptionType = "VirtualFloppyRemoteDeviceBackingOption"

	// Parallel port backing option types
	VirtualDeviceBackingOptionTypeParallelPortDevice VirtualDeviceBackingOptionType = "VirtualParallelPortDeviceBackingOption"
	VirtualDeviceBackingOptionTypeParallelPortFile   VirtualDeviceBackingOptionType = "VirtualParallelPortFileBackingOption"

	// PCI passthrough backing option types
	VirtualDeviceBackingOptionTypePCIPassthroughDevice  VirtualDeviceBackingOptionType = "VirtualPCIPassthroughDeviceBackingOption"
	VirtualDeviceBackingOptionTypePCIPassthroughDynamic VirtualDeviceBackingOptionType = "VirtualPCIPassthroughDynamicBackingOption"
	VirtualDeviceBackingOptionTypePCIPassthroughPlugin  VirtualDeviceBackingOptionType = "VirtualPCIPassthroughPluginBackingOption"
	VirtualDeviceBackingOptionTypePCIPassthroughVmiop   VirtualDeviceBackingOptionType = "VirtualPCIPassthroughVmiopBackingOption"

	// Pointing device backing option type
	VirtualDeviceBackingOptionTypePointingDevice VirtualDeviceBackingOptionType = "VirtualPointingDeviceBackingOption"

	// Precision clock backing option type
	VirtualDeviceBackingOptionTypePrecisionClockSystemClock VirtualDeviceBackingOptionType = "VirtualPrecisionClockSystemClockBackingOption"

	// SCSI passthrough backing option type
	VirtualDeviceBackingOptionTypeSCSIPassthroughDevice VirtualDeviceBackingOptionType = "VirtualSCSIPassthroughDeviceBackingOption"

	// Serial port backing option types
	VirtualDeviceBackingOptionTypeSerialPortDevice    VirtualDeviceBackingOptionType = "VirtualSerialPortDeviceBackingOption"
	VirtualDeviceBackingOptionTypeSerialPortFile      VirtualDeviceBackingOptionType = "VirtualSerialPortFileBackingOption"
	VirtualDeviceBackingOptionTypeSerialPortPipe      VirtualDeviceBackingOptionType = "VirtualSerialPortPipeBackingOption"
	VirtualDeviceBackingOptionTypeSerialPortThinPrint VirtualDeviceBackingOptionType = "VirtualSerialPortThinPrintBackingOption"
	VirtualDeviceBackingOptionTypeSerialPortURI       VirtualDeviceBackingOptionType = "VirtualSerialPortURIBackingOption"

	// Sound card backing option type
	VirtualDeviceBackingOptionTypeSoundCardDevice VirtualDeviceBackingOptionType = "VirtualSoundCardDeviceBackingOption"

	// USB backing option types
	VirtualDeviceBackingOptionTypeUSBRemoteClient VirtualDeviceBackingOptionType = "VirtualUSBRemoteClientBackingOption"
	VirtualDeviceBackingOptionTypeUSBRemoteHost   VirtualDeviceBackingOptionType = "VirtualUSBRemoteHostBackingOption"
	VirtualDeviceBackingOptionTypeUSBUSB          VirtualDeviceBackingOptionType = "VirtualUSBUSBBackingOption"
)

// +kubebuilder:validation:XValidation:rule="[has(self.virtualCdromAtapiBackingOption), has(self.virtualCdromIsoBackingOption), has(self.virtualCdromPassthroughBackingOption), has(self.virtualCdromRemoteAtapiBackingOption), has(self.virtualCdromRemotePassthroughBackingOption), has(self.virtualDiskFlatVer1BackingOption), has(self.virtualDiskFlatVer2BackingOption), has(self.virtualDiskLocalPMemBackingOption), has(self.virtualDiskPartitionedRawDiskVer2BackingOption), has(self.virtualDiskRawDiskMappingVer1BackingOption), has(self.virtualDiskRawDiskVer2BackingOption), has(self.virtualDiskSeSparseBackingOption), has(self.virtualDiskSparseVer1BackingOption), has(self.virtualDiskSparseVer2BackingOption), has(self.virtualEthernetCardDVPortBackingOption), has(self.virtualEthernetCardLegacyNetworkBackingOption), has(self.virtualEthernetCardNetworkBackingOption), has(self.virtualEthernetCardOpaqueNetworkBackingOption), has(self.virtualFloppyDeviceBackingOption), has(self.virtualFloppyImageBackingOption), has(self.virtualFloppyRemoteDeviceBackingOption), has(self.virtualParallelPortDeviceBackingOption), has(self.virtualParallelPortFileBackingOption), has(self.virtualPCIPassthroughDeviceBackingOption), has(self.virtualPCIPassthroughDynamicBackingOption), has(self.virtualPCIPassthroughPluginBackingOption), has(self.virtualPCIPassthroughVmiopBackingOption), has(self.virtualPointingDeviceBackingOption), has(self.virtualPrecisionClockSystemClockBackingOption), has(self.virtualSCSIPassthroughDeviceBackingOption), has(self.virtualSerialPortDeviceBackingOption), has(self.virtualSerialPortFileBackingOption), has(self.virtualSerialPortPipeBackingOption), has(self.virtualSerialPortThinPrintBackingOption), has(self.virtualSerialPortURIBackingOption), has(self.virtualSoundCardDeviceBackingOption), has(self.virtualUSBRemoteClientBackingOption), has(self.virtualUSBRemoteHostBackingOption), has(self.virtualUSBUSBBackingOption)].filter(x, x).size() <= 1",message="at most one backing-option-specific data field may be specified"

// VirtualDeviceBackingOption maps the polymorphic virtual device backing
// option hierarchy to a Kubernetes-compatible flat structure.
// It corresponds to vim.vm.device.VirtualDeviceOption.BackingOption.
type VirtualDeviceBackingOption struct {

	// Type is the type of the virtual device backing option.
	Type VirtualDeviceBackingOptionType `json:"type"`

	// CD-ROM backing option types
	VirtualCdromAtapiBackingOption             *VirtualCdromAtapiBackingOption             `json:"virtualCdromAtapiBackingOption,omitempty"`
	VirtualCdromIsoBackingOption               *VirtualCdromIsoBackingOption               `json:"virtualCdromIsoBackingOption,omitempty"`
	VirtualCdromPassthroughBackingOption       *VirtualCdromPassthroughBackingOption       `json:"virtualCdromPassthroughBackingOption,omitempty"`
	VirtualCdromRemoteAtapiBackingOption       *VirtualCdromRemoteAtapiBackingOption       `json:"virtualCdromRemoteAtapiBackingOption,omitempty"`
	VirtualCdromRemotePassthroughBackingOption *VirtualCdromRemotePassthroughBackingOption `json:"virtualCdromRemotePassthroughBackingOption,omitempty"`

	// Disk backing option types
	VirtualDiskFlatVer1BackingOption               *VirtualDiskFlatVer1BackingOption               `json:"virtualDiskFlatVer1BackingOption,omitempty"`
	VirtualDiskFlatVer2BackingOption               *VirtualDiskFlatVer2BackingOption               `json:"virtualDiskFlatVer2BackingOption,omitempty"`
	VirtualDiskLocalPMemBackingOption              *VirtualDiskLocalPMemBackingOption              `json:"virtualDiskLocalPMemBackingOption,omitempty"`
	VirtualDiskPartitionedRawDiskVer2BackingOption *VirtualDiskPartitionedRawDiskVer2BackingOption `json:"virtualDiskPartitionedRawDiskVer2BackingOption,omitempty"`
	VirtualDiskRawDiskMappingVer1BackingOption     *VirtualDiskRawDiskMappingVer1BackingOption     `json:"virtualDiskRawDiskMappingVer1BackingOption,omitempty"`
	VirtualDiskRawDiskVer2BackingOption            *VirtualDiskRawDiskVer2BackingOption            `json:"virtualDiskRawDiskVer2BackingOption,omitempty"`
	VirtualDiskSeSparseBackingOption               *VirtualDiskSeSparseBackingOption               `json:"virtualDiskSeSparseBackingOption,omitempty"`
	VirtualDiskSparseVer1BackingOption             *VirtualDiskSparseVer1BackingOption             `json:"virtualDiskSparseVer1BackingOption,omitempty"`
	VirtualDiskSparseVer2BackingOption             *VirtualDiskSparseVer2BackingOption             `json:"virtualDiskSparseVer2BackingOption,omitempty"`

	// Ethernet card backing option types
	VirtualEthernetCardDVPortBackingOption        *VirtualEthernetCardDVPortBackingOption        `json:"virtualEthernetCardDVPortBackingOption,omitempty"`
	VirtualEthernetCardLegacyNetworkBackingOption *VirtualEthernetCardLegacyNetworkBackingOption `json:"virtualEthernetCardLegacyNetworkBackingOption,omitempty"`
	VirtualEthernetCardNetworkBackingOption       *VirtualEthernetCardNetworkBackingOption       `json:"virtualEthernetCardNetworkBackingOption,omitempty"`
	VirtualEthernetCardOpaqueNetworkBackingOption *VirtualEthernetCardOpaqueNetworkBackingOption `json:"virtualEthernetCardOpaqueNetworkBackingOption,omitempty"`

	// Floppy backing option types
	VirtualFloppyDeviceBackingOption       *VirtualFloppyDeviceBackingOption       `json:"virtualFloppyDeviceBackingOption,omitempty"`
	VirtualFloppyImageBackingOption        *VirtualFloppyImageBackingOption        `json:"virtualFloppyImageBackingOption,omitempty"`
	VirtualFloppyRemoteDeviceBackingOption *VirtualFloppyRemoteDeviceBackingOption `json:"virtualFloppyRemoteDeviceBackingOption,omitempty"`

	// Parallel port backing option types
	VirtualParallelPortDeviceBackingOption *VirtualParallelPortDeviceBackingOption `json:"virtualParallelPortDeviceBackingOption,omitempty"`
	VirtualParallelPortFileBackingOption   *VirtualParallelPortFileBackingOption   `json:"virtualParallelPortFileBackingOption,omitempty"`

	// PCI passthrough backing option types
	VirtualPCIPassthroughDeviceBackingOption  *VirtualPCIPassthroughDeviceBackingOption  `json:"virtualPCIPassthroughDeviceBackingOption,omitempty"`
	VirtualPCIPassthroughDynamicBackingOption *VirtualPCIPassthroughDynamicBackingOption `json:"virtualPCIPassthroughDynamicBackingOption,omitempty"`
	VirtualPCIPassthroughPluginBackingOption  *VirtualPCIPassthroughPluginBackingOption  `json:"virtualPCIPassthroughPluginBackingOption,omitempty"`
	VirtualPCIPassthroughVmiopBackingOption   *VirtualPCIPassthroughVmiopBackingOption   `json:"virtualPCIPassthroughVmiopBackingOption,omitempty"`

	// Pointing device backing option type
	VirtualPointingDeviceBackingOption *VirtualPointingDeviceBackingOption `json:"virtualPointingDeviceBackingOption,omitempty"`

	// Precision clock backing option type
	VirtualPrecisionClockSystemClockBackingOption *VirtualPrecisionClockSystemClockBackingOption `json:"virtualPrecisionClockSystemClockBackingOption,omitempty"`

	// SCSI passthrough backing option type
	VirtualSCSIPassthroughDeviceBackingOption *VirtualSCSIPassthroughDeviceBackingOption `json:"virtualSCSIPassthroughDeviceBackingOption,omitempty"`

	// Serial port backing option types
	VirtualSerialPortDeviceBackingOption    *VirtualSerialPortDeviceBackingOption    `json:"virtualSerialPortDeviceBackingOption,omitempty"`
	VirtualSerialPortFileBackingOption      *VirtualSerialPortFileBackingOption      `json:"virtualSerialPortFileBackingOption,omitempty"`
	VirtualSerialPortPipeBackingOption      *VirtualSerialPortPipeBackingOption      `json:"virtualSerialPortPipeBackingOption,omitempty"`
	VirtualSerialPortThinPrintBackingOption *VirtualSerialPortThinPrintBackingOption `json:"virtualSerialPortThinPrintBackingOption,omitempty"`
	VirtualSerialPortURIBackingOption       *VirtualSerialPortURIBackingOption       `json:"virtualSerialPortURIBackingOption,omitempty"`

	// Sound card backing option type
	VirtualSoundCardDeviceBackingOption *VirtualSoundCardDeviceBackingOption `json:"virtualSoundCardDeviceBackingOption,omitempty"`

	// USB backing option types
	VirtualUSBRemoteClientBackingOption *VirtualUSBRemoteClientBackingOption `json:"virtualUSBRemoteClientBackingOption,omitempty"`
	VirtualUSBRemoteHostBackingOption   *VirtualUSBRemoteHostBackingOption   `json:"virtualUSBRemoteHostBackingOption,omitempty"`
	VirtualUSBUSBBackingOption          *VirtualUSBUSBBackingOption          `json:"virtualUSBUSBBackingOption,omitempty"`
}

// VirtualDeviceDeviceBackingOption contains the options for a host device
// backing.
// It corresponds to vim.vm.device.VirtualDeviceOption.DeviceBackingOption.
type VirtualDeviceDeviceBackingOption struct {
	// AutoDetectAvailable indicates whether the specific instance of this
	// device can be auto-detected on the host instead of having to specify a
	// particular physical device.
	AutoDetectAvailable BoolOption `json:"autoDetectAvailable"`
}

// VirtualDeviceFileBackingOption contains the options for a host file backing.
// It corresponds to vim.vm.device.VirtualDeviceOption.FileBackingOption.
type VirtualDeviceFileBackingOption struct {
	// +optional

	// FileNameExtensions lists the valid filename extensions for the backing
	// file. When empty, any file extension is acceptable.
	FileNameExtensions *ChoiceOption `json:"fileNameExtensions,omitempty"`
}

// VirtualDeviceRemoteDeviceBackingOption contains the options for a remote
// device backing.
// It corresponds to vim.vm.device.VirtualDeviceOption.RemoteDeviceBackingOption.
type VirtualDeviceRemoteDeviceBackingOption struct {
	// AutoDetectAvailable indicates whether the specific instance of this
	// device can be auto-detected on the host instead of having to specify a
	// particular physical device.
	AutoDetectAvailable BoolOption `json:"autoDetectAvailable"`
}

// VirtualDevicePipeBackingOption contains the options for a named pipe
// backing.
// It corresponds to vim.vm.device.VirtualDeviceOption.PipeBackingOption, which
// carries no data beyond the base backing option.
type VirtualDevicePipeBackingOption struct{}

// VirtualDeviceURIBackingOption contains the options for a network URI
// backing.
// It corresponds to vim.vm.device.VirtualDeviceOption.URIBackingOption.
type VirtualDeviceURIBackingOption struct {
	// Directions lists the possible connection directions (e.g. "server",
	// "client").
	Directions ChoiceOption `json:"directions"`
}

// VirtualCdromAtapiBackingOption contains the options for ATAPI device backing
// of a virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.AtapiBackingOption.
type VirtualCdromAtapiBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualCdromIsoBackingOption contains the options for ISO image file backing
// of a virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.IsoBackingOption.
type VirtualCdromIsoBackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`
}

// VirtualCdromPassthroughBackingOption contains the options for device
// pass-through backing of a virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.PassthroughBackingOption.
type VirtualCdromPassthroughBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`

	// Exclusive indicates whether exclusive CD-ROM device access is supported.
	Exclusive BoolOption `json:"exclusive"`
}

// VirtualCdromRemoteAtapiBackingOption contains the options for remote ATAPI
// device backing of a virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.RemoteAtapiBackingOption.
type VirtualCdromRemoteAtapiBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualCdromRemotePassthroughBackingOption contains the options for remote
// pass-through device backing of a virtual CD-ROM.
// It corresponds to vim.vm.device.VirtualCdrom.RemotePassthroughBackingOption.
type VirtualCdromRemotePassthroughBackingOption struct {
	VirtualDeviceRemoteDeviceBackingOption `json:",inline"`

	// Exclusive indicates whether exclusive CD-ROM device access is supported.
	Exclusive BoolOption `json:"exclusive"`
}

// VirtualDiskDeltaDiskFormatsSupported describes the delta disk formats
// supported for a given datastore type.
// It corresponds to vim.vm.device.VirtualDiskOption.DeltaDiskFormatsSupported.
type VirtualDiskDeltaDiskFormatsSupported struct {
	// DatastoreType is the datastore type name.
	DatastoreType string `json:"datastoreType"`

	// DeltaDiskFormat lists the supported delta disk formats (e.g.
	// "redoLogFormat", "nativeFormat").
	DeltaDiskFormat ChoiceOption `json:"deltaDiskFormat"`
}

// VirtualDiskFlatVer1BackingOption contains the options for flat-format
// (GSX Server 2.x) file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.FlatVer1BackingOption.
type VirtualDiskFlatVer1BackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`

	// DiskMode describes the supported disk modes.
	DiskMode ChoiceOption `json:"diskMode"`

	// Split indicates whether the host supports letting the client select
	// whether a disk should be split.
	Split BoolOption `json:"split"`

	// WriteThrough indicates whether the host supports letting the client
	// select "writethrough" as a mode for virtual disks.
	WriteThrough BoolOption `json:"writeThrough"`

	// Growable indicates whether this backing can have its size changed.
	Growable bool `json:"growable"`
}

// VirtualDiskFlatVer2BackingOption contains the options for flat-format
// (ESX Server 2.x/3.x) file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.FlatVer2BackingOption.
type VirtualDiskFlatVer2BackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`

	// DiskMode describes the supported disk modes.
	DiskMode ChoiceOption `json:"diskMode"`

	// Split indicates whether the host supports letting the client select
	// whether a disk should be split.
	Split BoolOption `json:"split"`

	// WriteThrough indicates whether the host supports letting the client
	// select "writethrough" as a mode for virtual disks.
	WriteThrough BoolOption `json:"writeThrough"`

	// Growable indicates whether this disk backing can be extended to larger
	// sizes through a reconfigure operation.
	Growable bool `json:"growable"`

	// HotGrowable indicates whether this disk backing can be extended to
	// larger sizes through a reconfigure operation while the virtual machine
	// is powered on.
	HotGrowable bool `json:"hotGrowable"`

	// Uuid indicates whether this backing supports the disk UUID property.
	Uuid bool `json:"uuid"`

	// ThinProvisioned indicates whether this backing supports
	// thin-provisioned disks.
	ThinProvisioned BoolOption `json:"thinProvisioned"`

	// EagerlyScrub indicates whether this backing supports eager scrubbing.
	EagerlyScrub BoolOption `json:"eagerlyScrub"`

	// DeltaDiskFormat describes the supported delta disk formats.
	//
	// Deprecated: As of vSphere API 5.1, use DeltaDiskFormatsSupported.
	DeltaDiskFormat ChoiceOption `json:"deltaDiskFormat"`

	// +optional

	// DeltaDiskFormatsSupported lists the delta disk formats supported for
	// each datastore type.
	DeltaDiskFormatsSupported []VirtualDiskDeltaDiskFormatsSupported `json:"deltaDiskFormatsSupported,omitempty"`

	// +optional

	// VirtualDiskFormat describes the supported virtual disk formats.
	VirtualDiskFormat *ChoiceOption `json:"virtualDiskFormat,omitempty"`
}

// VirtualDiskLocalPMemBackingOption contains the options for persistent memory
// (PMem) file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.LocalPMemBackingOption.
type VirtualDiskLocalPMemBackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`

	// DiskMode describes the supported disk modes.
	DiskMode ChoiceOption `json:"diskMode"`

	// Growable indicates whether this disk backing can be extended to larger
	// sizes through a reconfigure operation.
	Growable bool `json:"growable"`

	// HotGrowable indicates whether this disk backing can be extended to
	// larger sizes through a reconfigure operation while the virtual machine
	// is powered on.
	HotGrowable bool `json:"hotGrowable"`

	// Uuid indicates whether this backing supports the disk UUID property.
	Uuid bool `json:"uuid"`
}

// VirtualDiskRawDiskMappingVer1BackingOption contains the options for raw
// device mapping (RDM) file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.RawDiskMappingVer1BackingOption.
type VirtualDiskRawDiskMappingVer1BackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`

	// +optional

	// DescriptorFileNameExtensions lists the valid extensions for the
	// filename of the optional raw disk mapping descriptor file.
	DescriptorFileNameExtensions *ChoiceOption `json:"descriptorFileNameExtensions,omitempty"`

	// CompatibilityMode describes the supported raw disk mapping
	// compatibility modes.
	CompatibilityMode ChoiceOption `json:"compatibilityMode"`

	// DiskMode describes the supported disk modes.
	DiskMode ChoiceOption `json:"diskMode"`

	// Uuid indicates whether this backing supports the disk UUID property.
	Uuid bool `json:"uuid"`

	// +optional

	// VirtualDiskFormat describes the supported virtual disk formats.
	VirtualDiskFormat *ChoiceOption `json:"virtualDiskFormat,omitempty"`
}

// VirtualDiskRawDiskVer2BackingOption contains the options for raw host device
// (VMware Server) backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.RawDiskVer2BackingOption.
type VirtualDiskRawDiskVer2BackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`

	// DescriptorFileNameExtensions lists the valid extensions for the
	// filename of the raw disk descriptor file.
	DescriptorFileNameExtensions ChoiceOption `json:"descriptorFileNameExtensions"`

	// Uuid indicates whether this backing supports the disk UUID property.
	Uuid bool `json:"uuid"`
}

// VirtualDiskPartitionedRawDiskVer2BackingOption contains the options for
// partitioned raw host device backing of a virtual disk.
// It corresponds to
// vim.vm.device.VirtualDisk.PartitionedRawDiskVer2BackingOption.
type VirtualDiskPartitionedRawDiskVer2BackingOption struct {
	VirtualDiskRawDiskVer2BackingOption `json:",inline"`
}

// VirtualDiskSeSparseBackingOption contains the options for space-efficient
// sparse format file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.SeSparseBackingOption.
type VirtualDiskSeSparseBackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`

	// DiskMode describes the supported disk modes.
	DiskMode ChoiceOption `json:"diskMode"`

	// WriteThrough indicates whether the host supports letting the client
	// select "writethrough" as a mode for virtual disks.
	WriteThrough BoolOption `json:"writeThrough"`

	// Growable indicates whether this disk backing can be extended to larger
	// sizes through a reconfigure operation.
	Growable bool `json:"growable"`

	// HotGrowable indicates whether this disk backing can be extended to
	// larger sizes through a reconfigure operation while the virtual machine
	// is powered on.
	HotGrowable bool `json:"hotGrowable"`

	// Uuid indicates whether this backing supports the disk UUID property.
	Uuid bool `json:"uuid"`

	// +optional

	// DeltaDiskFormatsSupported lists the delta disk formats supported for
	// each datastore type.
	DeltaDiskFormatsSupported []VirtualDiskDeltaDiskFormatsSupported `json:"deltaDiskFormatsSupported,omitempty"`

	// +optional

	// VirtualDiskFormat describes the supported virtual disk formats.
	VirtualDiskFormat *ChoiceOption `json:"virtualDiskFormat,omitempty"`
}

// VirtualDiskSparseVer1BackingOption contains the options for sparse-format
// (GSX Server 2.x) file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.SparseVer1BackingOption.
type VirtualDiskSparseVer1BackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`

	// DiskModes describes the supported disk modes.
	DiskModes ChoiceOption `json:"diskModes"`

	// Split indicates whether the host supports letting the client select
	// whether a sparse disk should be split.
	Split BoolOption `json:"split"`

	// WriteThrough indicates whether the host supports letting the client
	// select "writethrough" as a mode for virtual disks.
	WriteThrough BoolOption `json:"writeThrough"`

	// Growable indicates whether this backing can have its size changed.
	Growable bool `json:"growable"`
}

// VirtualDiskSparseVer2BackingOption contains the options for sparse-format
// (VMware Server) file backing of a virtual disk.
// It corresponds to vim.vm.device.VirtualDisk.SparseVer2BackingOption.
type VirtualDiskSparseVer2BackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`

	// DiskMode describes the supported disk modes.
	DiskMode ChoiceOption `json:"diskMode"`

	// Split indicates whether the host supports letting the client select
	// whether a sparse disk should be split.
	Split BoolOption `json:"split"`

	// WriteThrough indicates whether the host supports letting the client
	// select "writethrough" as a mode for virtual disks.
	WriteThrough BoolOption `json:"writeThrough"`

	// Growable indicates whether this disk backing can be extended to larger
	// sizes through a reconfigure operation.
	Growable bool `json:"growable"`

	// HotGrowable indicates whether this disk backing can be extended to
	// larger sizes through a reconfigure operation while the virtual machine
	// is powered on.
	HotGrowable bool `json:"hotGrowable"`

	// Uuid indicates whether this backing supports the disk UUID property.
	Uuid bool `json:"uuid"`

	// +optional

	// VirtualDiskFormat describes the supported virtual disk formats.
	VirtualDiskFormat *ChoiceOption `json:"virtualDiskFormat,omitempty"`
}

// VirtualEthernetCardDVPortBackingOption contains the options for distributed
// virtual port backing of a virtual Ethernet card.
// It corresponds to vim.vm.device.VirtualEthernetCard.DVPortBackingOption,
// which carries no data beyond the base backing option.
type VirtualEthernetCardDVPortBackingOption struct{}

// VirtualEthernetCardLegacyNetworkBackingOption contains the options for
// legacy network backing of a virtual Ethernet card.
// It corresponds to
// vim.vm.device.VirtualEthernetCard.LegacyNetworkBackingOption.
type VirtualEthernetCardLegacyNetworkBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualEthernetCardNetworkBackingOption contains the options for standard
// network backing of a virtual Ethernet card.
// It corresponds to vim.vm.device.VirtualEthernetCard.NetworkBackingOption.
type VirtualEthernetCardNetworkBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualEthernetCardOpaqueNetworkBackingOption contains the options for
// opaque network backing of a virtual Ethernet card.
// It corresponds to
// vim.vm.device.VirtualEthernetCard.OpaqueNetworkBackingOption, which carries
// no data beyond the base backing option.
type VirtualEthernetCardOpaqueNetworkBackingOption struct{}

// VirtualFloppyDeviceBackingOption contains the options for host device
// backing of a virtual floppy drive.
// It corresponds to vim.vm.device.VirtualFloppy.DeviceBackingOption.
type VirtualFloppyDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualFloppyImageBackingOption contains the options for image file backing
// of a virtual floppy drive.
// It corresponds to vim.vm.device.VirtualFloppy.ImageBackingOption.
type VirtualFloppyImageBackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`
}

// VirtualFloppyRemoteDeviceBackingOption contains the options for remote
// device backing of a virtual floppy drive.
// It corresponds to vim.vm.device.VirtualFloppy.RemoteDeviceBackingOption.
type VirtualFloppyRemoteDeviceBackingOption struct {
	VirtualDeviceRemoteDeviceBackingOption `json:",inline"`
}

// VirtualParallelPortDeviceBackingOption contains the options for host device
// backing of a virtual parallel port.
// It corresponds to vim.vm.device.VirtualParallelPort.DeviceBackingOption.
type VirtualParallelPortDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualParallelPortFileBackingOption contains the options for host file
// backing of a virtual parallel port.
// It corresponds to vim.vm.device.VirtualParallelPort.FileBackingOption.
type VirtualParallelPortFileBackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`
}

// VirtualPCIPassthroughDeviceBackingOption contains the options for host PCI
// device backing of a PCI passthrough device.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.DeviceBackingOption.
type VirtualPCIPassthroughDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualPCIPassthroughDynamicBackingOption contains the options for Dynamic
// DirectPath PCI device backing.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.DynamicBackingOption.
type VirtualPCIPassthroughDynamicBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualPCIPassthroughPluginBackingOption is a base backing option type for
// plugin-based PCI passthrough devices.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.PluginBackingOption,
// which carries no data beyond the base backing option.
type VirtualPCIPassthroughPluginBackingOption struct{}

// VirtualPCIPassthroughVmiopBackingOption contains the options for a VMIOP
// plugin-based PCI passthrough device (typically vGPU).
// It corresponds to vim.vm.device.VirtualPCIPassthrough.VmiopBackingOption.
type VirtualPCIPassthroughVmiopBackingOption struct {
	VirtualPCIPassthroughPluginBackingOption `json:",inline"`

	// Vgpu indicates which GPU profile the plugin should emulate.
	Vgpu StringOption `json:"vgpu"`

	// MaxInstances is the maximum number of instances of this backing type
	// allowed per virtual machine.
	MaxInstances int32 `json:"maxInstances"`
}

// VirtualPointingDeviceBackingOption contains the options for host device
// backing of a virtual pointing device.
// It corresponds to vim.vm.device.VirtualPointingDevice.DeviceBackingOption.
type VirtualPointingDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`

	// HostPointingDevice describes the supported mouse types, including the
	// default supported mouse type.
	HostPointingDevice ChoiceOption `json:"hostPointingDevice"`
}

// VirtualPrecisionClockSystemClockBackingOption contains the options for using
// the host system clock as the backing for a virtual precision clock device.
// It corresponds to
// vim.vm.device.VirtualPrecisionClock.SystemClockBackingOption.
type VirtualPrecisionClockSystemClockBackingOption struct {
	// Protocol describes the supported protocols used to discipline the host
	// system clock.
	Protocol ChoiceOption `json:"protocol"`
}

// VirtualSCSIPassthroughDeviceBackingOption contains the options for host
// device backing of a virtual SCSI passthrough device.
// It corresponds to vim.vm.device.VirtualSCSIPassthrough.DeviceBackingOption.
type VirtualSCSIPassthroughDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualSerialPortDeviceBackingOption contains the options for host device
// backing of a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.DeviceBackingOption.
type VirtualSerialPortDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualSerialPortFileBackingOption contains the options for host file
// backing of a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.FileBackingOption.
type VirtualSerialPortFileBackingOption struct {
	VirtualDeviceFileBackingOption `json:",inline"`
}

// VirtualSerialPortPipeBackingOption contains the options for named pipe
// backing of a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.PipeBackingOption.
type VirtualSerialPortPipeBackingOption struct {
	VirtualDevicePipeBackingOption `json:",inline"`

	// Endpoint describes the choices and default setting for the pipe
	// endpoint. As an endpoint, the virtual machine can act as a client or a
	// server.
	Endpoint ChoiceOption `json:"endpoint"`

	// NoRxLoss indicates whether the server supports optimized data transfer
	// over the pipe and specifies the default behavior.
	NoRxLoss BoolOption `json:"noRxLoss"`
}

// VirtualSerialPortThinPrintBackingOption contains the options for ThinPrint
// device backing of a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.ThinPrintBackingOption,
// which carries no data beyond the base backing option.
type VirtualSerialPortThinPrintBackingOption struct{}

// VirtualSerialPortURIBackingOption contains the options for network URI
// backing of a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPort.URIBackingOption.
type VirtualSerialPortURIBackingOption struct {
	VirtualDeviceURIBackingOption `json:",inline"`
}

// VirtualSoundCardDeviceBackingOption contains the options for host device
// backing of a virtual sound card.
// It corresponds to vim.vm.device.VirtualSoundCard.DeviceBackingOption.
type VirtualSoundCardDeviceBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualUSBRemoteClientBackingOption contains the options for remote client
// host backing of a virtual USB device.
// It corresponds to vim.vm.device.VirtualUSB.RemoteClientBackingOption.
type VirtualUSBRemoteClientBackingOption struct {
	VirtualDeviceRemoteDeviceBackingOption `json:",inline"`
}

// VirtualUSBRemoteHostBackingOption contains the options for remote ESX host
// backing of a virtual USB device.
// It corresponds to vim.vm.device.VirtualUSB.RemoteHostBackingOption.
type VirtualUSBRemoteHostBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}

// VirtualUSBUSBBackingOption contains the options for host USB device backing
// of a virtual USB device.
// It corresponds to vim.vm.device.VirtualUSB.USBBackingOption.
type VirtualUSBUSBBackingOption struct {
	VirtualDeviceDeviceBackingOption `json:",inline"`
}
