// Copyright (c) 2024-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

//go:generate go run ../../pkg/gen/guestosid/guestosid.go v1alpha3 zz_virtualmachine_guestosid_generated.go

package v1alpha3

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
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

	// VirtualMachineConditionVMSetResourcePolicyReady indicates that a referenced
	// VirtualMachineSetResourcePolicy is Ready.
	VirtualMachineConditionVMSetResourcePolicyReady = "VirtualMachineConditionVMSetResourcePolicyReady"

	// VirtualMachineConditionStorageReady indicates that the storage prerequisites for the VM are ready.
	VirtualMachineConditionStorageReady = "VirtualMachineStorageReady"

	// VirtualMachineConditionBootstrapReady indicates that the bootstrap prerequisites for the VM are ready.
	VirtualMachineConditionBootstrapReady = "VirtualMachineBootstrapReady"

	// VirtualMachineConditionNetworkReady indicates that the network prerequisites for the VM are ready.
	VirtualMachineConditionNetworkReady = "VirtualMachineNetworkReady"

	// VirtualMachineConditionPlacementReady indicates that the placement decision for the VM is ready.
	VirtualMachineConditionPlacementReady = "VirtualMachineConditionPlacementReady"

	// VirtualMachineConditionCreated indicates that the VM has been created.
	VirtualMachineConditionCreated = "VirtualMachineCreated"
)

const (
	// GuestBootstrapCondition exposes the status of guest bootstrap from within
	// the guest OS, when available.
	GuestBootstrapCondition = "GuestBootstrap"
)

const (
	// GuestCustomizationCondition exposes the status of guest customization
	// from within the guest OS, when available.
	GuestCustomizationCondition = "GuestCustomization"

	// GuestCustomizationIdleReason documents that guest
	// customizations were not applied for the VirtualMachine.
	GuestCustomizationIdleReason = "GuestCustomizationIdle"

	// GuestCustomizationPendingReason documents that guest
	// customization is still pending within the guest OS.
	GuestCustomizationPendingReason = "GuestCustomizationPending"

	// GuestCustomizationRunningReason documents that the guest
	// customization is now running on the guest OS.
	GuestCustomizationRunningReason = "GuestCustomizationRunning"

	// GuestCustomizationSucceededReason documents that the
	// guest customization succeeded within the guest OS.
	GuestCustomizationSucceededReason = "GuestCustomizationSucceeded"

	// GuestCustomizationFailedReason documents that the guest
	// customization failed within the guest OS.
	GuestCustomizationFailedReason = "GuestCustomizationFailed"
)

const (
	// VirtualMachineToolsCondition exposes the status of VMware Tools running
	// in the guest OS, when available.
	VirtualMachineToolsCondition = "VirtualMachineTools"

	// VirtualMachineToolsNotRunningReason documents that
	// VMware Tools is not running.
	VirtualMachineToolsNotRunningReason = "VirtualMachineToolsNotRunning"

	// VirtualMachineToolsRunningReason documents that VMware
	// Tools is running.
	VirtualMachineToolsRunningReason = "VirtualMachineToolsRunning"
)

const (
	// VirtualMachineReconcileReady exposes the status of VirtualMachine reconciliation.
	VirtualMachineReconcileReady = "VirtualMachineReconcileReady"

	// VirtualMachineReconcileRunningReason indicates that VirtualMachine
	// reconciliation is running.
	VirtualMachineReconcileRunningReason = "VirtualMachineReconcileRunning"

	// VirtualMachineReconcilePausedReason indicates that VirtualMachine
	// reconciliation is being paused.
	VirtualMachineReconcilePausedReason = "VirtualMachineReconcilePaused"
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

	// InstanceIDAnnotation is an annotation that can be applied to set Cloud-Init metadata Instance ID.
	//
	// This cannot be set by users. It is for VM Operator to handle corner cases.
	//
	// In a corner case where a VM first boot failed to bootstrap with Cloud-Init, VM Operator sets Instance ID
	// the same with the first boot Instance ID to prevent Cloud-Init from treating this VM as first boot
	// due to different Instance ID. This annotation is used in upgrade script.
	InstanceIDAnnotation = GroupName + "/cloud-init-instance-id"

	// FirstBootDoneAnnotation is an annotation that indicates the VM has been
	// booted at least once. This annotation cannot be set by users and will not
	// be removed once set until the VM is deleted.
	FirstBootDoneAnnotation = "virtualmachine." + GroupName + "/first-boot-done"

	// V1alpha1ConfigMapTransportAnnotation is an annotation that indicates that the VM
	// was created with the v1alpha1 API and specifies a configMap as the metadata transport resource type.
	V1alpha1ConfigMapTransportAnnotation = GroupName + "/v1a1-configmap-md-transport"
)

const (
	// ManagedByExtensionKey and ManagedByExtensionType represent the ManagedBy
	// field on the VM. They are used to differentiate VM Service managed VMs
	// from traditional vSphere VMs.
	ManagedByExtensionKey  = "com.vmware.vcenter.wcp"
	ManagedByExtensionType = "VirtualMachine"
)

// VirtualMachine backup/restore related constants.
const (
	// VMResourceYAMLExtraConfigKey is the ExtraConfig key to persist VM
	// Kubernetes resource YAML, compressed using gzip and base64-encoded.
	VMResourceYAMLExtraConfigKey = "vmservice.virtualmachine.resource.yaml"
	// AdditionalResourcesYAMLExtraConfigKey is the ExtraConfig key to persist
	// VM-relevant Kubernetes resource YAML, compressed using gzip and base64-encoded.
	AdditionalResourcesYAMLExtraConfigKey = "vmservice.virtualmachine.additional.resources.yaml"
	// PVCDiskDataExtraConfigKey is the ExtraConfig key to persist the VM's
	// PVC disk data in JSON, compressed using gzip and base64-encoded.
	PVCDiskDataExtraConfigKey = "vmservice.virtualmachine.pvc.disk.data"
)

const (
	// PauseVMExtraConfigKey is the ExtraConfig key to allow override
	// operations for admins to pause reconciliation of VM Service VM.
	//
	// Please note, the value that takes effect is the string "True"(case-insensitive).
	PauseVMExtraConfigKey = "vmservice.virtualmachine.pause"

	// PausedVMLabelKey is the label key to identify VMs that reconciliation
	// are paused. Value will specify whose operation is responsible for
	// the pause. It can be admins or devops or both.
	//
	// Only privileged user can edit this label.
	PausedVMLabelKey = GroupName + "/paused"
)

// +kubebuilder:validation:Enum=PoweredOff;PoweredOn;Suspended

// VirtualMachinePowerState defines a VM's desired and observed power states.
type VirtualMachinePowerState string

const (
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

// +kubebuilder:validation:Enum=Hard;Soft;TrySoft

// VirtualMachinePowerOpMode represents the various power operation modes when
// powering off or suspending a VM.
type VirtualMachinePowerOpMode string

const (
	// VirtualMachinePowerOpModeHard indicates to halt a VM when powering it
	// off or when suspending a VM to not involve the guest.
	VirtualMachinePowerOpModeHard VirtualMachinePowerOpMode = "Hard"

	// VirtualMachinePowerOpModeSoft indicates to ask VM Tools running
	// inside of a VM's guest to shutdown the guest gracefully when powering
	// off a VM or when suspending a VM to allow the guest to participate.
	//
	// If this mode is set on a VM whose guest does not have VM Tools or if
	// VM Tools is present but the operation fails, the VM may never realize
	// the desired power state. This can prevent a VM from being deleted as well
	// as many other unexpected issues. It is recommended to use trySoft
	// instead.
	VirtualMachinePowerOpModeSoft VirtualMachinePowerOpMode = "Soft"

	// VirtualMachinePowerOpModeTrySoft indicates to first attempt a Soft
	// operation and fall back to Hard if VM Tools is not present in the guest,
	// if the Soft operation fails, or if the VM is not in the desired power
	// state within five minutes.
	VirtualMachinePowerOpModeTrySoft VirtualMachinePowerOpMode = "TrySoft"
)

type VirtualMachineImageRef struct {
	// Kind describes the type of image, either a namespace-scoped
	// VirtualMachineImage or cluster-scoped ClusterVirtualMachineImage.
	Kind string `json:"kind"`

	// Name refers to the name of a VirtualMachineImage resource in the same
	// namespace as this VM or a cluster-scoped ClusterVirtualMachineImage.
	Name string `json:"name"`
}

// VirtualMachineSpec defines the desired state of a VirtualMachine.
type VirtualMachineSpec struct {

	// +optional

	// Image describes the reference to the VirtualMachineImage or
	// ClusterVirtualMachineImage resource used to deploy this VM.
	//
	// Please note, unlike the field spec.imageName, the value of
	// spec.image.name MUST be a Kubernetes object name.
	//
	// Please also note, when creating a new VirtualMachine, if this field and
	// spec.imageName are both non-empty, then they must refer to the same
	// resource or an error is returned.
	Image *VirtualMachineImageRef `json:"image,omitempty"`

	// +optional

	// ImageName describes the name of the image resource used to deploy this
	// VM.
	//
	// This field may be used to specify the name of a VirtualMachineImage
	// or ClusterVirtualMachineImage resource. The resolver first checks to see
	// if there is a VirtualMachineImage with the specified name in the
	// same namespace as the VM being deployed. If no such resource exists, the
	// resolver then checks to see if there is a ClusterVirtualMachineImage
	// resource with the specified name.
	//
	// This field may also be used to specify the display name (vSphere name) of
	// a VirtualMachineImage or ClusterVirtualMachineImage resource. If the
	// display name unambiguously resolves to a distinct VM image (among all
	// existing VirtualMachineImages in the VM's namespace and all existing
	// ClusterVirtualMachineImages), then a mutation webhook updates the
	// spec.image field with the reference to the resolved VM image. If the
	// display name resolves to multiple or no VM images, then the mutation
	// webhook denies the request and returns an error.
	//
	// Please also note, when creating a new VirtualMachine, if this field and
	// spec.image are both non-empty, then they must refer to the same
	// resource or an error is returned.
	ImageName string `json:"imageName,omitempty"`

	// +optional

	// ClassName describes the name of the VirtualMachineClass resource used to
	// deploy this VM.
	ClassName string `json:"className,omitempty"`

	// +optional

	// StorageClass describes the name of a Kubernetes StorageClass resource
	// used to configure this VM's storage-related attributes.
	//
	// Please see https://kubernetes.io/docs/concepts/storage/storage-classes/
	// for more information on Kubernetes storage classes.
	StorageClass string `json:"storageClass,omitempty"`

	// +optional

	// Bootstrap describes the desired state of the guest's bootstrap
	// configuration.
	//
	// If omitted, a default bootstrap method may be selected based on the
	// guest OS identifier. If Linux, then the LinuxPrep method is used.
	Bootstrap *VirtualMachineBootstrapSpec `json:"bootstrap,omitempty"`

	// +optional

	// Network describes the desired network configuration for the VM.
	//
	// Please note this value may be omitted entirely and the VM will be
	// assigned a single, virtual network interface that is connected to the
	// Namespace's default network.
	Network *VirtualMachineNetworkSpec `json:"network,omitempty"`

	// +optional

	// PowerState describes the desired power state of a VirtualMachine.
	//
	// Please note this field may be omitted when creating a new VM and will
	// default to "PoweredOn." However, once the field is set to a non-empty
	// value, it may no longer be set to an empty value.
	//
	// Additionally, setting this value to "Suspended" is not supported when
	// creating a new VM. The valid values when creating a new VM are
	// "PoweredOn" and "PoweredOff." An empty value is also allowed on create
	// since this value defaults to "PoweredOn" for new VMs.
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`

	// +optional
	// +kubebuilder:default=TrySoft

	// PowerOffMode describes the desired behavior when powering off a VM.
	//
	// There are three, supported power off modes: Hard, Soft, and
	// TrySoft. The first mode, Hard, is the equivalent of a physical
	// system's power cord being ripped from the wall. The Soft mode
	// requires the VM's guest to have VM Tools installed and attempts to
	// gracefully shutdown the VM. Its variant, TrySoft, first attempts
	// a graceful shutdown, and if that fails or the VM is not in a powered off
	// state after five minutes, the VM is halted.
	//
	// If omitted, the mode defaults to TrySoft.
	PowerOffMode VirtualMachinePowerOpMode `json:"powerOffMode,omitempty"`

	// +optional
	// +kubebuilder:default=TrySoft

	// SuspendMode describes the desired behavior when suspending a VM.
	//
	// There are three, supported suspend modes: Hard, Soft, and
	// TrySoft. The first mode, Hard, is where vSphere suspends the VM to
	// disk without any interaction inside of the guest. The Soft mode
	// requires the VM's guest to have VM Tools installed and attempts to
	// gracefully suspend the VM. Its variant, TrySoft, first attempts
	// a graceful suspend, and if that fails or the VM is not in a put into
	// standby by the guest after five minutes, the VM is suspended.
	//
	// If omitted, the mode defaults to TrySoft.
	SuspendMode VirtualMachinePowerOpMode `json:"suspendMode,omitempty"`

	// +optional

	// NextRestartTime may be used to restart the VM, in accordance with
	// RestartMode, by setting the value of this field to "now"
	// (case-insensitive).
	//
	// A mutating webhook changes this value to the current time (UTC), which
	// the VM controller then uses to determine the VM should be restarted by
	// comparing the value to the timestamp of the last time the VM was
	// restarted.
	//
	// Please note it is not possible to schedule future restarts using this
	// field. The only value that users may set is the string "now"
	// (case-insensitive).
	NextRestartTime string `json:"nextRestartTime,omitempty"`

	// +optional
	// +kubebuilder:default=TrySoft

	// RestartMode describes the desired behavior for restarting a VM when
	// spec.nextRestartTime is set to "now" (case-insensitive).
	//
	// There are three, supported suspend modes: Hard, Soft, and
	// TrySoft. The first mode, Hard, is where vSphere resets the VM without any
	// interaction inside of the guest. The Soft mode requires the VM's guest to
	// have VM Tools installed and asks the guest to restart the VM. Its
	// variant, TrySoft, first attempts a soft restart, and if that fails or
	// does not complete within five minutes, the VM is hard reset.
	//
	// If omitted, the mode defaults to TrySoft.
	RestartMode VirtualMachinePowerOpMode `json:"restartMode,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// Volumes describes a list of volumes that can be mounted to the VM.
	Volumes []VirtualMachineVolume `json:"volumes,omitempty"`

	// +optional

	// ReadinessProbe describes a probe used to determine the VM's ready state.
	ReadinessProbe *VirtualMachineReadinessProbeSpec `json:"readinessProbe,omitempty"`

	// +optional

	// Advanced describes a set of optional, advanced VM configuration options.
	Advanced *VirtualMachineAdvancedSpec `json:"advanced,omitempty"`

	// +optional

	// Reserved describes a set of VM configuration options reserved for system
	// use.
	//
	// Please note attempts to modify the value of this field by a DevOps user
	// will result in a validation error.
	Reserved *VirtualMachineReservedSpec `json:"reserved,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=13

	// MinHardwareVersion describes the desired, minimum hardware version.
	//
	// The logic that determines the hardware version is as follows:
	//
	// 1. If this field is set, then its value is used.
	// 2. Otherwise, if the VirtualMachineClass used to deploy the VM contains a
	//    non-empty hardware version, then it is used.
	// 3. Finally, if the hardware version is still undetermined, the value is
	//    set to the default hardware version for the Datacenter/Cluster/Host
	//    where the VM is provisioned.
	//
	// This field is never updated to reflect the derived hardware version.
	// Instead, VirtualMachineStatus.HardwareVersion surfaces
	// the observed hardware version.
	//
	// Please note, setting this field's value to N ensures a VM's hardware
	// version is equal to or greater than N. For example, if a VM's observed
	// hardware version is 10 and this field's value is 13, then the VM will be
	// upgraded to hardware version 13. However, if the observed hardware
	// version is 17 and this field's value is 13, no change will occur.
	//
	// Several features are hardware version dependent, for example:
	//
	// * NVMe Controllers                >= 14
	// * Dynamic Direct Path I/O devices >= 17
	//
	// Please refer to https://kb.vmware.com/s/article/1003746 for a list of VM
	// hardware versions.
	//
	// It is important to remember that a VM's hardware version may not be
	// downgraded and upgrading a VM deployed from an image based on an older
	// hardware version to a more recent one may result in unpredictable
	// behavior. In other words, please be careful when choosing to upgrade a
	// VM to a newer hardware version.
	MinHardwareVersion int32 `json:"minHardwareVersion,omitempty"`

	// +optional
	// +kubebuilder:validation:Format:=uuid4

	// BiosUUID describes the desired BIOS UUID for a VM.
	// If omitted, this field defaults to a random UUID.
	// When the bootstrap provider is Cloud-Init, this value is used as the
	// default value for spec.bootstrap.cloudInit.instanceID if it is omitted.
	BiosUUID string `json:"biosUUID,omitempty"`
}

// VirtualMachineReservedSpec describes a set of VM configuration options
// reserved for system use. Modification attempts by DevOps users will result
// in a validation error.
type VirtualMachineReservedSpec struct {
	// +optional

	// ResourcePolicyName describes the name of a
	// VirtualMachineSetResourcePolicy resource used to configure the VM's

	ResourcePolicyName string `json:"resourcePolicyName,omitempty"`
}

// VirtualMachineAdvancedSpec describes a set of optional, advanced VM
// configuration options.
type VirtualMachineAdvancedSpec struct {
	// +optional

	// BootDiskCapacity is the capacity of the VM's boot disk -- the first disk
	// from the VirtualMachineImage from which the VM was deployed.
	//
	// Please note it is not advised to change this value while the VM is
	// running. Also, resizing the VM's boot disk may require actions inside of
	// the guest to take advantage of the additional capacity. Finally, changing
	// the size of the VM's boot disk, even increasing it, could adversely
	// affect the VM.
	BootDiskCapacity *resource.Quantity `json:"bootDiskCapacity,omitempty"`

	// +optional

	// DefaultVolumeProvisioningMode specifies the default provisioning mode for
	// persistent volumes managed by this VM.
	DefaultVolumeProvisioningMode VirtualMachineVolumeProvisioningMode `json:"defaultVolumeProvisioningMode,omitempty"`

	// +optional

	// ChangeBlockTracking is a flag that enables incremental backup support
	// for this VM, a feature utilized by external backup systems such as
	// VMware Data Recovery.
	ChangeBlockTracking *bool `json:"changeBlockTracking,omitempty"`
}

// VirtualMachineStatus defines the observed state of a VirtualMachine instance.
type VirtualMachineStatus struct {
	// +optional

	// Class is a reference to the VirtualMachineClass resource used to deploy
	// this VM.
	Class *vmopv1common.LocalObjectRef `json:"class,omitempty"`

	// +optional

	// Host describes the hostname or IP address of the infrastructure host
	// where the VM is executed.
	Host string `json:"host,omitempty"`

	// +optional

	// PowerState describes the observed power state of the VirtualMachine.
	PowerState VirtualMachinePowerState `json:"powerState,omitempty"`

	// +optional

	// Conditions describes the observed conditions of the VirtualMachine.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional

	// Network describes the observed state of the VM's network configuration.
	// Please note much of the network status information is only available if
	// the guest has VM Tools installed.
	Network *VirtualMachineNetworkStatus `json:"network,omitempty"`

	// +optional

	// UniqueID describes a unique identifier that is provided by the underlying
	// infrastructure provider, such as vSphere.
	UniqueID string `json:"uniqueID,omitempty"`

	// +optional

	// BiosUUID describes a unique identifier provided by the underlying
	// infrastructure provider that is exposed to the Guest OS BIOS as a unique
	// hardware identifier.
	BiosUUID string `json:"biosUUID,omitempty"`

	// +optional

	// InstanceUUID describes the unique instance UUID provided by the
	// underlying infrastructure provider, such as vSphere.
	InstanceUUID string `json:"instanceUUID,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// Volumes describes a list of current status information for each Volume
	// that is desired to be attached to the VM.
	Volumes []VirtualMachineVolumeStatus `json:"volumes,omitempty"`

	// +optional

	// ChangeBlockTracking describes the CBT enablement status on the VM.
	ChangeBlockTracking *bool `json:"changeBlockTracking,omitempty"`

	// +optional

	// Zone describes the availability zone where the VirtualMachine has been
	// scheduled.
	//
	// Please note this field may be empty when the cluster is not zone-aware.
	Zone string `json:"zone,omitempty"`

	// +optional

	// LastRestartTime describes the last time the VM was restarted.
	LastRestartTime *metav1.Time `json:"lastRestartTime,omitempty"`

	// +optional

	// HardwareVersion describes the VirtualMachine resource's observed
	// hardware version.
	//
	// Please refer to VirtualMachineSpec.MinHardwareVersion for more
	// information on the topic of a VM's hardware version.
	HardwareVersion int32 `json:"hardwareVersion,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vm
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Power-State",type="string",JSONPath=".status.powerState"
// +kubebuilder:printcolumn:name="Class",type="string",priority=1,JSONPath=".spec.className"
// +kubebuilder:printcolumn:name="Image",type="string",priority=1,JSONPath=".spec.image.name"
// +kubebuilder:printcolumn:name="Primary-IP4",type="string",priority=1,JSONPath=".status.network.primaryIP4"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// VirtualMachine is the schema for the virtualmachines API and represents the
// desired state and observed status of a virtualmachines resource.
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec,omitempty"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

func (vm *VirtualMachine) NamespacedName() string {
	return vm.Namespace + "/" + vm.Name
}

func (vm *VirtualMachine) GetConditions() []metav1.Condition {
	return vm.Status.Conditions
}

func (vm *VirtualMachine) SetConditions(conditions []metav1.Condition) {
	vm.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// VirtualMachineList contains a list of VirtualMachine.
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachine{}, &VirtualMachineList{})
}
