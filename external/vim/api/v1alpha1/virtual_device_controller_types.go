// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// VirtualControllerType identifies the concrete type of a virtual
// controller device in a virtual machine.
type VirtualControllerType string

const (
	// VirtualControllerTypeAHCI is an AHCI (SATA) controller.
	VirtualControllerTypeAHCI = VirtualControllerType(VirtualDeviceTypeAHCIController)

	// VirtualControllerTypeBusLogic is a BusLogic SCSI controller.
	VirtualControllerTypeBusLogic = VirtualControllerType(VirtualDeviceTypeBusLogicController)

	// VirtualControllerTypeIDE is an IDE controller.
	VirtualControllerTypeIDE = VirtualControllerType(VirtualDeviceTypeIDEController)

	// VirtualControllerTypeLsiLogic is an LSI Logic SCSI controller.
	VirtualControllerTypeLsiLogic = VirtualControllerType(VirtualDeviceTypeLsiLogicController)

	// VirtualControllerTypeLsiLogicSAS is an LSI Logic SAS controller.
	VirtualControllerTypeLsiLogicSAS = VirtualControllerType(VirtualDeviceTypeLsiLogicSASController)

	// VirtualControllerTypeNVMe is an NVMe controller.
	VirtualControllerTypeNVMe = VirtualControllerType(VirtualDeviceTypeNVMeController)

	// VirtualControllerTypeParaVirtualSCSI is a VMware Paravirtual SCSI
	// controller.
	VirtualControllerTypeParaVirtualSCSI = VirtualControllerType(VirtualDeviceTypeParaVirtualSCSIController)

	// VirtualControllerTypePCI is a PCI controller.
	VirtualControllerTypePCI = VirtualControllerType(VirtualDeviceTypePCIController)

	// VirtualControllerTypePS2 is a PS/2 controller.
	VirtualControllerTypePS2 = VirtualControllerType(VirtualDeviceTypePS2Controller)

	// VirtualControllerTypeSATA is a SATA controller.
	VirtualControllerTypeSATA = VirtualControllerType(VirtualDeviceTypeSATAController)

	// VirtualControllerTypeSIO is a Super IO (SIO) controller.
	VirtualControllerTypeSIO = VirtualControllerType(VirtualDeviceTypeSIOController)

	// VirtualControllerTypeUSB is a USB (UHCI/EHCI) controller.
	VirtualControllerTypeUSB = VirtualControllerType(VirtualDeviceTypeUSBController)

	// VirtualControllerTypeUSBXHCI is a USB 3.0 (XHCI) controller.
	VirtualControllerTypeUSBXHCI = VirtualControllerType(VirtualDeviceTypeUSBXHCIController)
)

type VirtualUSBControllerType string

const (
	// VirtualUSBControllerTypeUHCI is a USB 1.1 (UHCI) controller.
	VirtualUSBControllerTypeUHCI = VirtualUSBControllerType(VirtualDeviceTypeUSBController)

	// VirtualUSBControllerTypeEHCI is a USB 2.0 (EHCI) controller.
	VirtualUSBControllerTypeEHCI = VirtualUSBControllerType(VirtualDeviceTypeUSBController)

	// VirtualUSBControllerTypeXHCI is a USB 3.0 (XHCI) controller.
	VirtualUSBControllerTypeXHCI = VirtualUSBControllerType(VirtualDeviceTypeUSBXHCIController)
)

// VirtualController is the base data object for a device controller in a
// virtual machine.
// It corresponds to vim.vm.device.VirtualController.
type VirtualController struct {
	// BusNumber is the bus number associated with this controller.
	BusNumber int32 `json:"busNumber"`

	// +optional

	// Device is the list of keys of virtual devices controlled by this
	// controller.
	Device []int32 `json:"device,omitempty"`
}

// +kubebuilder:validation:Enum=None;Physical;Virtual

// VirtualNVMESharing describes the NVME bus sharing mode.
// It corresponds to vim.vm.device.VirtualSCSIController.Sharing.
type VirtualNVMESharing string

const (
	// VirtualNVMESharingNone disables NVME bus sharing.
	VirtualNVMESharingNone VirtualNVMESharing = "None"

	// VirtualNVMESharingPhysical enables physical NVME bus sharing.
	VirtualNVMESharingPhysical VirtualNVMESharing = "Physical"
)

// VirtualNVMEController represents an NVMe controller in a virtual machine.
// It corresponds to vim.vm.device.VirtualNVMEController.
type VirtualNVMEController struct {
	VirtualController `json:",inline"`

	// +optional

	// SharedBus is the shared bus mode of the NVMe controller.
	SharedBus VirtualNVMESharing `json:"sharedBus,omitempty"`
}

// +kubebuilder:validation:Enum=None;Physical;Virtual

// VirtualSCSISharing describes the SCSI bus sharing mode.
// It corresponds to vim.vm.device.VirtualSCSIController.Sharing.
type VirtualSCSISharing string

const (
	// VirtualSCSISharingNone disables SCSI bus sharing.
	VirtualSCSISharingNone VirtualSCSISharing = "None"

	// VirtualSCSISharingPhysical enables physical SCSI bus sharing.
	VirtualSCSISharingPhysical VirtualSCSISharing = "Physical"

	// VirtualSCSISharingVirtualSharing enables virtual SCSI bus sharing.
	VirtualSCSISharingVirtualSharing VirtualSCSISharing = "Virtual"
)

// VirtualSCSIController represents a SCSI controller in a virtual machine.
// It corresponds to vim.vm.device.VirtualSCSIController.
type VirtualSCSIController struct {
	VirtualController `json:",inline"`

	// +optional

	// HotAddRemove indicates whether hot-add and hot-remove of devices is
	// supported. Always true in the current implementation.
	HotAddRemove *bool `json:"hotAddRemove,omitempty"`

	// +optional
	// +kubebuilder:default=None

	// SharedBus is the SCSI bus sharing mode.
	SharedBus VirtualSCSISharing `json:"sharedBus,omitempty"`

	// +optional

	// ScsiCtlrUnitNumber is the unit number of the SCSI controller on its
	// own bus.
	ScsiCtlrUnitNumber int32 `json:"scsiCtlrUnitNumber,omitempty"`
}

// VirtualUSBController represents a USB Host Controller Interface (HCI) in a
// virtual machine.
// It corresponds to vim.vm.device.VirtualUSBController.
type VirtualUSBController struct {
	VirtualController `json:",inline"`
	// +optional

	// AutoConnectDevices indicates whether hot-plugging of devices is enabled
	// on this controller.
	AutoConnectDevices *bool `json:"autoConnectDevices,omitempty"`

	// +optional

	// EhciEnabled indicates whether enhanced host controller interface
	// (USB 2.0) is enabled on this controller.
	EhciEnabled *bool `json:"ehciEnabled,omitempty"`
}

// VirtualUSBXHCIController represents a USB 3.0 eXtensible Host Controller
// Interface (XHCI) in a virtual machine.
// It corresponds to vim.vm.device.VirtualUSBXHCIController.
type VirtualUSBXHCIController struct {
	VirtualController `json:",inline"`
	// +optional

	// AutoConnectDevices indicates whether hot-plugging of devices is enabled
	// on this controller.
	AutoConnectDevices *bool `json:"autoConnectDevices,omitempty"`
}

// VirtualIDEController represents an IDE controller in a virtual machine.
// It corresponds to vim.vm.device.VirtualIDEController, which carries no data
// beyond the base controller.
type VirtualIDEController struct {
	VirtualController `json:",inline"`
}

// VirtualPCIController represents a PCI controller in a virtual machine.
// It corresponds to vim.vm.device.VirtualPCIController, which carries no data
// beyond the base controller.
type VirtualPCIController struct {
	VirtualController `json:",inline"`
}

// VirtualPS2Controller represents a PS/2 controller in a virtual machine.
// It corresponds to vim.vm.device.VirtualPS2Controller, which carries no data
// beyond the base controller.
type VirtualPS2Controller struct {
	VirtualController `json:",inline"`
}

// VirtualSIOController represents a Super IO (SIO) controller in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualSIOController, which carries no data
// beyond the base controller.
type VirtualSIOController struct {
	VirtualController `json:",inline"`
}

// VirtualSATAController represents a SATA controller in a virtual machine.
// It corresponds to vim.vm.device.VirtualSATAController, which carries no data
// beyond the base controller.
type VirtualSATAController struct {
	VirtualController `json:",inline"`
}

// VirtualAHCIController represents an AHCI (SATA) controller in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualAHCIController, which carries no data
// beyond the base SATA controller.
type VirtualAHCIController struct {
	VirtualSATAController `json:",inline"`
}

// VirtualBusLogicController represents a BusLogic SCSI controller in a virtual
// machine.
// It corresponds to vim.vm.device.VirtualBusLogicController, which carries no
// data beyond the base SCSI controller.
type VirtualBusLogicController struct {
	VirtualSCSIController `json:",inline"`
}

// VirtualLsiLogicController represents an LSI Logic SCSI controller in a
// virtual machine.
// It corresponds to vim.vm.device.VirtualLsiLogicController, which carries no
// data beyond the base SCSI controller.
type VirtualLsiLogicController struct {
	VirtualSCSIController `json:",inline"`
}

// VirtualLsiLogicSASController represents an LSI Logic SAS controller in a
// virtual machine.
// It corresponds to vim.vm.device.VirtualLsiLogicSASController, which carries
// no data beyond the base SCSI controller.
type VirtualLsiLogicSASController struct {
	VirtualSCSIController `json:",inline"`
}

// ParaVirtualSCSIController represents a VMware Paravirtual SCSI controller in
// a virtual machine.
// It corresponds to vim.vm.device.ParaVirtualSCSIController, which carries no
// data beyond the base SCSI controller.
type ParaVirtualSCSIController struct {
	VirtualSCSIController `json:",inline"`
}
