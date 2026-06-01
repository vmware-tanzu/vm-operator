// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// VirtualControllerOption describes the options for a virtual controller.
// It corresponds to vim.vm.device.VirtualControllerOption.
type VirtualControllerOption struct {

	// Devices describes the minimum and maximum number of devices this
	// controller can control at run time.
	Devices IntOption `json:"devices"`

	// +optional

	// SupportedDevice lists the device types supported by this controller.
	SupportedDevice []VirtualDeviceOptionType `json:"supportedDevice,omitempty"`
}

// VirtualIDEControllerOption describes the options for an IDE controller.
// It corresponds to vim.vm.device.VirtualIDEControllerOption, which carries no
// data beyond the base controller option.
type VirtualIDEControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualPCIControllerOption describes the options for a PCI controller.
// It corresponds to vim.vm.device.VirtualPCIControllerOption, which carries no
// data beyond the base controller option.
type VirtualPCIControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualPS2ControllerOption describes the options for a PS/2 controller.
// It corresponds to vim.vm.device.VirtualPS2ControllerOption, which carries no
// data beyond the base controller option.
type VirtualPS2ControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualSIOControllerOption describes the options for a Super IO (SIO)
// controller.
// It corresponds to vim.vm.device.VirtualSIOControllerOption, which carries no
// data beyond the base controller option.
type VirtualSIOControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualSATAControllerOption describes the options for a SATA controller.
// It corresponds to vim.vm.device.VirtualSATAControllerOption, which carries
// no data beyond the base controller option.
type VirtualSATAControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualAHCIControllerOption describes the options for an AHCI (SATA)
// controller.
// It corresponds to vim.vm.device.VirtualAHCIControllerOption, which carries
// no data beyond the base SATA controller option.
type VirtualAHCIControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualNVMEControllerOption describes the options for an NVMe controller.
// It corresponds to vim.vm.device.VirtualNVMEControllerOption, which carries
// no data beyond the base controller option.
type VirtualNVMEControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualSCSIControllerOption describes the options for a virtual SCSI
// controller.
// It corresponds to vim.vm.device.VirtualSCSIControllerOption.
type VirtualSCSIControllerOption struct {
	VirtualControllerOption `json:",inline"`

	// NumSCSIDisks describes the minimum, maximum, and default number of SCSI
	// VirtualDisk instances available in the SCSI controller.
	NumSCSIDisks IntOption `json:"numSCSIDisks"`

	// NumSCSICDROMs describes the minimum, maximum, and default number of SCSI
	// VirtualCdrom instances available in the SCSI controller.
	NumSCSICDROMs IntOption `json:"numSCSICdroms"`

	// NumSCSIPassthrough describes the minimum, maximum, and default number of
	// VirtualSCSIPassthrough instances available in the SCSI controller.
	NumSCSIPassthrough IntOption `json:"numSCSIPassthrough"`

	// +optional

	// Sharing lists the supported shared bus modes.
	Sharing []VirtualSCSISharing `json:"sharing,omitempty"`

	// DefaultSharedIndex is the index into the Sharing array specifying the
	// default value.
	DefaultSharedIndex int32 `json:"defaultSharedIndex"`

	// HotAddRemove indicates whether hot add and remove of devices is
	// supported. All SCSI controllers support this; it cannot be toggled and
	// is always true when reading an existing configuration.
	HotAddRemove BoolOption `json:"hotAddRemove"`

	// SCSICtlrUnitNumber is the unit number of the SCSI controller. The SCSI
	// controller sits on its own bus, so this field defines which slot the
	// controller uses.
	SCSICtlrUnitNumber int32 `json:"scsiCtlrUnitNumber"`
}

// VirtualBusLogicControllerOption describes the options for a BusLogic SCSI
// controller.
// It corresponds to vim.vm.device.VirtualBusLogicControllerOption, which
// carries no data beyond the base SCSI controller option.
type VirtualBusLogicControllerOption struct {
	VirtualSCSIControllerOption `json:",inline"`
}

// VirtualLsiLogicControllerOption describes the options for an LSI Logic SCSI
// controller.
// It corresponds to vim.vm.device.VirtualLsiLogicControllerOption, which
// carries no data beyond the base SCSI controller option.
type VirtualLsiLogicControllerOption struct {
	VirtualSCSIControllerOption `json:",inline"`
}

// VirtualLsiLogicSASControllerOption describes the options for an LSI Logic
// SAS controller.
// It corresponds to vim.vm.device.VirtualLsiLogicSASControllerOption, which
// carries no data beyond the base SCSI controller option.
type VirtualLsiLogicSASControllerOption struct {
	VirtualSCSIControllerOption `json:",inline"`
}

// ParaVirtualSCSIControllerOption describes the options for a VMware
// Paravirtual SCSI controller.
// It corresponds to vim.vm.device.ParaVirtualSCSIControllerOption, which
// carries no data beyond the base SCSI controller option.
type ParaVirtualSCSIControllerOption struct {
	VirtualSCSIControllerOption `json:",inline"`
}

// VirtualUSBControllerOption describes the options for a USB (UHCI/EHCI)
// controller.
// It corresponds to vim.vm.device.VirtualUSBControllerOption, which carries no
// data beyond the base controller option.
type VirtualUSBControllerOption struct {
	VirtualControllerOption `json:",inline"`
}

// VirtualUSBXHCIControllerOption describes the options for a USB 3.0 (XHCI)
// controller.
// It corresponds to vim.vm.device.VirtualUSBXHCIControllerOption, which
// carries no data beyond the base controller option.
type VirtualUSBXHCIControllerOption struct {
	VirtualControllerOption `json:",inline"`
}
