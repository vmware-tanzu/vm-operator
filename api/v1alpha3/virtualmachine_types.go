// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

//go:generate go -C ../.. run ./pkg/gen/guestosid/guestosid.go v1alpha3 ./api/v1alpha3/zz_virtualmachine_guestosid_generated.go

package v1alpha3

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1a3common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
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

	// VirtualMachineEncryptionSynced indicates that the VirtualMachine's
	// encryption state is synced to the desired encryption state.
	VirtualMachineEncryptionSynced = "VirtualMachineEncryptionSynced"

	// VirtualMachineConditionCreated indicates that the VM has been created.
	VirtualMachineConditionCreated = "VirtualMachineCreated"

	// VirtualMachineClassConfigurationSynced indicates that the VM's current configuration is synced to the
	// current version of its VirtualMachineClass.
	VirtualMachineClassConfigurationSynced = "VirtualMachineClassConfigurationSynced"
)

const (
	// GuestBootstrapCondition exposes the status of guest bootstrap from within
	// the guest OS, when available.
	GuestBootstrapCondition = "GuestBootstrap"

	// GuestIDReconfiguredCondition exposes the status of guest ID
	// reconfiguration after a VM has been created, when available.
	GuestIDReconfiguredCondition = "GuestIDReconfigured"
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
	PauseAnnotation = GroupName + "/paused"

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

	// VirtualMachineSameVMClassResizeAnnotation is an annotation that indicates the VM
	// should be resized as the class it points to changes.
	VirtualMachineSameVMClassResizeAnnotation = GroupName + "/same-vm-class-resize"
)

const (
	// ManagedByExtensionKey and ManagedByExtensionType represent the ManagedBy
	// field on the VM. They are used to differentiate VM Service managed VMs
	// from traditional vSphere VMs.
	ManagedByExtensionKey  = "com.vmware.vcenter.wcp"
	ManagedByExtensionType = "VirtualMachine"
)

const (
	// VirtualMachineBackupUpToDateCondition exposes the status of the latest VirtualMachine Backup, when available.
	VirtualMachineBackupUpToDateCondition = "VirtualMachineBackupUpToDate"

	// VirtualMachineBackupPausedReason documents that VirtualMachine backup is paused.
	// This can happen after the backing virtual machine is restored by a backup/restore vendor, or a failover operation
	// by a data protection solution. In either of these operations, VM operator does not persist backup information and
	// waits for the virtual machine to be (re)-registered with VM Service.
	VirtualMachineBackupPausedReason = "VirtualMachineBackupPaused"

	// VirtualMachineBackupFailedReason documents that the VirtualMachine backup failed due to an error.
	VirtualMachineBackupFailedReason = "VirtualMachineBackupFailed"
)

const (
	// ForceEnableBackupAnnotation is an annotation that instructs VM operator to
	// ignore all exclusion rules and persist the configuration of the resource in
	// virtual machine in relevant ExtraConfig fields.
	//
	// This is an experimental flag which only guarantees that the configuration
	// of the VirtualMachine resource will be persisted in the virtual machine on
	// vSphere.  There is no guarantee that the registration of the VM will be
	// successful post a restore or failover operation.
	ForceEnableBackupAnnotation = GroupName + "/force-enable-backup"

	// VirtualMachineBackupVersionAnnotation is an annotation that indicates the VM's
	// last backup version. It is a monotonically increasing counter and
	// is only supposed to be used by IaaS control plane and vCenter for virtual machine registration
	// post a restore operation.
	//
	// The VirtualMachineBackupVersionAnnotation on the VM resource in Supervisor and the BackupVersionExtraConfigKey on the vSphere VM
	// indicate whether the backups are in sync.
	VirtualMachineBackupVersionAnnotation = GroupName + "/backup-version"
)

const (
	// ManagerID on a VirtualMachine contains the UUID of the
	// VMware vCenter (VC) that is managing this virtual machine.
	ManagerID = GroupName + "/manager-id"

	// RestoredVMAnnotation on a VirtualMachine represents that a virtual
	// machine has been restored using the RegisterVM API, typically by a
	// VADP based data protection vendor. The presence of this annotation is
	// used to bypass some validation checks that are otherwise
	// applicable to all VirtualMachine create/update requests.
	RestoredVMAnnotation = GroupName + "/restored-vm"

	// ImportedVMAnnotation on a VirtualMachine represents that a traditional virtual
	// machine has been imported into Supervisor using the ImportVM API. The presence of this
	// annotation is used to bypass some validation checks that are otherwise applicable
	// to all VirtualMachine create/update requests.
	ImportedVMAnnotation = GroupName + "/imported-vm"

	// FailedOverVMAnnotation on a VirtualMachine resource represents that a virtual
	// machine has been failed over from one site to the other, typically as part of a
	// disaster recovery workflow.  The presence of this annotation is used to bypass
	// some validation checks that are otherwise applicable to all VirtualMachine
	// create/update requests.
	FailedOverVMAnnotation = GroupName + "/failed-over-vm"
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

// VirtualMachineCdromSpec describes the desired state of a CD-ROM device.
type VirtualMachineCdromSpec struct {
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

// VirtualMachineCryptoSpec defines the desired state of a VirtualMachine's
// encryption state.
type VirtualMachineCryptoSpec struct {
	// +optional

	// EncryptionClassName describes the name of the EncryptionClass resource
	// used to encrypt this VM.
	//
	// Please note, this field is not required to encrypt the VM. If the
	// underlying platform has a default key provider, the VM may still be fully
	// or partially encrypted depending on the specified storage and VM classes.
	//
	// If there is a default key provider and an encryption storage class is
	// selected, the files in the VM's home directory and non-PVC virtual disks
	// will be encrypted
	//
	// If there is a default key provider and a VM Class with a virtual, trusted
	// platform module (vTPM) is selected, the files in the VM's home directory,
	// minus any virtual disks, will be encrypted.
	//
	// If the underlying vSphere platform does not have a default key provider,
	// then this field is required when specifying an encryption storage class
	// and/or a VM Class with a vTPM.
	//
	// If this field is set, spec.storageClass must use an encryption-enabled
	// storage class.
	EncryptionClassName string `json:"encryptionClassName,omitempty"`

	// +optional
	// +kubebuilder:default=true

	// UseDefaultKeyProvider describes the desired behavior for when an explicit
	// EncryptionClass is not provided.
	//
	// When an explicit EncryptionClass is not provided and this value is true:
	//
	// - Deploying a VirtualMachine with an encryption storage policy or vTPM
	//   will be encrypted using the default key provider.
	//
	// - If a VirtualMachine is not encrypted, uses an encryption storage
	//   policy or has a virtual, trusted platform module (vTPM), there is a
	//   default key provider, the VM will be encrypted using the default key
	//   provider.
	//
	// - If a VirtualMachine is encrypted with a provider other than the default
	//   key provider, the VM will be rekeyed using the default key provider.
	//
	// When an explicit EncryptionClass is not provided and this value is false:
	//
	// - Deploying a VirtualMachine with an encryption storage policy or vTPM
	//   will fail.
	//
	// - If a VirtualMachine is encrypted with a provider other than the default
	//   key provider, the VM will be not be rekeyed.
	//
	//   Please note, this could result in a VirtualMachine that cannot be
	//   powered on since it is encrypted using a provider or key that may have
	//   been removed. Without the key, the VM cannot be decrypted and thus
	//   cannot be powered on.
	//
	// Defaults to true if omitted.
	UseDefaultKeyProvider *bool `json:"useDefaultKeyProvider,omitempty"`
}

// VirtualMachineSpec defines the desired state of a VirtualMachine.
type VirtualMachineSpec struct {
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

	// Image describes the reference to the VirtualMachineImage or
	// ClusterVirtualMachineImage resource used to deploy this VM.
	//
	// Please note, unlike the field spec.imageName, the value of
	// spec.image.name MUST be a Kubernetes object name.
	//
	// Please also note, when creating a new VirtualMachine, if this field and
	// spec.imageName are both non-empty, then they must refer to the same
	// resource or an error is returned.
	//
	// Please note, this field *may* be empty if the VM was imported instead of
	// deployed by VM Operator. An imported VirtualMachine resource references
	// an existing VM on the underlying platform that was not deployed from a
	// VM image.
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
	//
	// Please note, this field *may* be empty if the VM was imported instead of
	// deployed by VM Operator. An imported VirtualMachine resource references
	// an existing VM on the underlying platform that was not deployed from a
	// VM image.
	ImageName string `json:"imageName,omitempty"`

	// +optional

	// ClassName describes the name of the VirtualMachineClass resource used to
	// deploy this VM.
	//
	// Please note, this field *may* be empty if the VM was imported instead of
	// deployed by VM Operator. An imported VirtualMachine resource references
	// an existing VM on the underlying platform that was not deployed from a
	// VM class.
	ClassName string `json:"className,omitempty"`

	// +optional

	// Crypto describes the desired encryption state of the VirtualMachine.
	Crypto *VirtualMachineCryptoSpec `json:"crypto,omitempty"`

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
	// +kubebuilder:validation:Format:=uuid

	// InstanceUUID describes the desired Instance UUID for a VM.
	// If omitted, this field defaults to a random UUID.
	// This value is only used for the VM Instance UUID,
	// it is not used within cloudInit.
	// This identifier is used by VirtualCenter to uniquely identify all
	// virtual machine instances, including those that may share the same BIOS UUID.
	InstanceUUID string `json:"instanceUUID,omitempty"`

	// +optional
	// +kubebuilder:validation:Format:=uuid

	// BiosUUID describes the desired BIOS UUID for a VM.
	// If omitted, this field defaults to a random UUID.
	// When the bootstrap provider is Cloud-Init, this value is used as the
	// default value for spec.bootstrap.cloudInit.instanceID if it is omitted.
	BiosUUID string `json:"biosUUID,omitempty"`

	// +optional

	// GuestID describes the desired guest operating system identifier for a VM.
	//
	// The logic that determines the guest ID is as follows:
	//
	// If this field is set, then its value is used.
	// Otherwise, if the VM is deployed from an OVF template that defines a
	// guest ID, then that value is used.
	// The guest ID from VirtualMachineClass used to deploy the VM is ignored.
	//
	// For a complete list of supported values, refer to https://bit.ly/3TiZX3G.
	// Note that some guest ID values may require a minimal hardware version,
	// which can be set using the `spec.minHardwareVersion` field.
	// To see the mapping between virtual hardware versions and the product
	// versions that support a specific guest ID, visit the following link:
	// https://knowledge.broadcom.com/external/article/315655/virtual-machine-hardware-versions.html
	//
	// Please note that this field is immutable after the VM is powered on.
	// To change the guest ID after the VM is powered on, the VM must be powered
	// off and then powered on again with the updated guest ID spec.
	//
	// This field is required when the VM has any CD-ROM devices attached.
	GuestID string `json:"guestID,omitempty"`
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
	//
	// Please note this field is ignored if the VM is deployed from an ISO with
	// CD-ROM devices attached.
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

type VirtualMachineEncryptionType string

const (
	VirtualMachineEncryptionTypeConfig VirtualMachineEncryptionType = "Config"
	VirtualMachineEncryptionTypeDisks  VirtualMachineEncryptionType = "Disks"
)

type VirtualMachineCryptoStatus struct {
	// +optional

	// Encrypted describes the observed state of the VirtualMachine's
	// encryption. There may be two values in this list:
	//
	// - Config -- This refers to all of the files related to a VM except any
	//             virtual disks.
	// - Disks  -- This refers to at least one of the VM's attached disks. To
	//             determine the encryption state of the individual disks,
	//             please refer to status.volumes[].crypto.
	Encrypted []VirtualMachineEncryptionType `json:"encrypted,omitempty"`

	// +optional

	// ProviderID describes the provider ID used to encrypt the VirtualMachine.
	// Please note, this field will be empty if the VirtualMachine is not
	// encrypted.
	ProviderID string `json:"providerID,omitempty"`

	// +optional

	// KeyID describes the key ID used to encrypt the VirtualMachine.
	// Please note, this field will be empty if the VirtualMachine is not
	// encrypted.
	KeyID string `json:"keyID,omitempty"`
}

// VirtualMachineStatus defines the observed state of a VirtualMachine instance.
type VirtualMachineStatus struct {
	// +optional

	// Class is a reference to the VirtualMachineClass resource used to deploy
	// this VM.
	Class *vmopv1a3common.LocalObjectRef `json:"class,omitempty"`

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

	// Crypto describes the observed state of the VirtualMachine's encryption
	// configuration.
	Crypto *VirtualMachineCryptoStatus `json:"crypto,omitempty"`

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
	// +listMapKey=type

	// Volumes describes the observed state of the volumes that are intended to
	// be attached to the VirtualMachine.
	Volumes []VirtualMachineVolumeStatus `json:"volumes,omitempty"`

	// +optional

	// ChangeBlockTracking describes whether or not change block tracking is
	// enabled for the VirtualMachine.
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

	// +optional

	// Storage describes the observed state of the VirtualMachine's storage.
	Storage *VirtualMachineStorageStatus `json:"storage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vm
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
	objectTypes = append(objectTypes, &VirtualMachine{}, &VirtualMachineList{})
}
