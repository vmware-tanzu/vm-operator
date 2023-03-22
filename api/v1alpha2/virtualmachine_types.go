// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha2/common"
)

const (
	// VirtualMachineConditionClassReady indicates that a referenced
	// VirtualMachineClass is ready.
	//
	// For more information please see VirtualMachineClass.Status.Ready.
	VirtualMachineConditionClassReady = "VirtualMachineClassReady"

	// VirtualMachineConditionImageReady indicates that a referenced
	// VirtualMachineImage is ready.
	//
	// For more information please see VirtualMachineImage.Status.Ready.
	VirtualMachineConditionImageReady = "VirtualMachineImageReady"

	VirtualMachineConditionNetworkReady = "VirtualMachineNetworkReady"

	VirtualMachineConditionStorageReady = "VirtualMachineStorageReady"

	VirtualMachineConditionBootstrapReady = "VirtualMachineBootstrapReady"
)

const (
	// GuestCustomizationCondition exposes the status of guest customization
	// from within the guest OS, when available.
	GuestCustomizationCondition = "GuestCustomization"

	// GuestCustomizationIdleReason (Severity=Info) documents that guest
	// customizations were not applied for the VirtualMachine.
	GuestCustomizationIdleReason = "GuestCustomizationIdle"

	// GuestCustomizationPendingReason (Severity=Info) documents that guest
	// customization is still pending within the guest OS.
	GuestCustomizationPendingReason = "GuestCustomizationPending"

	// GuestCustomizationRunningReason (Severity=Info) documents that the guest
	// customization is now running on the guest OS.
	GuestCustomizationRunningReason = "GuestCustomizationRunning"

	// GuestCustomizationSucceededReason (Severity=Info) documents that the
	// guest customization succeeded within the guest OS.
	GuestCustomizationSucceededReason = "GuestCustomizationSucceeded"

	// GuestCustomizationFailedReason (Severity=Error) documents that the guest
	// customization failed within the guest OS.
	GuestCustomizationFailedReason = "GuestCustomizationFailed"
)

const (
	// VirtualMachineToolsCondition exposes the status of VMware Tools running
	// in the guest OS, when available.
	VirtualMachineToolsCondition = "VirtualMachineTools"

	// VirtualMachineToolsNotRunningReason (Severity=Error) documents that
	// VMware Tools is not running.
	VirtualMachineToolsNotRunningReason = "VirtualMachineToolsNotRunning"

	// VirtualMachineToolsRunningReason (Severity=Info) documents that VMware
	// Tools is running.
	VirtualMachineToolsRunningReason = "VirtualMachineToolsRunning"
)

const (
	// PauseAnnotation is an annotation that prevents a VM from being
	// reconciled.
	//
	// This can be used when a VM needs to be modified directly on the
	// underlying infrastructure without VM Service attempting to direct the
	// VM back to its intended state.
	//
	// The VM will not be reconciled again until this annotation is removed.
	PauseAnnotation = GroupName + "/pause-reconcile"
)

// VirtualMachinePowerState defines a VM's desired and observed power states.
// +kubebuilder:validation:Enum=GuestPoweredOff;PoweredOff;PoweredOn;Suspended
type VirtualMachinePowerState string

const (
	// VirtualMachinePowerStateGuestOff indicates to shut down a VM's guest.
	// Please note when a VM is shut down this way, its observed power state
	// will be VirtualMachinePowerStateOff.
	VirtualMachinePowerStateGuestOff VirtualMachinePowerState = "GuestPoweredOff"

	// VirtualMachinePowerStateOff indicates to shut down a VM and/or it is
	// shut down.
	VirtualMachinePowerStateOff VirtualMachinePowerState = "PoweredOff"

	// VirtualMachinePowerStateOn indicates to power on a VM and/or it is
	// powered on.
	VirtualMachinePowerStateOn VirtualMachinePowerState = "PoweredOn"

	// VirtualMachinePowerStateSuspended indicates to suspend a VM and/or it is
	// suspended.
	VirtualMachinePowerStateSuspended VirtualMachinePowerState = "Suspended"
)

// VirtualMachineSpec defines the desired state of a VirtualMachine.
type VirtualMachineSpec struct {
	// ImageName describes the name of the image resource used to deploy this
	// VM.
	//
	// This field may be used to specify the name of a VirtualMachineImage
	// or ClusterVirtualMachineImage resource. The resolver first checks to see
	// if there is a ClusterVirtualMachineImage with the specified name. If no
	// such resource exists, the resolver then checks to see if there is a
	// VirtualMachineImage resource with the specified name in the same
	// Namespace as the VM being deployed.
	//
	// This field is optional in the cases where there exists a sensible
	// default value, such as when there is a single VirtualMachineImage
	// resource available in the same Namespace as the VM being deployed.
	//
	// +optional
	ImageName string `json:"imageName,omitempty"`

	// Class describes the name of the VirtualMachineClass resource used to
	// deploy this VM.
	//
	// This field is optional in the cases where there exists a sensible
	// default value, such as when there is a single VirtualMachineClass
	// resource available in the same Namespace as the VM being deployed.
	//
	// +optional
	ClassName string `json:"className,omitempty"`

	// StorageClass describes the name of a Kubernetes StorageClass resource
	// used to configure this VM's storage-related attributes.
	//
	// Please see https://kubernetes.io/docs/concepts/storage/storage-classes/
	// for more information on Kubernetes storage classes.
	//
	// This field is optional in the cases where there exists a sensible
	// default value, such as when there is a single StorageClass
	// resource available in the same Namespace as the VM being deployed.
	//
	// +optional
	StorageClass string `json:"storageClass,omitempty"`

	// Bootstrap describes the desired state of the guest's bootstrap
	// configuration.
	//
	// If omitted, then the bootstrap method is determined based on the guest
	// identifier from the VirtualMachineImage. If the image's guest OS type is
	// Windows, then the Sysprep bootstrap method is used; if Linux, the
	// LinuxPrep method is used.
	//
	// Please note that defaulting to Sysprep for Windows images only works if
	// the image uses a volume license key, otherwise the image's product ID is
	// required.
	//
	// +optional
	Bootstrap VirtualMachineBootstrapSpec `json:"bootstrap,omitempty"`

	// Network describes the desired network configuration for the VM.
	//
	// Please note this value may be omitted entirely and the VM will be
	// assigned a single, virtual network interface that is connected to the
	// Namespace's default network.
	//
	// +optional
	Network VirtualMachineNetworkSpec `json:"network,omitempty"`

	// PowerState describes the desired power state of a VirtualMachine.
	//
	// +optional
	// +kubebuilder:default=PoweredOn
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`

	// Volumes describes a list of volumes that can be mounted to the VM.
	//
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []VirtualMachineVolume `json:"volumes,omitempty"`

	// ReadinessProbe describes a probe used to determine the VM's ready state.
	//
	// +optional
	ReadinessProbe VirtualMachineReadinessProbeSpec `json:"readinessProbe,omitempty"`

	// ReadinessGates, if specified, will be evaluated to determine the VM's
	// readiness.
	//
	// A VM is ready when its readiness probe, if specified, is true AND all of
	// the conditions specified by the readiness gates have a status equal to
	// "True".
	//
	// +optional
	// +listType=map
	// +listMapKey=conditionType
	ReadinessGates []VirtualMachineReadinessGate `json:"readinessGates,omitempty"`

	// Advanced describes a set of optional, advanced VM configuration options.
	// +optional
	Advanced VirtualMachineAdvancedSpec `json:"advanced,omitempty"`

	// Reserved describes a set of VM configuration options reserved for system
	// use.
	//
	// Please note attempts to modify the value of this field by a DevOps user
	// will result in a validation error.
	//
	// +optional
	Reserved VirtualMachineReservedSpec `json:"reserved,omitempty"`
}

// VirtualMachineReservedSpec describes a set of VM configuration options
// reserved for system use. Modification attempts by DevOps users will result
// in a validation error.
type VirtualMachineReservedSpec struct {
	// ResourcePolicyName describes the name of a
	// VirtualMachineSetResourcePolicy resource used to configure the VM's
	// resource policy.
	//
	// +optional
	ResourcePolicyName string `json:"resourcePolicyName,omitempty"`
}

// VirtualMachineAdvancedSpec describes a set of optional, advanced VM
// configuration options.
type VirtualMachineAdvancedSpec struct {
	// BootDiskCapacity is the capacity of the VM's boot disk -- the first disk
	// from the VirtualMachineImage from which the VM was deployed.
	//
	// Please note it is not advised to change this value while the VM is
	// running. Also, resizing the VM's boot disk may require actions inside of
	// the guest to take advantage of the additional capacity. Finally, changing
	// the size of the VM's boot disk, even increasing it, could adversely
	// affect the VM.
	//
	// +optional
	BootDiskCapacity resource.Quantity `json:"bootDiskCapacity,omitempty"`

	// DefaultVolumeProvisioningMode specifies the default provisioning mode for
	// persistent volumes managed by this VM.
	DefaultVolumeProvisioningMode string `json:"defaultVolumeProvisioningMode,omitempty"`

	// ChangeBlockTracking is a flag that enables incremental backup support
	// for this VM, a feature utilized by external backup systems such as
	// VMware Data Recovery.
	//
	// +optional
	ChangeBlockTracking bool `json:"changeBlockTracking,omitempty"`
}

// VirtualMachineStatus defines the observed state of a VirtualMachine instance.
type VirtualMachineStatus struct {
	// Image is a reference to the VirtualMachineImage resource used to deploy
	// this VM.
	//
	// +optional
	Image *common.LocalObjectRef `json:"image,omitempty"`

	// Class is a reference to the VirtualMachineClass resource used to deploy
	// this VM.
	//
	// +optional
	Class *common.LocalObjectRef `json:"class,omitempty"`

	// Host describes the hostname or IP address of the infrastructure host
	// where the VM is executed.
	//
	// +optional
	Host string `json:"host,omitempty"`

	// PowerState describes the observed power state of the VirtualMachine.
	// +optional
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`

	// Conditions describes the observed conditions of the VirtualMachine.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Network describes the observed state of the VM's network configuration.
	// Please note much of the network status information is only available if
	// the guest has VM Tools installed.
	// +optional
	Network *VirtualMachineNetworkStatus `json:"network,omitempty"`

	// UniqueID describes a unique identifier that is provided by the underlying
	// infrastructure provider, such as vSphere.
	//
	// +optional
	UniqueID string `json:"uniqueID,omitempty"`

	// BiosUUID describes a unique identifier provided by the underlying
	// infrastructure provider that is exposed to the Guest OS BIOS as a unique
	// hardware identifier.
	//
	// +optional
	BiosUUID string `json:"biosUUID,omitempty"`

	// InstanceUUID describes the unique instance UUID provided by the
	// underlying infrastructure provider, such as vSphere.
	//
	// +optional
	InstanceUUID string `json:"instanceUUID,omitempty"`

	// Volumes describes a list of current status information for each Volume
	// that is desired to be attached to the VM.
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []VirtualMachineVolumeStatus `json:"volumes,omitempty"`

	// ChangeBlockTracking describes the CBT enablement status on the VM.
	//
	// +optional
	ChangeBlockTracking *bool `json:"changeBlockTracking,omitempty"`

	// Zone describes the availability zone where the VirtualMachine has been
	// scheduled.
	//
	// Please note this field may be empty when the cluster is not zone-aware.
	//
	// +optional
	Zone string `json:"zone,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vm
// +kubebuilder:storageversion:false
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Class",type="string",priority=1,JSONPath=".status.class.name"
// +kubebuilder:printcolumn:name="Image",type="string",priority=1,JSONPath=".status.image.name"
// +kubebuilder:printcolumn:name="PowerState",type="string",JSONPath=".status.powerState"
// +kubebuilder:printcolumn:name="Primary-IP",type="string",priority=1,JSONPath=".status.vmIp"

// VirtualMachine is the schema for the virtualmachines API and represents the
// desired state and observed status of a virtualmachines resource.
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec,omitempty"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

func (vm VirtualMachine) NamespacedName() string {
	return vm.Namespace + "/" + vm.Name
}

// +kubebuilder:object:root=true

// VirtualMachineList contains a list of VirtualMachine.
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachine `json:"items"`
}

func init() {
	RegisterTypeWithScheme(&VirtualMachine{}, &VirtualMachineList{})
}
