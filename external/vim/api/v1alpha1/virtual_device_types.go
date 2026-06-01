// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import "k8s.io/apimachinery/pkg/api/resource"

// VirtualDeviceType identifies the concrete runtime type of a virtual
// device in a virtual machine. The values correspond to vSphere API
// device class names (e.g. vim.vm.device.VirtualDisk).
type VirtualDeviceType string

const (
	// Base virtual device types
	VirtualDeviceTypeDisk           VirtualDeviceType = "VirtualDisk"
	VirtualDeviceTypeCdrom          VirtualDeviceType = "VirtualCdrom"
	VirtualDeviceTypeFloppy         VirtualDeviceType = "VirtualFloppy"
	VirtualDeviceTypeKeyboard       VirtualDeviceType = "VirtualKeyboard"
	VirtualDeviceTypePointingDevice VirtualDeviceType = "VirtualPointingDevice"
	VirtualDeviceTypeSerialPort     VirtualDeviceType = "VirtualSerialPort"
	VirtualDeviceTypeParallelPort   VirtualDeviceType = "VirtualParallelPort"
	VirtualDeviceTypeSoundCard      VirtualDeviceType = "VirtualSoundCard"
	VirtualDeviceTypeVideoCard      VirtualDeviceType = "VirtualVideoCard"
	VirtualDeviceTypePCIPassthrough VirtualDeviceType = "VirtualPCIPassthrough"
	VirtualDeviceTypePrecisionClock VirtualDeviceType = "VirtualPrecisionClock"
	VirtualDeviceTypeNVDIMM         VirtualDeviceType = "VirtualNVDIMM"
	VirtualDeviceTypeVMCIDevice     VirtualDeviceType = "VirtualMachineVMCIDevice"
	VirtualDeviceTypeTPM            VirtualDeviceType = "VirtualTPM"
	VirtualDeviceTypeUSB            VirtualDeviceType = "VirtualUSB"
	VirtualDeviceTypeWDT            VirtualDeviceType = "VirtualWDT"

	// Controller sub-types
	VirtualDeviceTypeAHCIController            VirtualDeviceType = "VirtualAHCIController"
	VirtualDeviceTypeBusLogicController        VirtualDeviceType = "VirtualBusLogicController"
	VirtualDeviceTypeIDEController             VirtualDeviceType = "VirtualIDEController"
	VirtualDeviceTypeLsiLogicController        VirtualDeviceType = "VirtualLsiLogicController"
	VirtualDeviceTypeLsiLogicSASController     VirtualDeviceType = "VirtualLsiLogicSASController"
	VirtualDeviceTypeNVMeController            VirtualDeviceType = "VirtualNVMeController"
	VirtualDeviceTypeParaVirtualSCSIController VirtualDeviceType = "ParaVirtualSCSIController"
	VirtualDeviceTypePCIController             VirtualDeviceType = "VirtualPCIController"
	VirtualDeviceTypePS2Controller             VirtualDeviceType = "VirtualPS2Controller"
	VirtualDeviceTypeSATAController            VirtualDeviceType = "VirtualSATAController"
	VirtualDeviceTypeSIOController             VirtualDeviceType = "VirtualSIOController"
	VirtualDeviceTypeUSBController             VirtualDeviceType = "VirtualUSBController"
	VirtualDeviceTypeUSBXHCIController         VirtualDeviceType = "VirtualUSBXHCIController"

	// Ethernet card sub-types
	VirtualDeviceTypeE1000             VirtualDeviceType = "VirtualE1000"
	VirtualDeviceTypeE1000e            VirtualDeviceType = "VirtualE1000e"
	VirtualDeviceTypePCNet32           VirtualDeviceType = "VirtualPCNet32"
	VirtualDeviceTypeSriovEthernetCard VirtualDeviceType = "VirtualSriovEthernetCard"
	VirtualDeviceTypeVmxnet            VirtualDeviceType = "VirtualVmxnet"
	VirtualDeviceTypeVmxnet2           VirtualDeviceType = "VirtualVmxnet2"
	VirtualDeviceTypeVmxnet3           VirtualDeviceType = "VirtualVmxnet3"
	VirtualDeviceTypeVmxnet3Vrdma      VirtualDeviceType = "VirtualVmxnet3Vrdma"

	// Sound card sub-types
	VirtualDeviceTypeSoundBlaster16 VirtualDeviceType = "VirtualSoundBlaster16"
	VirtualDeviceTypeEnsoniq1371    VirtualDeviceType = "VirtualEnsoniq1371"
	VirtualDeviceTypeHdAudioCard    VirtualDeviceType = "VirtualHdAudioCard"
)

// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualE1000' || d.type == 'VirtualE1000e' || d.type == 'VirtualPCNet32' || d.type == 'VirtualSriovEthernetCard' || d.type == 'VirtualVmxnet' || d.type == 'VirtualVmxnet2' || d.type == 'VirtualVmxnet3' || d.type == 'VirtualVmxnet3Vrdma').size() <= 26",message="devices must not contain more than 26 ethernet cards"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualSriovEthernetCard').size() <= 16",message="devices must not contain more than 16 SR-IOV ethernet cards"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualE1000' || d.type == 'VirtualE1000e' || d.type == 'VirtualPCNet32' || d.type == 'VirtualVmxnet' || d.type == 'VirtualVmxnet2' || d.type == 'VirtualVmxnet3' || d.type == 'VirtualVmxnet3Vrdma').size() <= 10",message="devices must not contain more than 10 non-SR-IOV ethernet cards"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualIDEController').size() <= 2",message="devices must not contain more than 2 IDE controllers"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualLsiLogicController' || d.type == 'VirtualLsiLogicSASController' || d.type == 'VirtualBusLogicController' || d.type == 'ParaVirtualSCSIController').size() <= 4",message="devices must not contain more than 4 SCSI controllers"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualNVMeController').size() <= 4",message="devices must not contain more than 4 NVMe controllers"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualSIOController').size() <= 1",message="devices must not contain more than 1 SIO controller"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualPCIController').size() <= 1",message="devices must not contain more than 1 PCI controller"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualPS2Controller').size() <= 1",message="devices must not contain more than 1 PS2 controller"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualUSBController').size() <= 1",message="devices must not contain more than 1 USB controller"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualUSBXHCIController').size() <= 1",message="devices must not contain more than 1 USB XHCI controller"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualSATAController' || d.type == 'VirtualAHCIController').size() <= 4",message="devices must not contain more than 4 SATA controllers"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualUSB').size() <= 20",message="devices must not contain more than 20 USB devices"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualDisk').size() <= 1208",message="devices must not contain more than 1208 virtual disks"
// - 100 SATA, 60 NVMe, 1024 SCSI, 4 IDE, 20 USB
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualCdrom').size() <= 124",message="devices must not contain more than 124 CD-ROM devices"
// - 4 IDE, 20 USB, 100 SATA
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualKeyboard').size() <= 1",message="devices must not contain more than 1 keyboard"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualPointingDevice').size() <= 1",message="devices must not contain more than 1 pointing device"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualSerialPort').size() <= 32",message="devices must not contain more than 32 serial ports"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualParallelPort').size() <= 4",message="devices must not contain more than 4 parallel ports"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualSoundBlaster16' || d.type == 'VirtualHdAudioCard' || d.type == 'VirtualEnsoniq1371').size() <= 1",message="devices must not contain more than 1 sound card"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualVideoCard').size() <= 1",message="devices must not contain more than 1 video card"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualPCIPassthrough').size() <= 128",message="devices must not contain more than 128 PCI passthrough devices"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualPrecisionClock').size() <= 1",message="devices must not contain more than 1 precision clock"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualNVDIMM').size() <= 64",message="devices must not contain more than 64 NVDIMM devices"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualTPM').size() <= 1",message="devices must not contain more than 1 TPM"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualMachineVMCIDevice').size() <= 1",message="devices must not contain more than 1 VMCI device"
// +kubebuilder:validation:XValidation:rule="self.filter(d, d.type == 'VirtualWDT').size() <= 1",message="devices must not contain more than 1 watchdog timer"
type VirtualDevices []VirtualDevice

// +kubebuilder:validation:XValidation:rule="[has(self.virtualDisk), has(self.virtualSerialPort), has(self.virtualMachineVideoCard), has(self.virtualNVDIMM), has(self.virtualMachineVMCIDevice), has(self.virtualTPM), has(self.virtualUSB), has(self.virtualWDT), has(self.virtualPCIController), has(self.virtualPS2Controller), has(self.virtualSIOController), has(self.virtualIDEController), has(self.virtualSATAController), has(self.virtualAHCIController), has(self.virtualNVMEController), has(self.virtualBusLogicController), has(self.virtualLsiLogicController), has(self.virtualLsiLogicSASController), has(self.paraVirtualSCSIController), has(self.virtualUSBController), has(self.virtualUSBXHCIController), has(self.virtualE1000), has(self.virtualE1000e), has(self.virtualPCNet32), has(self.virtualSriovEthernetCard), has(self.virtualVmxnet), has(self.virtualVmxnet2), has(self.virtualVmxnet3), has(self.virtualVmxnet3Vrdma)].filter(x, x).size() <= 1",message="at most one device-specific data field may be specified"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || self.type != 'VirtualVideoCard' || self.unitNumber == 0",message="video card unit number must be 0"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || (self.type != 'VirtualSoundBlaster16' && self.type != 'VirtualEnsoniq1371' && self.type != 'VirtualHdAudioCard') || self.unitNumber == 2",message="sound card unit number must be 2"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || (self.type != 'VirtualBusLogicController' && self.type != 'VirtualLsiLogicController' && self.type != 'VirtualLsiLogicSASController' && self.type != 'ParaVirtualSCSIController') || (self.unitNumber >= 3 && self.unitNumber <= 6)",message="SCSI HBA unit number must be in [3, 6]"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || (self.type != 'VirtualE1000' && self.type != 'VirtualE1000e' && self.type != 'VirtualPCNet32' && self.type != 'VirtualVmxnet' && self.type != 'VirtualVmxnet2' && self.type != 'VirtualVmxnet3' && self.type != 'VirtualVmxnet3Vrdma') || (self.unitNumber >= 7 && self.unitNumber <= 16)",message="ethernet NIC unit number must be in [7, 16]"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || self.type != 'VirtualMachineVMCIDevice' || self.unitNumber == 17",message="VMCI device unit number must be 17"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || (self.type != 'VirtualPCIPassthrough' && self.type != 'VirtualSriovEthernetCard') || (self.unitNumber >= 18 && self.unitNumber <= 21) || (self.unitNumber >= 38 && self.unitNumber <= 161)",message="PCI passthrough and SR-IOV NIC unit number must be in [18, 21] or [38, 161]"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || self.type != 'VirtualUSBController' || self.unitNumber == 22",message="USB (UHCI/EHCI) controller unit number must be 22"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || self.type != 'VirtualUSBXHCIController' || self.unitNumber == 23",message="XHCI controller unit number must be 23"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || (self.type != 'VirtualSATAController' && self.type != 'VirtualAHCIController') || (self.unitNumber >= 24 && self.unitNumber <= 27)",message="SATA HBA unit number must be in [24, 27]"
// +kubebuilder:validation:XValidation:rule="!has(self.unitNumber) || self.type != 'VirtualNVMeController' || (self.unitNumber >= 30 && self.unitNumber <= 33)",message="NVMe HBA unit number must be in [30, 33]"

// VirtualDevice is the base data object type for devices in a virtual machine.
// It corresponds to vim.vm.device.VirtualDevice.
type VirtualDevice struct {
	// +optional

	// ControllerType is the type of the controller to which this device should
	// be attached.
	ControllerType VirtualControllerType `json:"controllerType,omitempty"`

	// +optional

	// ControllerBusNumber is the bus number of the controller to which this
	// device should be attached.
	ControllerBusNumber *int32 `json:"controllerBusNumber,omitempty"`

	// +optional

	// Backing describes the device's backing information.
	Backing *VirtualDeviceBackingInfo `json:"backing,omitempty"`

	// +optional

	// DeviceInfo provides a label and summary for the device.
	DeviceInfo *VirtualDeviceDescription `json:"deviceInfo,omitempty"`

	// +optional

	// Connectable provides information about the connect/disconnect behavior
	// of the device while the virtual machine is running.
	Connectable *VirtualDeviceConnectInfo `json:"connectable,omitempty"`

	// +optional

	// SlotInfo describes the bus slot of the device in the virtual machine.
	SlotInfo *VirtualDeviceBusSlotInfo `json:"slotInfo,omitempty"`

	// +optional

	// UnitNumber is the unit number of this device on its controller.
	UnitNumber *int32 `json:"unitNumber,omitempty"`

	// +optional

	// NUMANode is the virtual NUMA node for this device.
	// A negative number indicates no NUMA affinity.
	NUMANode int32 `json:"numaNode,omitempty"`

	// +optional

	// DeviceGroupInfo describes the device group this device belongs to.
	// Devices in a group must be added or removed as a unit.
	DeviceGroupInfo *VirtualDeviceDeviceGroupInfo `json:"deviceGroupInfo,omitempty"`

	// +required

	Type VirtualDeviceType `json:"type"`

	// Base virtual device types
	VirtualCDROM             *VirtualCDROM             `json:"virtualCDROM,omitempty"`
	VirtualDisk              *VirtualDisk              `json:"virtualDisk,omitempty"`
	VirtualFloppy            *VirtualFloppy            `json:"virtualFloppy,omitempty"`
	VirtualMachineVideoCard  *VirtualMachineVideoCard  `json:"virtualMachineVideoCard,omitempty"`
	VirtualNVDIMM            *VirtualNVDIMM            `json:"virtualNVDIMM,omitempty"`
	VirtualMachineVMCIDevice *VirtualMachineVMCIDevice `json:"virtualMachineVMCIDevice,omitempty"`
	VirtualPCIPassthrough    *VirtualPCIPassthrough    `json:"virtualPCIPassthrough,omitempty"`
	VirtualPrecisionClock    *VirtualPrecisionClock    `json:"virtualPrecisionClock,omitempty"`
	VirtualTPM               *VirtualTPM               `json:"virtualTPM,omitempty"`
	VirtualUSB               *VirtualUSB               `json:"virtualUSB,omitempty"`
	VirtualWDT               *VirtualWDT               `json:"virtualWDT,omitempty"`

	// Port sub-types
	VirtualParallelPort *VirtualParallelPort `json:"virtualParallelPort,omitempty"`
	VirtualSerialPort   *VirtualSerialPort   `json:"virtualSerialPort,omitempty"`

	// Input device sub-types
	VirtualKeyboard       *VirtualKeyboard       `json:"virtualKeyboard,omitempty"`
	VirtualPointingDevice *VirtualPointingDevice `json:"virtualPointingDevice,omitempty"`

	// Controller sub-types
	VirtualIDEController *VirtualIDEController `json:"virtualIDEController,omitempty"`
	VirtualPCIController *VirtualPCIController `json:"virtualPCIController,omitempty"`
	VirtualPS2Controller *VirtualPS2Controller `json:"virtualPS2Controller,omitempty"`
	VirtualSIOController *VirtualSIOController `json:"virtualSIOController,omitempty"`

	// SATA controller sub-types
	VirtualSATAController *VirtualSATAController `json:"virtualSATAController,omitempty"`
	VirtualAHCIController *VirtualAHCIController `json:"virtualAHCIController,omitempty"`

	// NVMe controller sub-types
	VirtualNVMEController *VirtualNVMEController `json:"virtualNVMEController,omitempty"`

	// SCSI controller sub-types
	VirtualBusLogicController    *VirtualBusLogicController    `json:"virtualBusLogicController,omitempty"`
	VirtualLsiLogicController    *VirtualLsiLogicController    `json:"virtualLsiLogicController,omitempty"`
	VirtualLsiLogicSASController *VirtualLsiLogicSASController `json:"virtualLsiLogicSASController,omitempty"`
	ParaVirtualSCSIController    *ParaVirtualSCSIController    `json:"paraVirtualSCSIController,omitempty"`

	// USB controller sub-types
	VirtualUSBController     *VirtualUSBController     `json:"virtualUSBController,omitempty"`
	VirtualUSBXHCIController *VirtualUSBXHCIController `json:"virtualUSBXHCIController,omitempty"`

	// Ethernet card sub-types
	VirtualE1000             *VirtualE1000             `json:"virtualE1000,omitempty"`
	VirtualE1000e            *VirtualE1000e            `json:"virtualE1000e,omitempty"`
	VirtualPCNet32           *VirtualPCNet32           `json:"virtualPCNet32,omitempty"`
	VirtualSriovEthernetCard *VirtualSriovEthernetCard `json:"virtualSriovEthernetCard,omitempty"`
	VirtualVmxnet            *VirtualVmxnet            `json:"virtualVmxnet,omitempty"`
	VirtualVmxnet2           *VirtualVmxnet2           `json:"virtualVmxnet2,omitempty"`
	VirtualVmxnet3           *VirtualVmxnet3           `json:"virtualVmxnet3,omitempty"`
	VirtualVmxnet3Vrdma      *VirtualVmxnet3Vrdma      `json:"virtualVmxnet3Vrdma,omitempty"`

	// Sound card sub-types
	VirtualSoundBlaster16 *VirtualSoundBlaster16 `json:"virtualSoundBlaster16,omitempty"`
	VirtualEnsoniq1371    *VirtualEnsoniq1371    `json:"virtualEnsoniq1371,omitempty"`
	VirtualHdAudioCard    *VirtualHdAudioCard    `json:"virtualHdAudioCard,omitempty"`
}

// VirtualDeviceDescription provides a label and summary for a virtual device.
// It corresponds to vim.Description.
type VirtualDeviceDescription struct {
	// Label is the display label.
	Label string `json:"label"`

	// Summary is the summary description.
	Summary string `json:"summary"`
}

// VirtualDeviceConnectInfo contains information about connecting and
// disconnecting a virtual device.
// It corresponds to vim.vm.device.VirtualDevice.ConnectInfo.
type VirtualDeviceConnectInfo struct {
	// +optional

	// MigrateConnect specifies whether to override the virtual device
	// connection state upon completion of a migration.
	MigrateConnect string `json:"migrateConnect,omitempty"`

	// StartConnected specifies whether to connect the device when the virtual
	// machine starts.
	StartConnected bool `json:"startConnected"`

	// AllowGuestControl enables guest control over whether the connectable
	// device is connected.
	AllowGuestControl bool `json:"allowGuestControl"`

	// Connected indicates whether the device is currently connected.
	// Valid only while the virtual machine is running.
	Connected bool `json:"connected"`

	// +optional

	// Status indicates the current status of the connectable device.
	// Valid only while the virtual machine is running.
	Status string `json:"status,omitempty"`
}

// VirtualDeviceDeviceGroupInfo contains information about the device group a
// device is assigned to.
// It corresponds to vim.vm.device.VirtualDevice.DeviceGroupInfo.
type VirtualDeviceDeviceGroupInfo struct {
	// GroupInstanceKey is the device group instance key.
	GroupInstanceKey int32 `json:"groupInstanceKey"`

	// SequenceId is the device's sequence position within the group.
	SequenceId int32 `json:"sequenceId"`
}

// KeyProviderId identifies a cryptographic key provider.
// It corresponds to vim.encryption.KeyProviderId.
type KeyProviderId struct {
	// Id is the globally unique ID for the crypto key provider.
	Id string `json:"id"`
}

// CryptoKeyId identifies a cryptographic key by ID and provider.
// It corresponds to vim.encryption.CryptoKeyId.
type CryptoKeyId struct {
	// KeyId is the unique key ID.
	KeyId string `json:"keyId"`

	// +optional

	// ProviderId identifies the provider that holds the key.
	ProviderId *KeyProviderId `json:"providerId,omitempty"`
}

// OptionValue is a key-value configuration parameter.
// It corresponds to vim.option.OptionValue.
type OptionValue struct {
	// Key is the option key.
	Key string `json:"key"`

	// Value is the option value, encoded as a string.
	Value string `json:"value"`
}

// VirtualPCIPassthroughAllowedDevice specifies an allowed PCI device for
// Dynamic DirectPath.
// It corresponds to vim.vm.device.VirtualPCIPassthrough.AllowedDevice.
type VirtualPCIPassthroughAllowedDevice struct {
	// VendorId is the PCI vendor ID.
	VendorId int32 `json:"vendorId"`

	// DeviceId is the PCI device ID.
	DeviceId int32 `json:"deviceId"`

	// +optional

	// SubVendorId is the PCI subvendor ID.
	SubVendorId int32 `json:"subVendorId,omitempty"`

	// +optional

	// SubDeviceId is the PCI subdevice ID.
	SubDeviceId int32 `json:"subDeviceId,omitempty"`

	// +optional

	// RevisionId is the PCI revision ID.
	RevisionId int32 `json:"revisionId,omitempty"`
}

// +kubebuilder:validation:Enum=PCI;USBControllerPCI

// VirtualDeviceBusSlotInfoType enumerates the valid bus slot info types.
type VirtualDeviceBusSlotInfoType string

const (
	// VirtualDeviceBusSlotInfoTypePCI indicates a PCI bus slot.
	VirtualDeviceBusSlotInfoTypePCI VirtualDeviceBusSlotInfoType = "PCI"

	// VirtualDeviceBusSlotInfoTypeUSBControllerPCI indicates a USB controller
	// PCI bus slot that may include a separate eHCI slot.
	VirtualDeviceBusSlotInfoTypeUSBControllerPCI VirtualDeviceBusSlotInfoType = "USBControllerPCI"
)

// VirtualDeviceBusSlotInfo maps the polymorphic bus slot info hierarchy to a
// Kubernetes-compatible flat structure.
// It corresponds to vim.vm.device.VirtualDevice.BusSlotInfo.
type VirtualDeviceBusSlotInfo struct {
	// Type indicates the concrete bus slot info type.
	Type VirtualDeviceBusSlotInfoType `json:"type"`

	// +optional

	// PCI contains PCI bus slot info.
	PCI *VirtualDevicePciBusSlotInfo `json:"pci,omitempty"`

	// +optional

	// USBControllerPCI contains USB controller PCI bus slot info, which may
	// include a separate slot for the eHCI controller.
	USBControllerPCI *VirtualUSBControllerPciBusSlotInfo `json:"usbControllerPCI,omitempty"`
}

// VirtualDevicePciBusSlotInfo defines the PCI bus slot of a device in a
// virtual machine.
// It corresponds to vim.vm.device.VirtualDevicePciBusSlotInfo.
type VirtualDevicePciBusSlotInfo struct {
	// PciSlotNumber is the PCI slot number of the virtual device.
	// The server assigns this value; client-specified values may be
	// overridden if invalid or duplicated.
	PciSlotNumber int32 `json:"pciSlotNumber"`
}

// VirtualUSBControllerPciBusSlotInfo defines the PCI bus slot of a USB
// controller device, including an optional separate eHCI slot.
// It corresponds to vim.vm.device.VirtualUSBControllerPciBusSlotInfo.
type VirtualUSBControllerPciBusSlotInfo struct {
	VirtualDevicePciBusSlotInfo `json:",inline"`

	// +optional

	// EhciPciSlotNumber is the PCI slot number of the eHCI controller.
	// Only meaningful when EhciEnabled is set on the USB controller.
	EhciPciSlotNumber int32 `json:"ehciPciSlotNumber,omitempty"`
}

// VirtualMachineVideoCard represents a video card in a virtual machine.
// It corresponds to vim.vm.device.VirtualMachineVideoCard.
type VirtualMachineVideoCard struct {
	// +optional

	// VideoRamSize is the size of the framebuffer.
	VideoRamSize *resource.Quantity `json:"videoRamSize,omitempty"`

	// +optional

	// NumDisplays is the number of supported monitors.
	// This property can only be updated when the virtual machine is powered
	// off.
	NumDisplays int32 `json:"numDisplays,omitempty"`

	// +optional

	// UseAutoDetect indicates whether the host display settings are used to
	// automatically determine the virtual video card display settings. When
	// true, NumDisplays is ignored.
	UseAutoDetect *bool `json:"useAutoDetect,omitempty"`

	// +optional

	// Enable3DSupport indicates whether the virtual video card supports 3D
	// functions. This property can only be updated when the virtual machine
	// is powered off.
	Enable3DSupport *bool `json:"enable3DSupport,omitempty"`

	// +optional

	// Use3dRenderer indicates how the virtual video device renders 3D
	// graphics.
	// Valid values: "automatic", "software", "hardware".
	Use3dRenderer string `json:"use3dRenderer,omitempty"`

	// +optional

	// GraphicsMemorySize is the amount of guest memory used for graphics
	// resources. Only meaningful when 3D support is enabled.
	// This property can only be updated when the virtual machine is powered
	// off.
	GraphicsMemorySize *resource.Quantity `json:"graphicsMemorySize,omitempty"`
}

// VirtualMachineVMCIDeviceFilterSpec describes a VMCI communication filter
// rule based on protocol, direction, and port range.
// It corresponds to vim.vm.VirtualMachineVMCIDevice.FilterSpec.
type VirtualMachineVMCIDeviceFilterSpec struct {
	// Rank is the filter rank. Filters are processed in ascending rank order.
	// Ranks within a filter array must be unique.
	Rank int64 `json:"rank"`

	// Action is the filter action.
	Action string `json:"action"`

	// Protocol is the filter protocol.
	Protocol string `json:"protocol"`

	// Direction is the filter direction.
	Direction string `json:"direction"`

	// +optional

	// LowerDstPortBoundary is the lower bound of the destination port range.
	// Defaults to the lowest port number for the given protocol when unset.
	LowerDstPortBoundary int64 `json:"lowerDstPortBoundary,omitempty"`

	// +optional

	// UpperDstPortBoundary is the upper bound of the destination port range.
	// Defaults to the highest port number for the given protocol when unset.
	UpperDstPortBoundary int64 `json:"upperDstPortBoundary,omitempty"`
}

// VirtualMachineVMCIDeviceFilterInfo contains the list of VMCI communication
// filter rules for a virtual machine.
// It corresponds to vim.vm.VirtualMachineVMCIDevice.FilterInfo.
type VirtualMachineVMCIDeviceFilterInfo struct {
	// +optional

	// Filters is the list of VMCI filter specifications.
	// To clear all existing filters, set to an empty list or leave unset.
	Filters []VirtualMachineVMCIDeviceFilterSpec `json:"filters,omitempty"`
}

// VirtualMachineVMCIDevice represents a VMCI device in a virtual machine.
// It corresponds to vim.vm.device.VirtualMachineVMCIDevice.
type VirtualMachineVMCIDevice struct {
	// +optional

	// Id is the unique identifier for VMCI socket access to this virtual
	// machine. Applications on other virtual machines use this value to
	// connect to this virtual machine.
	Id int64 `json:"id,omitempty"`

	// +optional

	// AllowUnrestrictedCommunication determines whether VMCI communication
	// with all virtual machines on the host is allowed.
	//
	// Deprecated: As of vSphere API 5.1, the VMCI device does not support
	// communication with other virtual machines.
	AllowUnrestrictedCommunication *bool `json:"allowUnrestrictedCommunication,omitempty"`

	// +optional

	// FilterEnable determines whether filtering of VMCI communication is
	// enabled for this virtual machine.
	FilterEnable *bool `json:"filterEnable,omitempty"`

	// +optional

	// FilterInfo specifies the VMCI filter rules controlling the extent of
	// VMCI communication.
	FilterInfo *VirtualMachineVMCIDeviceFilterInfo `json:"filterInfo,omitempty"`
}

// VirtualNVDIMM represents a virtual NVDIMM device in a virtual machine.
// It corresponds to vim.vm.device.VirtualNVDIMM.
type VirtualNVDIMM struct {
	// Capacity is the NVDIMM backing size.
	// Reported as 0 if the backing is inaccessible.
	Capacity resource.Quantity `json:"capacity"`

	// +optional

	// ConfiguredCapacity is the NVDIMM device's configured size.
	ConfiguredCapacity *resource.Quantity `json:"configuredCapacity,omitempty"`
}

// VirtualParallelPort represents a virtual parallel port in a virtual machine.
// It corresponds to vim.vm.device.VirtualParallelPort.
type VirtualParallelPort struct{}

// VirtualSerialPort represents a virtual serial port in a virtual machine.
// It corresponds to vim.vm.device.VirtualSerialPort.
type VirtualSerialPort struct {
	// YieldOnPoll enables CPU yield behavior when the virtual machine's sole
	// task is polling the virtual serial port. Requires the CPU yield option
	// to be supported on the host.
	YieldOnPoll bool `json:"yieldOnPoll"`
}

// VirtualTPM represents a virtual TPM 2.0 module in a virtual machine.
// It corresponds to vim.vm.device.VirtualTPM.
type VirtualTPM struct {
	// +optional

	// EndorsementKeyCertificateSigningRequest contains one or more Endorsement
	// Key Certificate Signing Requests in DER format.
	EndorsementKeyCertificateSigningRequest [][]byte `json:"endorsementKeyCertificateSigningRequest,omitempty"`

	// +optional

	// EndorsementKeyCertificate contains one or more Endorsement Key
	// Certificates in DER format.
	EndorsementKeyCertificate [][]byte `json:"endorsementKeyCertificate,omitempty"`
}

// VirtualUSB represents a virtual USB device in a virtual machine.
// It corresponds to vim.vm.device.VirtualUSB.
type VirtualUSB struct {
	// Connected indicates whether the device is currently connected.
	// The device may not be connected if the auto-connect pattern in the
	// device backing cannot be satisfied.
	Connected bool `json:"connected"`

	// +optional

	// Vendor is the vendor ID of the USB device.
	Vendor int32 `json:"vendor,omitempty"`

	// +optional

	// Product is the product ID of the USB device.
	Product int32 `json:"product,omitempty"`

	// +optional

	// Family lists the device class families.
	Family []string `json:"family,omitempty"`

	// +optional

	// Speed lists the device speeds detected by the server.
	Speed []string `json:"speed,omitempty"`
}

// VirtualWDT represents a virtual watchdog timer device in a virtual machine.
// It corresponds to vim.vm.device.VirtualWDT.
type VirtualWDT struct {
	// RunOnBoot indicates whether the virtual watchdog timer device should
	// be initialized in the Enabled/Running sub-state.
	// When false, the device initializes in the Enabled/Stopped sub-state.
	RunOnBoot bool `json:"runOnBoot"`

	// Running indicates whether the virtual watchdog timer device is
	// currently in the Enabled/Running state.
	Running bool `json:"running"`
}

// VirtualCDROM represents a CD-ROM device in a virtual machine.
// It corresponds to vim.vm.device.VirtualCDROM, which carries no data beyond
// the base device.
type VirtualCDROM struct{}

// VirtualFloppy represents a floppy disk drive in a virtual machine.
// It corresponds to vim.vm.device.VirtualFloppy, which carries no data beyond
// the base device.
type VirtualFloppy struct{}

// VirtualKeyboard represents a keyboard device in a virtual machine.
// It corresponds to vim.vm.device.VirtualKeyboard, which carries no data
// beyond the base device.
type VirtualKeyboard struct{}

// VirtualPointingDevice represents a pointing device (mouse) in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualPointingDevice, which carries no data
// beyond the base device.
type VirtualPointingDevice struct{}

// VirtualPCIPassthrough represents a PCI passthrough device in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualPCIPassthrough, which carries no data
// beyond the base device.
type VirtualPCIPassthrough struct{}

// VirtualPrecisionClock represents a precision clock device in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualPrecisionClock, which carries no data
// beyond the base device.
type VirtualPrecisionClock struct{}

// VirtualSoundBlaster16 represents a Sound Blaster 16 sound card in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualSoundBlaster16, which carries no data
// beyond the base sound card.
type VirtualSoundBlaster16 struct{}

// VirtualEnsoniq1371 represents an Ensoniq 1371 sound card in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualEnsoniq1371, which carries no data
// beyond the base sound card.
type VirtualEnsoniq1371 struct{}

// VirtualHdAudioCard represents an Intel HD Audio sound card in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualHdAudioCard, which carries no data
// beyond the base sound card.
type VirtualHdAudioCard struct{}
