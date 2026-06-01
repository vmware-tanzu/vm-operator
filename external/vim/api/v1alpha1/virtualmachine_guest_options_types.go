// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type VirtualMachineGuestOptionsHardwareVersionStatus struct {
	// +required

	// HardwareVersion is the hardware version to which these guest options
	// apply, ex.: vmx-19.
	HardwareVersion string `json:"hardwareVersion"`

	// +optional

	// SupportedMaxCPUs is the maximum number of processors (virtual CPUs)
	// supported for this guest.
	SupportedMaxCPUs int32 `json:"supportedMaxCPUs,omitempty"`

	// +optional

	// NumSupportedPhysicalSockets is the maximum number of sockets supported
	// for this guest.
	NumSupportedPhysicalSockets int32 `json:"numSupportedPhysicalSockets,omitempty"`

	// +optional

	// NumSupportedCoresPerSocket is the maximum number of cores per socket
	// supported for this guest.
	NumSupportedCoresPerSocket int32 `json:"numSupportedCoresPerSocket,omitempty"`

	// +optional

	// SupportedMaxMem is the maximum memory supported for this guest.
	SupportedMaxMem *resource.Quantity `json:"supportedMaxMem,omitempty"`

	// +optional

	// SupportedMinMem is the minimum memory required for this guest.
	SupportedMinMem *resource.Quantity `json:"supportedMinMem,omitempty"`

	// +optional

	// RecommendedMem is the recommended default memory size for this guest.
	RecommendedMem *resource.Quantity `json:"recommendedMem,omitempty"`

	// +optional

	// RecommendedColorDepth is the recommended default color depth for this
	// guest.
	RecommendedColorDepth int32 `json:"recommendedColorDepth,omitempty"`

	// +optional

	// RecommendedDiskSize is the recommended default disk size for this guest.
	RecommendedDiskSize *resource.Quantity `json:"recommendedDiskSize,omitempty"`

	// +optional

	// SupportedNumDisks is the number of disks supported for this guest.
	SupportedNumDisks int32 `json:"supportedNumDisks,omitempty"`

	// +optional

	// RecommendedDiskController is the recommended default disk controller
	// type for this guest.
	RecommendedDiskController VirtualControllerType `json:"recommendedDiskController,omitempty"`

	// +optional

	// RecommendedSCSIController is the recommended default SCSI controller
	// type for this guest.
	RecommendedSCSIController VirtualControllerType `json:"recommendedSCSIController,omitempty"`

	// +optional

	// RecommendedCdromController is the recommended default CD-ROM controller
	// type for this guest.
	RecommendedCdromController VirtualControllerType `json:"recommendedCdromController,omitempty"`

	// +optional

	// SupportedDiskControllers lists supported disk controller types for this
	// guest.
	SupportedDiskControllers []VirtualControllerType `json:"supportedDiskControllers,omitempty"`

	// +optional

	// RecommendedEthernetCard is the recommended default ethernet adapter type
	// for this guest.
	RecommendedEthernetCard EthernetCardType `json:"recommendedEthernetCard,omitempty"`

	// +optional

	// SupportedEthernetCards lists supported ethernet adapter types for this
	// guest.
	SupportedEthernetCards []EthernetCardType `json:"supportedEthernetCards,omitempty"`

	// +optional

	// SupportedUSBControllerList lists supported USB controller types for this
	// guest.
	SupportedUSBControllerList []VirtualUSBControllerType `json:"supportedUSBControllerList,omitempty"`

	// +optional

	// RecommendedUSBController is the recommended default USB controller type
	// for this guest.
	RecommendedUSBController VirtualUSBControllerType `json:"recommendedUSBController,omitempty"`

	// +optional

	// WakeOnLanEthernetCard lists the NIC types supported by this guest that
	// also support Wake-on-LAN.
	WakeOnLanEthernetCard []EthernetCardType `json:"wakeOnLanEthernetCard,omitempty"`

	// +optional

	// RecommendedFirmware is the recommended firmware type for this guest.
	RecommendedFirmware Firmware `json:"recommendedFirmware,omitempty"`

	// +optional

	// SupportedFirmware lists supported firmware types for this guest.
	SupportedFirmware []Firmware `json:"supportedFirmware,omitempty"`

	// +optional

	// VRAMSize describes the video RAM size limits supported by this
	// guest.
	VRAMSize *ResourceQuantityOption `json:"vRAMSize,omitempty"`

	// +optional

	// NumSupportedFloppyDevices is the maximum number of floppy devices
	// supported by this guest.
	NumSupportedFloppyDevices int32 `json:"numSupportedFloppyDevices,omitempty"`

	// +optional

	// NumRecommendedPhysicalSockets is the recommended number of sockets for
	// this guest.
	NumRecommendedPhysicalSockets int32 `json:"numRecommendedPhysicalSockets,omitempty"`

	// +optional

	// NumRecommendedCoresPerSocket is the recommended number of cores per
	// socket for this guest.
	NumRecommendedCoresPerSocket int32 `json:"numRecommendedCoresPerSocket,omitempty"`

	// +optional

	// SupportsSlaveDisk indicates whether this guest supports a disk
	// configured as a slave.
	SupportsSlaveDisk *bool `json:"supportsSlaveDisk,omitempty"`

	// +optional

	// SmcRequired indicates that this guest requires an SMC (Apple hardware).
	SmcRequired bool `json:"smcRequired,omitempty"`

	// +optional

	// SmcRecommended indicates whether SMC (Apple hardware) is recommended
	// for this guest.
	SmcRecommended bool `json:"smcRecommended,omitempty"`

	// +optional

	// Ich7mRecommended indicates whether an I/O Controller Hub is recommended
	// for this guest.
	Ich7mRecommended bool `json:"ich7mRecommended,omitempty"`

	// +optional

	// UsbRecommended indicates whether a USB controller is recommended for
	// this guest.
	UsbRecommended bool `json:"usbRecommended,omitempty"`

	// +optional

	// SupportsWakeOnLan indicates whether this guest supports Wake-on-LAN.
	SupportsWakeOnLan bool `json:"supportsWakeOnLan,omitempty"`

	// +optional

	// SupportsVMI indicates whether this guest supports the virtual machine
	// interface.
	SupportsVMI bool `json:"supportsVMI,omitempty"`

	// +optional

	// Supports3D indicates whether this guest supports 3D graphics.
	Supports3D bool `json:"supports3D,omitempty"`

	// +optional

	// Recommended3D indicates whether 3D graphics are recommended for this
	// guest.
	Recommended3D bool `json:"recommended3D,omitempty"`

	// +optional

	// SupportsSecureBoot indicates whether Secure Boot is supported for this
	// guest. Only meaningful when virtual EFI firmware is in use.
	SupportsSecureBoot *bool `json:"supportsSecureBoot,omitempty"`

	// +optional

	// DefaultSecureBoot indicates whether Secure Boot should be enabled by
	// default for this guest. Only meaningful when virtual EFI firmware is in
	// use.
	DefaultSecureBoot *bool `json:"defaultSecureBoot,omitempty"`

	// +optional

	// SupportsCpuHotAdd indicates whether CPUs can be added to this guest
	// while the virtual machine is running.
	SupportsCpuHotAdd bool `json:"supportsCpuHotAdd,omitempty"`

	// +optional

	// SupportsCpuHotRemove indicates whether CPUs can be removed from this
	// guest while the virtual machine is running.
	SupportsCpuHotRemove bool `json:"supportsCpuHotRemove,omitempty"`

	// +optional

	// SupportsMemoryHotAdd indicates whether memory can be added to this
	// guest while the virtual machine is running.
	SupportsMemoryHotAdd bool `json:"supportsMemoryHotAdd,omitempty"`

	// +optional

	// SupportsPvscsiControllerForBoot indicates whether this guest can use
	// a PVSCSI controller as the boot adapter.
	SupportsPvscsiControllerForBoot bool `json:"supportsPvscsiControllerForBoot,omitempty"`

	// +optional

	// DiskUuidEnabled indicates whether disk UUID should be enabled by
	// default for this guest.
	DiskUuidEnabled bool `json:"diskUuidEnabled,omitempty"`

	// +optional

	// SupportsHotPlugPCI indicates whether this guest supports hot-plug of
	// PCI devices.
	SupportsHotPlugPCI bool `json:"supportsHotPlugPCI,omitempty"`

	// +optional

	// SupportsTPM20 indicates whether TPM 2.0 is supported for this guest.
	SupportsTPM20 *bool `json:"supportsTPM20,omitempty"`

	// +optional

	// RecommendedTPM20 indicates whether TPM 2.0 is recommended for this
	// guest.
	RecommendedTPM20 *bool `json:"recommendedTPM20,omitempty"`

	// +optional

	// VvtdSupported indicates support for Intel Virtualization Technology for
	// Directed I/O (VT-d) for this guest.
	VvtdSupported *BoolOption `json:"vvtdSupported,omitempty"`

	// +optional

	// VbsSupported indicates support for Virtualization-based security for
	// this guest.
	VbsSupported *BoolOption `json:"vbsSupported,omitempty"`

	// +optional

	// VsgxSupported indicates support for Intel Software Guard Extensions
	// (SGX) for this guest.
	VsgxSupported *BoolOption `json:"vsgxSupported,omitempty"`

	// +optional

	// VsgxRemoteAttestationSupported indicates support for Intel SGX remote
	// attestation for this guest.
	VsgxRemoteAttestationSupported *bool `json:"vsgxRemoteAttestationSupported,omitempty"`

	// +optional

	// VwdtSupported indicates support for a virtual watchdog timer for this
	// guest.
	VwdtSupported *bool `json:"vwdtSupported,omitempty"`

	// +optional

	// PersistentMemorySupported indicates support for persistent memory
	// (virtual NVDIMM) for this guest.
	PersistentMemorySupported *bool `json:"persistentMemorySupported,omitempty"`

	// +optional

	// SupportedMinPersistentMemory is the minimum persistent memory supported
	// for this guest.
	SupportedMinPersistentMemory *resource.Quantity `json:"supportedMinPersistentMemory,omitempty"`

	// +optional

	// SupportedMaxPersistentMemory is the maximum total persistent memory
	// supported for this guest across all virtual NVDIMM devices.
	SupportedMaxPersistentMemory *resource.Quantity `json:"supportedMaxPersistentMemory,omitempty"`

	// +optional

	// RecommendedPersistentMemory is the recommended default persistent memory
	// size for this guest.
	RecommendedPersistentMemory *resource.Quantity `json:"recommendedPersistentMemory,omitempty"`

	// +optional

	// PersistentMemoryHotAddSupported indicates support for persistent memory
	// hot-add for this guest.
	PersistentMemoryHotAddSupported *bool `json:"persistentMemoryHotAddSupported,omitempty"`

	// +optional

	// PersistentMemoryHotRemoveSupported indicates support for persistent
	// memory hot-remove for this guest.
	PersistentMemoryHotRemoveSupported *bool `json:"persistentMemoryHotRemoveSupported,omitempty"`

	// +optional

	// PersistentMemoryColdGrowthSupported indicates support for virtual NVDIMM
	// cold-growth (capacity increase while powered off) for this guest.
	PersistentMemoryColdGrowthSupported *bool `json:"persistentMemoryColdGrowthSupported,omitempty"`

	// +optional

	// PersistentMemoryColdGrowthGranularity is the granularity for
	// virtual NVDIMM cold-growth operations.
	PersistentMemoryColdGrowthGranularity *ResourceQuantityOption `json:"persistentMemoryColdGrowthGranularity,omitempty"`

	// +optional

	// PersistentMemoryHotGrowthSupported indicates support for virtual NVDIMM
	// hot-growth (capacity increase while powered on) for this guest.
	PersistentMemoryHotGrowthSupported *bool `json:"persistentMemoryHotGrowthSupported,omitempty"`

	// +optional

	// PersistentMemoryHotGrowthGranularity is the granularity for
	// virtual NVDIMM hot-growth operations.
	PersistentMemoryHotGrowthGranularity *ResourceQuantityOption `json:"persistentMemoryHotGrowthGranularity,omitempty"`

	// +optional

	// SupportedForCreate indicates whether this guest OS can be selected
	// during VM creation.
	SupportedForCreate bool `json:"supportedForCreate,omitempty"`

	// +optional

	// SupportLevel indicates the support level for this guest OS.
	//
	// Valid values are:
	//   - Deprecated
	//   - Experimental
	//   - Legacy
	//   - Supported
	//   - TechPreview
	//   - Terminated
	//   - Unsupported
	SupportLevel SupportLevel `json:"supportLevel,omitempty"`
}

type VirtualMachineGuestOptionsSpec struct {
	// +required

	// ID is the desired guest ID for which to get the guest options,
	// ex.: OtherLinux64.
	ID VirtualMachineGuestOSIdentifier `json:"id"`
}

type VirtualMachineGuestOptionsStatus struct {
	// +required

	// FullName is the full descriptive name of the guest OS (e.g.,
	// "Windows 2000 Professional").
	FullName string `json:"fullName"`

	// +optional

	// Family is the family to which this guest OS belongs (e.g.,
	// "Windows", "Linux").
	Family VirtualMachineGuestOSFamily `json:"family,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=hardwareVersion

	// HardwareVersions contains the list of hardware versions for which this
	// guest is valid and the guest's options for that version.
	HardwareVersions []VirtualMachineGuestOptionsHardwareVersionStatus `json:"hardwareVersions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=vmguestoptions
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status

// VirtualMachineGuestOptions is the schema for the
// VirtualMachineGuestOptions API and
// represents the desired state and observed status of a
// VirtualMachineGuestOptions resource.
type VirtualMachineGuestOptions struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineGuestOptionsSpec   `json:"spec,omitempty"`
	Status VirtualMachineGuestOptionsStatus `json:"status,omitempty"`
}
