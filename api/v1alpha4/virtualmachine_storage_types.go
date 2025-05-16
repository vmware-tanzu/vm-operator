// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	"slices"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// +kubebuilder:validation:Enum=Classic;Managed

// VolumeType describes the type of a VirtualMachine volume.
type VolumeType string

const (
	// VolumeTypeClassic describes a classic virtual disk, such as the boot disk
	// for a VirtualMachine deployed from a VM Image of type OVF.
	VolumeTypeClassic VolumeType = "Classic"

	// VolumeTypeManaged describes a managed virtual disk, such as persistent
	// volumes.
	VolumeTypeManaged VolumeType = "Managed"
)

// +kubebuilder:validation:Enum=Thin;Thick;ThickEagerZero

// VolumeProvisioningMode is the type used to express the
// desired or observed provisioning mode for a virtual machine disk.
type VolumeProvisioningMode string

const (
	VolumeProvisioningModeThin           VolumeProvisioningMode = "Thin"
	VolumeProvisioningModeThick          VolumeProvisioningMode = "Thick"
	VolumeProvisioningModeThickEagerZero VolumeProvisioningMode = "ThickEagerZero"
)

// +kubebuilder:validation:Enum=IndependentNonPersistent;IndependentPersistent;NonPersistent;Persistent;Dependent

type VolumeDiskMode string

const (
	VolumeDiskModeIndependentNonPersistent VolumeDiskMode = "IndependentNonPersistent"
	VolumeDiskModeIndependentPersistent    VolumeDiskMode = "IndependentPersistent"
	VolumeDiskModeNonPersistent            VolumeDiskMode = "NonPersistent"
	VolumeDiskModePersistent               VolumeDiskMode = "Persistent"
)

// +kubebuilder:validation:Enum=MultiWriter;None

type VolumeSharingMode string

const (
	VolumeSharingModeMultiWriter VolumeSharingMode = "MultiWriter"
	VolumeSharingModeNone        VolumeSharingMode = "None"
)

// +kubebuilder:validation:Enum=OracleRAC;MicrosoftWSFC

type VolumeApplicationType string

const (
	VolumeApplicationTypeOracleRAC     VolumeApplicationType = "OracleRAC"
	VolumeApplicationTypeMicrosoftWSFC VolumeApplicationType = "MicrosoftWSFC"
)

// +kubebuilder:validation:Enum=IDE;NVME;SCSI;SATA

type StorageControllerType string

const (
	StorageControllerTypeIDE  = "IDE"
	StorageControllerTypeNVME = "NVME"
	StorageControllerTypeSCSI = "SCSI"
	StorageControllerTypeSATA = "SATA"
)

// +kubebuilder:validation:Enum=None;Physical;Virtual

type VolumeControllerSharingMode string

const (
	VolumeControllerSharingModeNone     VolumeControllerSharingMode = "None"
	VolumeControllerSharingModePhysical VolumeControllerSharingMode = "Physical"
	VolumeControllerSharingModeVirtual  VolumeControllerSharingMode = "Virtual"
)

// +kubebuilder:validation:Enum=ParaVirtual;BusLogic;LsiLogic;LsiLogicSAS

type SCSIControllerType string

const (
	SCSIControllerTypeParaVirtualSCSI = "ParaVirtual"
	SCSIControllerTypeBusLogic        = "BusLogic"
	SCSIControllerTypeLsiLogic        = "LsiLogic"
	SCSIControllerTypeLsiLogicSAS     = "LsiLogicSAS"
)

// VirtualMachineVolume represents a named volume in a VM.
type VirtualMachineVolume struct {
	// Name represents the volume's name. Must be a DNS_LABEL and unique within
	// the VM.
	Name string `json:"name"`

	// VirtualMachineVolumeSource represents the location and type of a volume
	// to mount.
	VirtualMachineVolumeSource `json:",inline"`
}

// VirtualMachineVolumeSource represents the source location of a volume to
// mount. Only one of its members may be specified.
type VirtualMachineVolumeSource struct {
	// +optional

	// PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
	// in the same namespace.
	//
	// More information is available at
	// https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims.
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// PersistentVolumeClaimVolumeSource is a composite for the Kubernetes
// corev1.PersistentVolumeClaimVolumeSource and instance storage options.
type PersistentVolumeClaimVolumeSource struct {
	corev1.PersistentVolumeClaimVolumeSource `json:",inline" yaml:",inline"`

	// +optional

	// InstanceVolumeClaim is set if the PVC is backed by instance storage.
	InstanceVolumeClaim *InstanceVolumeClaimVolumeSource `json:"instanceVolumeClaim,omitempty"`

	// +optional

	// ApplicationType describes the type of application for which this volume
	// is intended to be used.
	//
	//   - OracleRAC      -- The volume is configured with
	//                       diskMode=IndependentPersistent and
	//                       sharingMode=MultiWriter and attached to the first
	//                       SCSI controller with an available slot and
	//                       sharingMode=None. If no such controller exists,
	//                       a new ParaVirtual SCSI controller will be created
	//                       with sharingMode=None long as there are currently
	//                       three or fewer SCSI controllers.
	//   - MicrosoftWSFC  -- The volume is configured with
	//                       diskMode=IndependentPersistent and attached to a
	//                       SCSI controller with sharingMode=Physical.
	//                       If no such controller exists, a new ParaVirtual
	//                       SCSI controller will be created with
	//                       sharingMode=Physical as long as there are currently
	//                       three or fewer SCSI controllers.
	ApplicationType VolumeApplicationType `json:"applicationType,omitempty"`

	// +optional

	// ControllerBusNumber describes the bus number of the controller to which
	// this volume should be attached.
	//
	// The bus number specifies a controller based on the value of the
	// controllerType field:
	//
	//   - IDE  -- spec.hardware.ideControllers
	//   - NVME -- spec.hardware.nvmeControllers
	//   - SATA -- spec.hardware.sataControllers
	//   - SCSI -- spec.hardware.scsiControllers
	//
	// If this and controllerType are both omitted, the volume will be attached
	// to the first available SCSI controller. If there is no SCSI controller
	// with an available slot, a new ParaVirtual SCSI controller will be added
	// as long as there are currently three or fewer SCSI controllers.
	//
	// If the specified controller has no available slots, the request will be
	// denied.
	ControllerBusNumber *int32 `json:"controllerBusNumber,omitempty"`

	// +optional
	// +kubebuilder:default=SCSI

	// ControllerType describes the type of the controller to which this volume
	// should be attached.
	//
	// Please keep in mind the number of volumes supported by the different
	// types of controllers:
	//
	//   - IDE                -- 4 total (2 per controller)
	//   - NVME               -- 256 total (64 per controller)
	//   - SATA               -- 120 total (30 per controller)
	//   - SCSI (ParaVirtual) -- 252 total (63 per controller)
	//   - SCSI (BusLogic)    -- 60 total (15 per controller)
	//   - SCSI (LsiLogic)    -- 60 total (15 per controller)
	//   - SCSI (LsiLogicSAS) -- 60 total (15 per controller)
	//
	// Please note, the number of supported volumes per SCSI controller may seem
	// off, but remember that a SCSI controller occupies a slot on its own bus.
	// Thus even though a ParaVirtual SCSI controller supports 64 targets and
	// the other types of SCSI controllers support 16 targets, one of the
	// targets is occupied by the controller itself.
	//
	// Defaults to SCSI when controllerBusNumber is also omitted; otherwise the
	// default value is determined by the logic outlined in the description of
	// the controllerBusNumber field.
	ControllerType StorageControllerType `json:"controllerType,omitempty"`

	// +optional
	// +kubebuilder:default=Persistent

	// DiskMode describes the desired mode to use when attaching the volume.
	//
	// Please note, volumes attached as IndependentNonPersistent or
	// IndependentPersistent are not included in a VM's snapshots or backups.
	//
	// Also, any data written to volumes attached as IndependentNonPersistent or
	// NonPersistent will be discarded when the VM is powered off.
	//
	// Defaults to Persistent.
	DiskMode VolumeDiskMode `json:"diskMode,omitempty"`

	// +optional

	// SharingMode describes the volume's desired sharing mode.
	//
	// When applicationType=OracleRAC, the field defaults to MultiWriter.
	// Otherwise, defaults to None.
	SharingMode VolumeSharingMode `json:"sharingMode,omitempty"`

	// UnitNumber describes the desired unit number for attaching the volume to
	// a storage controller.
	//
	// When omitted, the next available unit number of the selected controller
	// is used.
	//
	// This value must be unique for the controller referenced by the
	// controllerBusNumber and controllerType properties. If the value is
	// already used by another device, this volume will not be attached.
	//
	// Please note the value 7 is invalid if controllerType=SCSI as 7 is the
	// unit number of the SCSI controller on its own bus.
	UnitNumber *int32 `json:"unitNumber,omitempty"`
}

// InstanceVolumeClaimVolumeSource contains information about the instance
// storage volume claimed as a PVC.
type InstanceVolumeClaimVolumeSource struct {
	// StorageClass is the name of the Kubernetes StorageClass that provides
	// the backing storage for this instance storage volume.
	StorageClass string `json:"storageClass"`

	// Size is the size of the requested instance storage volume.
	Size resource.Quantity `json:"size"`
}

type VirtualMachineVolumeCryptoStatus struct {
	// +optional

	// ProviderID describes the provider ID used to encrypt the volume.
	// Please note, this field will be empty if the volume is not
	// encrypted.
	ProviderID string `json:"providerID,omitempty"`

	// +optional

	// KeyID describes the key ID used to encrypt the volume.
	// Please note, this field will be empty if the volume is not
	// encrypted.
	KeyID string `json:"keyID,omitempty"`
}

// VirtualMachineVolumeStatus defines the observed state of a
// VirtualMachineVolume instance.
type VirtualMachineVolumeStatus struct {
	// Name is the name of the attached volume.
	Name string `json:"name"`

	// +optional

	// ControllerBusNumber describes volume's observed controller's bus number.
	ControllerBusNumber int32 `json:"controllerBusNumber,omitempty"`

	// +optional

	// ControllerType describes volume's observed controller's type.
	ControllerType StorageControllerType `json:"controllerType,omitempty"`

	// +kubebuilder:default=Managed

	// Type is the type of the attached volume.
	Type VolumeType `json:"type"`

	// +optional

	// DiskMode describes the volume's observed disk mode.
	DiskMode VolumeDiskMode `json:"diskMode,omitempty"`

	// +optional

	// SharingMode describes the volume's observed sharing mode.
	SharingMode VolumeSharingMode `json:"sharingMode,omitempty"`

	// +optional

	// Crypto describes the volume's encryption status.
	Crypto *VirtualMachineVolumeCryptoStatus `json:"crypto,omitempty"`

	// +optional

	// Limit describes the storage limit for the volume.
	Limit *resource.Quantity `json:"limit,omitempty"`

	// +optional

	// Used describes the observed, non-shared size of the volume on disk.
	// For example, if this is a linked-clone's boot volume, this value
	// represents the space consumed by the linked clone, not the parent.
	Used *resource.Quantity `json:"used,omitempty"`

	// +optional

	// Attached represents whether a volume has been successfully attached to
	// the VirtualMachine or not.
	Attached bool `json:"attached,omitempty"`

	// +optional

	// DiskUUID represents the underlying virtual disk UUID and is present when
	// attachment succeeds.
	DiskUUID string `json:"diskUUID,omitempty"`

	// +optional

	// Error represents the last error seen when attaching or detaching a
	// volume.  Error will be empty if attachment succeeds.
	Error string `json:"error,omitempty"`
}

// SortVirtualMachineVolumeStatuses sorts the provided list of
// VirtualMachineVolumeStatus objects.
func SortVirtualMachineVolumeStatuses(s []VirtualMachineVolumeStatus) {
	slices.SortFunc(s, func(a, b VirtualMachineVolumeStatus) int {
		switch {
		case a.DiskUUID < b.DiskUUID:
			return -1
		case a.DiskUUID > b.DiskUUID:
			return 1
		default:
			return 0
		}
	})
}

// VirtualMachineStorageStatus defines the observed state of a VirtualMachine's
// storage.
type VirtualMachineStorageStatus struct {
	// +optional

	// Usage describes the observed amount of storage used by a VirtualMachine.
	Usage *VirtualMachineStorageStatusUsage `json:"usage,omitempty"`
}

type VirtualMachineStorageStatusUsage struct {
	// +optional

	// Total describes the total storage space used by a VirtualMachine that
	// counts against the Namespace's storage quota.
	Total *resource.Quantity `json:"total,omitempty"`

	// +optional

	// Disks describes the total storage space used by a VirtualMachine's
	// non-PVC disks.
	Disks *resource.Quantity `json:"disks,omitempty"`

	// +optional

	// Other describes the total storage space used by the VirtualMachine's
	// non disk files, ex. the configuration file, swap space, logs, snapshots,
	// etc.
	Other *resource.Quantity `json:"other,omitempty"`
}

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
	SharingMode VolumeControllerSharingMode `json:"sharingMode,omitempty"`
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
	SharingMode VolumeControllerSharingMode `json:"sharingMode,omitempty"`

	// +optional
	// +kubebuilder:default=ParaVirtual

	// Type describes the desired type of SCSI controller.
	//
	// Defaults to ParaVirtual.
	Type SCSIControllerType `json:"type,omitempty"`
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
	ControllerType StorageControllerType `json:"controllerType,omitempty"`

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
