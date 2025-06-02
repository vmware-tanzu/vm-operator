// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

// +kubebuilder:validation:Enum=IDE;NVME;SCSI;SATA

type VirtualControllerType string

const (
	VirtualControllerTypeIDE  = "IDE"
	VirtualControllerTypeNVME = "NVME"
	VirtualControllerTypeSCSI = "SCSI"
	VirtualControllerTypeSATA = "SATA"
)

// +kubebuilder:validation:Enum=None;Physical;Virtual

type VirtualControllerSharingMode string

const (
	VirtualControllerSharingModeNone     VirtualControllerSharingMode = "None"
	VirtualControllerSharingModePhysical VirtualControllerSharingMode = "Physical"
	VirtualControllerSharingModeVirtual  VirtualControllerSharingMode = "Virtual"
)

// +kubebuilder:validation:Enum=ParaVirtual;BusLogic;LsiLogic;LsiLogicSAS

type SCSIControllerType string

const (
	SCSIControllerTypeParaVirtualSCSI = "ParaVirtual"
	SCSIControllerTypeBusLogic        = "BusLogic"
	SCSIControllerTypeLsiLogic        = "LsiLogic"
	SCSIControllerTypeLsiLogicSAS     = "LsiLogicSAS"
)

// +kubebuilder:validation:Enum=CDROM;Disk

type VirtualDeviceType string

const (
	VirtualDeviceTypeCDROM VirtualDeviceType = "CDROM"
	VirtualDeviceTypeDisk  VirtualDeviceType = "Disk"
)

type IDEControllerSpec struct {
	// +required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1

	// BusNumber describes the desired bus number of the controller.
	BusNumber int32 `json:"busNumber"`
}

type NVMEControllerSpec struct {
	// +required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3

	// BusNumber describes the desired bus number of the controller.
	BusNumber int32 `json:"busNumber"`

	// +optional

	// PCISlotNumber describes the desired PCI slot number to use for this
	// controller.
	//
	// Please note, most of the time this field should be empty so the system
	// can pick an available slot.
	PCISlotNumber *int32 `json:"pciSlotNumber,omitempty"`

	// +optional
	// +kubebuilder:default=None

	// SharingMode describes the sharing mode for the controller.
	//
	// Defaults to None.
	SharingMode VirtualControllerSharingMode `json:"sharingMode,omitempty"`
}

type SATAControllerSpec struct {
	// +required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3

	// BusNumber describes the desired bus number of the controller.
	BusNumber int32 `json:"busNumber"`

	// +optional

	// PCISlotNumber describes the desired PCI slot number to use for this
	// controller.
	//
	// Please note, most of the time this field should be empty so the system
	// can pick an available slot.
	PCISlotNumber *int32 `json:"pciSlotNumber,omitempty"`
}

type SCSIControllerSpec struct {
	// +required
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=3

	// BusNumber describes the desired bus number of the controller.
	BusNumber int32 `json:"busNumber"`

	// +optional

	// PCISlotNumber describes the desired PCI slot number to use for this
	// controller.
	//
	// Please note, most of the time this field should be empty so the system
	// can pick an available slot.
	PCISlotNumber *int32 `json:"pciSlotNumber,omitempty"`

	// +optional
	// +kubebuilder:default=None

	// SharingMode describes the sharing mode for the controller.
	//
	// Defaults to None.
	SharingMode VirtualControllerSharingMode `json:"sharingMode,omitempty"`

	// +optional
	// +kubebuilder:default=ParaVirtual

	// Type describes the desired type of SCSI controller.
	//
	// Defaults to ParaVirtual.
	Type SCSIControllerType `json:"type,omitempty"`
}

type VirtualDeviceStatus struct {
	// +required

	// Name describes the name of the virtual device.
	Name string `json:"name"`

	// +required

	// Type describes the type of the virtual device.
	Type VirtualDeviceType `json:"type"`
}

type VirtualControllerStatus struct {
	// +required

	// BusNumber describes the observed bus number of the controller.
	BusNumber int32 `json:"busNumber"`

	// +required

	// Type describes the observed type of the controller.
	Type VirtualControllerType `json:"type"`

	// +optional
	// +listType=map
	// +listMapKey=name
	// +listMapKey=type

	// Devices describes the observed devices connected to the controller.
	Devices []VirtualDeviceStatus `json:"devices,omitempty"`
}

// VirtualMachineCdromSpec describes the desired state of a CD-ROM device.
type VirtualMachineCdromSpec struct {
	// +required
	// +kubebuilder:validation:Pattern="^[a-z0-9]{2,}$"

	// Name consists of at least two lowercase letters or digits of this CD-ROM.
	// It must be unique among all CD-ROM devices attached to the VM.
	//
	// This field is immutable when the VM is powered on.
	Name string `json:"name"`

	// Image describes the reference to an ISO type VirtualMachineImage or
	// ClusterVirtualMachineImage resource used as the backing for the CD-ROM.
	// If the image kind is omitted, it defaults to VirtualMachineImage.
	//
	// This field is immutable when the VM is powered on.
	//
	// Please note, unlike the spec.imageName field, the value of this
	// spec.cdrom.image.name MUST be a Kubernetes object name.
	Image VirtualMachineImageRef `json:"image"`

	// +optional

	// ControllerBusNumber describes the bus number of the controller to which
	// this CD-ROM should be attached.
	//
	// The bus number specifies a controller based on the value of the
	// controllerType field:
	//
	//   - IDE  -- spec.hardware.ideControllers
	//   - SATA -- spec.hardware.sataControllers
	//
	// If this and controllerType are both omitted, the CD-ROM will be attached
	// to the  first available IDE controller. If there is no IDE controller
	// with an available slot, a new SATA controller will be added as long as
	// there are currently three or fewer SATA controllers.
	//
	// If the specified controller has no available slots, the request will be
	// denied.
	ControllerBusNumber *int32 `json:"controllerBusNumber,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=IDE;SATA

	// ControllerType describes the type of the controller to which this CD-ROM
	// should be attached.
	//
	// Please keep in mind the number of devices supported by the different
	// types of controllers:
	//
	//   - IDE                -- 4 total (2 per controller)
	//   - SATA               -- 120 total (30 per controller)
	//
	// Defaults to IDE when controllerBusNumber is also omitted; otherwise the
	// default value is determined by the logic outlined in the description of
	// the controllerBusNumber field.
	ControllerType VirtualControllerType `json:"controllerType,omitempty"`

	// UnitNumber describes the desired unit number for attaching the CD-ROM to
	// a storage controller.
	//
	// When omitted, the next available unit number of the selected controller
	// is used.
	//
	// This value must be unique for the controller referenced by the
	// controllerBusNumber and controllerType properties. If the value is
	// already used by another device, this CD-ROM will not be attached.
	UnitNumber *int32 `json:"unitNumber,omitempty"`

	// +optional
	// +kubebuilder:default=true

	// Connected describes the desired connection state of the CD-ROM device.
	//
	// When true, the CD-ROM device is added and connected to the VM.
	// If the device already exists, it is updated to a connected state.
	//
	// When explicitly set to false, the CD-ROM device is added but remains
	// disconnected from the VM. If the CD-ROM device already exists, it is
	// updated to a disconnected state.
	//
	// Note: Before disconnecting a CD-ROM, the device may need to be unmounted
	// in the guest OS. Refer to the following KB article for more details:
	// https://knowledge.broadcom.com/external/article?legacyId=2144053
	//
	// Defaults to true if omitted.
	Connected *bool `json:"connected,omitempty"`

	// +optional
	// +kubebuilder:default=true

	// AllowGuestControl describes whether or not a web console connection
	// may be used to connect/disconnect the CD-ROM device.
	//
	// Defaults to true if omitted.
	AllowGuestControl *bool `json:"allowGuestControl,omitempty"`
}

type VirtualMachineHardwareSpec struct {
	// +optional
	// +listType=map
	// +listMapKey=name

	// Cdrom describes the desired state of the VM's CD-ROM devices.
	//
	// Each CD-ROM device requires a reference to an ISO-type
	// VirtualMachineImage or ClusterVirtualMachineImage resource as backing.
	//
	// Multiple CD-ROM devices using the same backing image, regardless of image
	// kinds (namespace or cluster scope), are not allowed.
	//
	// CD-ROM devices can be added, updated, or removed when the VM is powered
	// off. When the VM is powered on, only the connection state of existing
	// CD-ROM devices can be changed.
	// CD-ROM devices are attached to the VM in the specified list-order.
	Cdrom []VirtualMachineCdromSpec `json:"cdrom,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=busNumber
	// +kubebuilder:validation:MaxItems=2

	// IDEControllers describes the desired list of IDE controllers for the VM.
	//
	// Defaults to two IDE controllers, with bus 0 and bus 1.
	IDEControllers []IDEControllerSpec `json:"ideControllers,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=busNumber
	// +kubebuilder:validation:MaxItems=4

	// NVMEControllers describes the desired list of NVME controllers for the
	// VM.
	NVMEControllers []NVMEControllerSpec `json:"nvmeControllers,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=busNumber
	// +kubebuilder:validation:MaxItems=4

	// SATAControllers describes the desired list of SATA controllers for the
	// VM.
	//
	// Please note, all SATA controllers are VirtualAHCI.
	SATAControllers []SATAControllerSpec `json:"sataControllers,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=busNumber
	// +kubebuilder:validation:MaxItems=4

	// SCSIControllers describes the desired list of SCSI controllers for the
	// VM.
	SCSIControllers []SCSIControllerSpec `json:"scsiControllers,omitempty"`
}

type VirtualMachineHardwareStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=busNumber
	// +listMapKey=type

	// Controllers describes the observed list of virtual controllers for the
	// VM.
	Controllers []VirtualControllerStatus `json:"controllers,omitempty"`
}
