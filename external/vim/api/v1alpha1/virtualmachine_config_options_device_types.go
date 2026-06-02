// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// VirtualDeviceOptionType identifies a type of virtual device option.
type VirtualDeviceOptionType string

const (
	// Base virtual device types
	VirtualDeviceOptionTypeDisk           = VirtualDeviceOptionType(VirtualDeviceTypeDisk + "Option")
	VirtualDeviceOptionTypeCdrom          = VirtualDeviceOptionType(VirtualDeviceTypeCdrom + "Option")
	VirtualDeviceOptionTypeFloppy         = VirtualDeviceOptionType(VirtualDeviceTypeFloppy + "Option")
	VirtualDeviceOptionTypeKeyboard       = VirtualDeviceOptionType(VirtualDeviceTypeKeyboard + "Option")
	VirtualDeviceOptionTypePointingDevice = VirtualDeviceOptionType(VirtualDeviceTypePointingDevice + "Option")
	VirtualDeviceOptionTypeSerialPort     = VirtualDeviceOptionType(VirtualDeviceTypeSerialPort + "Option")
	VirtualDeviceOptionTypeParallelPort   = VirtualDeviceOptionType(VirtualDeviceTypeParallelPort + "Option")
	VirtualDeviceOptionTypeSoundCard      = VirtualDeviceOptionType(VirtualDeviceTypeSoundCard + "Option")
	VirtualDeviceOptionTypeVideoCard      = VirtualDeviceOptionType(VirtualDeviceTypeVideoCard + "Option")
	VirtualDeviceOptionTypePCIPassthrough = VirtualDeviceOptionType(VirtualDeviceTypePCIPassthrough + "Option")
	VirtualDeviceOptionTypePrecisionClock = VirtualDeviceOptionType(VirtualDeviceTypePrecisionClock + "Option")
	VirtualDeviceOptionTypeNVDIMM         = VirtualDeviceOptionType(VirtualDeviceTypeNVDIMM + "Option")
	VirtualDeviceOptionTypeVMCIDevice     = VirtualDeviceOptionType(VirtualDeviceTypeVMCIDevice + "Option")
	VirtualDeviceOptionTypeTPM            = VirtualDeviceOptionType(VirtualDeviceTypeTPM + "Option")
	VirtualDeviceOptionTypeUSB            = VirtualDeviceOptionType(VirtualDeviceTypeUSB + "Option")
	VirtualDeviceOptionTypeWDT            = VirtualDeviceOptionType(VirtualDeviceTypeWDT + "Option")

	// Controller sub-types
	VirtualDeviceOptionTypeAHCIController            = VirtualDeviceOptionType(VirtualDeviceTypeAHCIController + "Option")
	VirtualDeviceOptionTypeBusLogicController        = VirtualDeviceOptionType(VirtualDeviceTypeBusLogicController + "Option")
	VirtualDeviceOptionTypeIDEController             = VirtualDeviceOptionType(VirtualDeviceTypeIDEController + "Option")
	VirtualDeviceOptionTypeLsiLogicController        = VirtualDeviceOptionType(VirtualDeviceTypeLsiLogicController + "Option")
	VirtualDeviceOptionTypeLsiLogicSASController     = VirtualDeviceOptionType(VirtualDeviceTypeLsiLogicSASController + "Option")
	VirtualDeviceOptionTypeNVMeController            = VirtualDeviceOptionType(VirtualDeviceTypeNVMeController + "Option")
	VirtualDeviceOptionTypeParaVirtualSCSIController = VirtualDeviceOptionType(VirtualDeviceTypeParaVirtualSCSIController + "Option")
	VirtualDeviceOptionTypePCIController             = VirtualDeviceOptionType(VirtualDeviceTypePCIController + "Option")
	VirtualDeviceOptionTypePS2Controller             = VirtualDeviceOptionType(VirtualDeviceTypePS2Controller + "Option")
	VirtualDeviceOptionTypeSATAController            = VirtualDeviceOptionType(VirtualDeviceTypeSATAController + "Option")
	VirtualDeviceOptionTypeSIOController             = VirtualDeviceOptionType(VirtualDeviceTypeSIOController + "Option")
	VirtualDeviceOptionTypeUSBController             = VirtualDeviceOptionType(VirtualDeviceTypeUSBController + "Option")
	VirtualDeviceOptionTypeUSBXHCIController         = VirtualDeviceOptionType(VirtualDeviceTypeUSBXHCIController + "Option")

	// Ethernet card sub-types
	VirtualDeviceOptionTypeE1000             = VirtualDeviceOptionType(VirtualDeviceTypeE1000 + "Option")
	VirtualDeviceOptionTypeE1000e            = VirtualDeviceOptionType(VirtualDeviceTypeE1000e + "Option")
	VirtualDeviceOptionTypePCNet32           = VirtualDeviceOptionType(VirtualDeviceTypePCNet32 + "Option")
	VirtualDeviceOptionTypeSriovEthernetCard = VirtualDeviceOptionType(VirtualDeviceTypeSriovEthernetCard + "Option")
	VirtualDeviceOptionTypeVmxnet            = VirtualDeviceOptionType(VirtualDeviceTypeVmxnet + "Option")
	VirtualDeviceOptionTypeVmxnet2           = VirtualDeviceOptionType(VirtualDeviceTypeVmxnet2 + "Option")
	VirtualDeviceOptionTypeVmxnet3           = VirtualDeviceOptionType(VirtualDeviceTypeVmxnet3 + "Option")
	VirtualDeviceOptionTypeVmxnet3Vrdma      = VirtualDeviceOptionType(VirtualDeviceTypeVmxnet3Vrdma + "Option")

	// Sound card sub-types
	VirtualDeviceOptionTypeSoundBlaster16 = VirtualDeviceOptionType(VirtualDeviceTypeSoundBlaster16 + "Option")
	VirtualDeviceOptionTypeEnsoniq1371    = VirtualDeviceOptionType(VirtualDeviceTypeEnsoniq1371 + "Option")
	VirtualDeviceOptionTypeHdAudioCard    = VirtualDeviceOptionType(VirtualDeviceTypeHdAudioCard + "Option")
)

// VirtualDeviceOption describes the options for a virtual device type,
// including configuration options and its relationship to other devices.
// It corresponds to vim.vm.device.VirtualDeviceOption.
type VirtualDeviceOption struct {
	// +optional

	// DeviceType is the name of the run-time class to instantiate for this
	// device.
	DeviceType VirtualDeviceType `json:"deviceType,omitempty"`

	// +optional

	// ConnectOption describes the connect options and defaults for connectable
	// devices.
	ConnectOption *VirtualDeviceConnectOption `json:"connectOption,omitempty"`

	// +optional

	// BusSlotOption describes the bus slot options when the device can use a
	// bus slot configuration.
	BusSlotOption *VirtualDeviceBusSlotOption `json:"busSlotOption,omitempty"`

	// +optional

	// ControllerType is the data object type of the controller option that is
	// valid for controlling this device.
	ControllerType VirtualControllerType `json:"controllerType,omitempty"`

	// +optional

	// AutoAssignController indicates whether this device will be
	// auto-assigned a controller if one is required.
	AutoAssignController *BoolOption `json:"autoAssignController,omitempty"`

	// +optional
	// +kubebuilder:validation:MaxItems=128

	// BackingOption lists the backing options that can be used to map the
	// virtual device to the host. The list is empty for devices that exist
	// only within the virtual machine, such as a controller.
	BackingOption []VirtualDeviceBackingOption `json:"backingOption,omitempty"`

	// +optional

	// DefaultBackingOptionIndex is an index into the backing option list that
	// indicates the default backing.
	DefaultBackingOptionIndex int32 `json:"defaultBackingOptionIndex,omitempty"`

	// +optional

	// LicensingLimit lists property names limited by a licensing restriction
	// of the underlying product.
	LicensingLimit []string `json:"licensingLimit,omitempty"`

	// +optional

	// Deprecated indicates whether this device type is deprecated and cannot
	// be used when creating or reconfiguring a virtual machine.
	Deprecated bool `json:"deprecated,omitempty"`

	// +optional

	// PlugAndPlay indicates whether this device type can be hot-added to a
	// running virtual machine.
	PlugAndPlay bool `json:"plugAndPlay,omitempty"`

	// +optional

	// HotRemoveSupported indicates whether this device type can be
	// hot-removed from a running virtual machine.
	HotRemoveSupported bool `json:"hotRemoveSupported,omitempty"`

	// +optional

	// NumaSupported indicates whether NUMA affinity is supported for this
	// device.
	NumaSupported *bool `json:"numaSupported,omitempty"`

	// +required

	// Type is the type of the device option to use.
	Type VirtualDeviceOptionType `json:"type"`

	// Base virtual device types
	VirtualCdromOption             *VirtualCdromOption             `json:"virtualCdromOption,omitempty"`
	VirtualDiskOption              *VirtualDiskOption              `json:"virtualDiskOption,omitempty"`
	VirtualFloppyOption            *VirtualFloppyOption            `json:"virtualFloppyOption,omitempty"`
	VirtualKeyboardOption          *VirtualKeyboardOption          `json:"virtualKeyboardOption,omitempty"`
	VirtualPointingDeviceOption    *VirtualPointingDeviceOption    `json:"virtualPointingDeviceOption,omitempty"`
	VirtualSerialPortOption        *VirtualSerialPortOption        `json:"virtualSerialPortOption,omitempty"`
	VirtualParallelPortOption      *VirtualParallelPortOption      `json:"virtualParallelPortOption,omitempty"`
	VirtualSoundCardOption         *VirtualSoundCardOption         `json:"virtualSoundCardOption,omitempty"`
	VirtualVideoCardOption         *VirtualVideoCardOption         `json:"virtualVideoCardOption,omitempty"`
	VirtualPCIPassthroughOption    *VirtualPCIPassthroughOption    `json:"virtualPCIPassthroughOption,omitempty"`
	VirtualPrecisionClockOption    *VirtualPrecisionClockOption    `json:"virtualPrecisionClockOption,omitempty"`
	VirtualNVDIMMOption            *VirtualNVDIMMOption            `json:"virtualNVDIMMOption,omitempty"`
	VirtualMachineVMCIDeviceOption *VirtualMachineVMCIDeviceOption `json:"virtualMachineVMCIDeviceOption,omitempty"`
	VirtualTPMOption               *VirtualTPMOption               `json:"virtualTPMOption,omitempty"`
	VirtualUSBOption               *VirtualUSBOption               `json:"virtualUSBOption,omitempty"`
	VirtualWDTOption               *VirtualWDTOption               `json:"virtualWDTOption,omitempty"`

	// Controller sub-types
	VirtualPCIControllerOption *VirtualPCIControllerOption `json:"virtualPCIControllerOption,omitempty"`
	VirtualPS2ControllerOption *VirtualPS2ControllerOption `json:"virtualPS2ControllerOption,omitempty"`
	VirtualSIOControllerOption *VirtualSIOControllerOption `json:"virtualSIOControllerOption,omitempty"`

	// IDE controller sub-types
	VirtualIDEControllerOption *VirtualIDEControllerOption `json:"virtualIDEControllerOption,omitempty"`

	// SATA controller sub-types
	VirtualSATAControllerOption *VirtualSATAControllerOption `json:"virtualSATAControllerOption,omitempty"`
	VirtualAHCIControllerOption *VirtualAHCIControllerOption `json:"virtualAHCIControllerOption,omitempty"`

	// NVMe controller sub-types
	VirtualNVMEControllerOption *VirtualNVMEControllerOption `json:"virtualNVMEControllerOption,omitempty"`

	// SCSI controller sub-types
	VirtualBusLogicControllerOption    *VirtualBusLogicControllerOption    `json:"virtualBusLogicControllerOption,omitempty"`
	VirtualLsiLogicControllerOption    *VirtualLsiLogicControllerOption    `json:"virtualLsiLogicControllerOption,omitempty"`
	VirtualLsiLogicSASControllerOption *VirtualLsiLogicSASControllerOption `json:"virtualLsiLogicSASControllerOption,omitempty"`
	ParaVirtualSCSIControllerOption    *ParaVirtualSCSIControllerOption    `json:"paraVirtualSCSIControllerOption,omitempty"`

	// USB controller sub-types
	VirtualUSBControllerOption     *VirtualUSBControllerOption     `json:"virtualUSBControllerOption,omitempty"`
	VirtualUSBXHCIControllerOption *VirtualUSBXHCIControllerOption `json:"virtualUSBXHCIControllerOption,omitempty"`

	// Ethernet card sub-types
	VirtualE1000Option             *VirtualE1000Option             `json:"virtualE1000Option,omitempty"`
	VirtualE1000eOption            *VirtualE1000eOption            `json:"virtualE1000eOption,omitempty"`
	VirtualPCNet32Option           *VirtualPCNet32Option           `json:"virtualPCNet32Option,omitempty"`
	VirtualSriovEthernetCardOption *VirtualSriovEthernetCardOption `json:"virtualSriovEthernetCardOption,omitempty"`
	VirtualVmxnetOption            *VirtualVmxnetOption            `json:"virtualVmxnetOption,omitempty"`
	VirtualVmxnet2Option           *VirtualVmxnet2Option           `json:"virtualVmxnet2Option,omitempty"`
	VirtualVmxnet3Option           *VirtualVmxnet3Option           `json:"virtualVmxnet3Option,omitempty"`
	VirtualVmxnet3VrdmaOption      *VirtualVmxnet3VrdmaOption      `json:"virtualVmxnet3VrdmaOption,omitempty"`

	// Sound card sub-types
	VirtualSoundBlaster16Option *VirtualSoundBlaster16Option `json:"virtualSoundBlaster16Option,omitempty"`
	VirtualEnsoniq1371Option    *VirtualEnsoniq1371Option    `json:"virtualEnsoniq1371Option,omitempty"`
	VirtualHdAudioCardOption    *VirtualHdAudioCardOption    `json:"virtualHdAudioCardOption,omitempty"`
}

// VirtualDeviceBusSlotOption describes the bus slot options for a virtual
// device. It corresponds to vim.vm.device.VirtualDeviceBusSlotOption.
type VirtualDeviceBusSlotOption struct {
	// +optional

	// Type is the name of the run-time class to instantiate for the device's
	// bus slot object.
	Type string `json:"type,omitempty"`
}

// VirtualDeviceConnectOption describes the connect options for a connectable
// virtual device. It corresponds to vim.vm.device.VirtualDeviceOption.ConnectOption.
type VirtualDeviceConnectOption struct {
	// StartConnected indicates whether the device supports the
	// startConnected feature.
	StartConnected BoolOption `json:"startConnected"`

	// AllowGuestControl indicates whether the device can be connected or
	// disconnected from within the guest operating system.
	AllowGuestControl BoolOption `json:"allowGuestControl"`
}

// VirtualMachineVMCIDeviceOption describes the options for a VMCI device.
// It corresponds to vim.vm.VirtualMachineVMCIDeviceOption.
type VirtualMachineVMCIDeviceOption struct {
	// AllowUnrestrictedCommunication indicates support for VMCI communication
	// with all other virtual machines on the host.
	AllowUnrestrictedCommunication BoolOption `json:"allowUnrestrictedCommunication"`

	// +optional

	// FilterSpecOption describes the available options for each VMCI firewall
	// filter specification.
	FilterSpecOption *VirtualMachineVMCIDeviceOptionFilterSpecOption `json:"filterSpecOption,omitempty"`

	// +optional

	// FilterSupported indicates support for VMCI firewall filters.
	FilterSupported *BoolOption `json:"filterSupported,omitempty"`
}

// VirtualNVDIMMOption describes the options for a virtual NVDIMM device.
// It corresponds to vim.vm.device.VirtualNVDIMMOption.
type VirtualNVDIMMOption struct {
	// Capacity describes the minimum and maximum capacity.
	Capacity ResourceQuantityOption `json:"capacity"`

	// Growable indicates whether capacity growth is supported for
	// powered-off virtual machines.
	Growable bool `json:"growable"`

	// HotGrowable indicates whether capacity growth is supported for
	// powered-on virtual machines.
	HotGrowable bool `json:"hotGrowable"`

	// Granularity is the capacity growth granularity, if
	// growth is supported.
	Granularity ResourceQuantityOption `json:"granularity"`
}

// VirtualSerialPortOption describes the options for a virtual serial port.
// It corresponds to vim.vm.device.VirtualSerialPortOption.
type VirtualSerialPortOption struct {
	// YieldOnPoll indicates whether the virtual machine supports CPU yield
	// behavior when polling the virtual serial port.
	YieldOnPoll BoolOption `json:"yieldOnPoll"`
}

// VirtualTPMOption describes the options for a virtual TPM device.
// It corresponds to vim.vm.device.VirtualTPMOption.
type VirtualTPMOption struct {
	// +optional

	// SupportedFirmware lists the supported firmware types for this TPM device,
	// using GuestOsDescriptorFirmwareType enumeration values.
	SupportedFirmware []Firmware `json:"supportedFirmware,omitempty"`
}

// VirtualVideoCardOption describes the options for a virtual video card.
// It corresponds to vim.vm.device.VirtualVideoCardOption.
type VirtualVideoCardOption struct {
	// +optional

	// VideoRamSize describes the minimum, maximum, and default video
	// frame buffer size.
	VideoRamSize *ResourceQuantityOption `json:"videoRamSize,omitempty"`

	// +optional

	// NumDisplays describes the minimum, maximum, and default number of
	// displays.
	NumDisplays *IntOption `json:"numDisplays,omitempty"`

	// +optional

	// UseAutoDetect indicates whether host display settings can be used to
	// automatically determine the virtual video card display settings.
	UseAutoDetect *BoolOption `json:"useAutoDetect,omitempty"`

	// +optional

	// Support3D indicates whether the virtual video card supports 3D
	// functions.
	Support3D *BoolOption `json:"support3D,omitempty"`

	// +optional

	// Use3dRendererSupported indicates whether the virtual video card can
	// specify how to render 3D graphics.
	Use3dRendererSupported *BoolOption `json:"use3dRendererSupported,omitempty"`

	// +optional

	// GraphicsMemorySize describes the minimum, maximum, and default
	// graphics memory size.
	GraphicsMemorySize *ResourceQuantityOption `json:"graphicsMemorySize,omitempty"`

	// +optional

	// GraphicsMemorySizeSupported indicates whether the virtual video card
	// can specify the graphics memory size.
	GraphicsMemorySizeSupported *BoolOption `json:"graphicsMemorySizeSupported,omitempty"`
}

// VirtualWDTOption describes the options for a virtual watchdog timer device.
// It corresponds to vim.vm.device.VirtualWDTOption.
type VirtualWDTOption struct {
	// RunOnBoot indicates whether the "run on boot" option is settable on
	// this device.
	RunOnBoot BoolOption `json:"runOnBoot"`
}

// VirtualMachineVMCIDeviceOptionFilterSpecOption describes the available
// options for a VMCI firewall filter specification.
// It corresponds to vim.vm.VirtualMachineVMCIDeviceOption.FilterSpecOption.
type VirtualMachineVMCIDeviceOptionFilterSpecOption struct {
	// Action describes the available filter actions.
	Action ChoiceOption `json:"action"`

	// Protocol describes the available filter protocols.
	Protocol ChoiceOption `json:"protocol"`

	// Direction describes the available filter directions.
	Direction ChoiceOption `json:"direction"`

	// LowerDstPortBoundary describes the range for the lower destination
	// port boundary.
	LowerDstPortBoundary LongOption `json:"lowerDstPortBoundary"`

	// UpperDstPortBoundary describes the range for the upper destination
	// port boundary.
	UpperDstPortBoundary LongOption `json:"upperDstPortBoundary"`
}

// VirtualCdromOption describes the options for a CD-ROM device.
// It corresponds to vim.vm.device.VirtualCdromOption, which carries no data
// beyond the base device option.
type VirtualCdromOption struct{}

// VirtualFloppyOption describes the options for a floppy disk drive.
// It corresponds to vim.vm.device.VirtualFloppyOption, which carries no data
// beyond the base device option.
type VirtualFloppyOption struct{}

// VirtualKeyboardOption describes the options for a keyboard device.
// It corresponds to vim.vm.device.VirtualKeyboardOption, which carries no data
// beyond the base device option.
type VirtualKeyboardOption struct{}

// VirtualPointingDeviceOption describes the options for a pointing device
// (mouse).
// It corresponds to vim.vm.device.VirtualPointingDeviceOption, which carries
// no data beyond the base device option.
type VirtualPointingDeviceOption struct{}

// VirtualParallelPortOption describes the options for a parallel port.
// It corresponds to vim.vm.device.VirtualParallelPortOption, which carries no
// data beyond the base device option.
type VirtualParallelPortOption struct{}

// VirtualPCIPassthroughOption describes the options for a PCI passthrough
// device.
// It corresponds to vim.vm.device.VirtualPCIPassthroughOption, which carries
// no data beyond the base device option.
type VirtualPCIPassthroughOption struct{}

// VirtualPrecisionClockOption describes the options for a precision clock
// device.
// It corresponds to vim.vm.device.VirtualPrecisionClockOption, which carries
// no data beyond the base device option.
type VirtualPrecisionClockOption struct{}

// VirtualUSBOption describes the options for a USB device.
// It corresponds to vim.vm.device.VirtualUSBOption, which carries no data
// beyond the base device option.
type VirtualUSBOption struct{}

// VirtualSoundCardOption is the base data object for the options of a sound
// card device.
// It corresponds to vim.vm.device.VirtualSoundCardOption, which carries no
// data beyond the base device option.
type VirtualSoundCardOption struct{}

// VirtualSoundBlaster16Option describes the options for a Sound Blaster 16
// sound card.
// It corresponds to vim.vm.device.VirtualSoundBlaster16Option, which carries
// no data beyond the base sound card option.
type VirtualSoundBlaster16Option struct{}

// VirtualEnsoniq1371Option describes the options for an Ensoniq 1371 sound
// card.
// It corresponds to vim.vm.device.VirtualEnsoniq1371Option, which carries no
// data beyond the base sound card option.
type VirtualEnsoniq1371Option struct{}

// VirtualHdAudioCardOption describes the options for an Intel HD Audio sound
// card.
// It corresponds to vim.vm.device.VirtualHdAudioCardOption, which carries no
// data beyond the base sound card option.
type VirtualHdAudioCardOption struct{}
