// Copyright (c) 2025 Broadcom. All Rights Reserved.

package v1alpha2

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImportOperationSpecValid and ImportOperationPrecheckSucceeded Condition.Type and their corresponding Condition.Reasons.
const (
	// ImportOperationSpecValid is set to true only when ImportOperation.Spec has been validated.
	ImportOperationSpecValid = "SpecValid"

	// ImportOperationPrecheckSucceeded is set to true only when ImportOperation.Spec has been validated.
	ImportOperationPrecheckSucceeded = "PrecheckSucceeded"

	// ImportOperationVMwareToolsMissing indicates that the import operation failed because the virtual machine does not have VMware Tools installed.
	ImportOperationVMwareToolsMissing = "VMwareToolsMissing"

	// ImportOperationUnsupportedGuestOS indicates that the import operation failed because the virtual machine has a guest OS that does not support Guest OS customization.
	ImportOperationUnsupportedGuestOS = "UnsupportedGuestOS"

	// ImportOperationIPv6NotSupported indicates that the import operation failed because the virtual machine has an IPv6 network, which is not supported.
	ImportOperationIPv6NotSupported = "IPv6NotSupported"

	// ImportOperationVirtualMachineManagedByFieldAlreadySet indicates that the import operation failed because the virtual machine is already managed by a VC Extension.
	ImportOperationVirtualMachineManagedByFieldAlreadySet = "ManagedByAlreadySet"

	// ImportOperationVirtualMachineTemplateNotSupported indicates that the import operation failed because the virtual machine is a template.
	ImportOperationVirtualMachineTemplateNotSupported = "VirtualMachineTemplateNotSupported"

	// ImportOperationVirtualMachineAlreadyExists indicates a virtual machine already exists at the target folder.

	ImportOperationVirtualMachineAlreadyExists = "VirtualMachineAlreadyExists"

	// ImportOperationVAppNotSupported indicates the VM is part of a vApp, which is not supported.
	ImportOperationVAppNotSupported = "VAppNotSupported"

	// ImportOperationImprovedVirtualDisksNotSupported indicates a virtual machine contains Improved Virtual Disks.
	ImportOperationImprovedVirtualDisksNotSupported = "ImprovedVirtualDisksNotSupported"

	// ImportOperationSuspendedVirtualMachineNotSupported  indicates that Virtual Machine is in Suspended state.
	ImportOperationSuspendedVirtualMachineNotSupported = "SuspendedVirtualMachineNotSupported"
)

// ImportOperationVirtualMachineSetManagedBySucceeded Condition.Type and its corresponding Condition.Reasons.
const (
	// ImportOperationVirtualMachineSetManagedBySucceeded is set to true only when the virtual machine has been successfully set managed by VM Service.
	ImportOperationVirtualMachineSetManagedBySucceeded = "VirtualMachineSetManagedBySucceeded"

	// ImportOperationVirtualMachineSetManagedByFailure indicates that there was a failure to set the virtual machine as managed by VM Service.
	ImportOperationVirtualMachineSetManagedByFailure = "VirtualMachineSetManagedByFailure"
)

// ImportOperationNetworkBackingReady Condition.Type and its corresponding Condition.Reasons.
const (
	// ImportOperationNetworkBackingReady is set to true only when the network backing is ready for use by the incoming virtual machine.
	ImportOperationNetworkBackingReady = "NetworkBackingReady"

	// ImportOperationNetworkCreationFailure indicates that there was a failure to create the target network.
	ImportOperationNetworkCreationFailure = "NetworkCreationFailure"

	// ImportOperationNetworkNotReady indicates that the target network is not ready for use.
	ImportOperationNetworkNotReady = "NetworkNotReady"

	// ImportOperationNetworkBackingMissing indicates that the target network is marked as ready to be used, but its network device backing cannot be retrieved.
	ImportOperationNetworkBackingMissing = "NetworkBackingMissing"
)

// ImportOperationVirtualMachineReadyForImport Condition.Type and its corresponding Condition.Reasons.
const (
	// ImportOperationVirtualMachineReadyForImport is set to true only when the virtual machine has been successfully moved to a suitable infrastructure servicing the target namespace and is ready to be imported.
	ImportOperationVirtualMachineReadyForImport = "VirtualMachineReadyForImport"

	// ImportOperationVirtualMachinePlacementFailure indicates that there was a failure to recommend the virtual machine placement.
	ImportOperationVirtualMachinePlacementFailure = "VirtualMachinePlacementFailure"

	// ImportOperationVirtualMachineReconfigureFailure indicates that there was a failure to reconfigure the virtual machine.
	ImportOperationVirtualMachineReconfigureFailure = "VirtualMachineReconfigureFailure"

	// ImportOperationVirtualMachineRelocateFailure indicates that there was a failure to relocate the virtual machine.
	ImportOperationVirtualMachineRelocateFailure = "VirtualMachineRelocateFailure"

	// ImportOperationVirtualMachineResetPermissionFailure indicates that there was a failure to reset the virtual machine permissions.
	ImportOperationVirtualMachineResetPermissionFailure = "VirtualMachineResetPermissionFailure"
)

// ImportOperationGuestCustomization Condition.Type and its corresponding Condition.Reason
const (
	// ImportOperationGuestCustomization is set to true when the virtual machine is customized successfully.
	ImportOperationGuestCustomization = "GuestCustomization"

	// ImportOperationGuestCustomizationVMCrNotFound indicates that the virtual machine resource was not found during guest customization.
	ImportOperationGuestCustomizationVMCrNotFound = "GuestCustomizationVMCrNotFound"

	// ImportOperationGuestCustomizationFailed indicates that the guest customization failed in the guest OS.
	ImportOperationGuestCustomizationFailed = "GuestCustomizationFailed"
)

// ImportOperation Condition.Types without any corresponding Condition.Reasons.
const (
	// ImportOperationVirtualMachineReady is set to true when the virtual machine is ready to be consumed in the target namespace.
	ImportOperationVirtualMachineReady = "VirtualMachineReady"

	// ImportOperationCompleted is set to true when all other conditions for the import operation have been set to true.
	ImportOperationCompleted = "Completed"
)

// ImportOperationVirtualMachineCreated Condition.Type and its corresponding Condition.Reason
const (
	// ImportOperationVirtualMachineCreated is set to true when the virtual machine has been successfully created in the target namespace.
	ImportOperationVirtualMachineCreated = "VirtualMachineCreated"

	// ImportOperationVMCRNotFoundPostCreation indicate a failure when the VM CR was created but cannot be found post creation.
	ImportOperationVMCRNotFoundPostCreation = "VMCRNotFoundPostCreation"

	// ImportOperationCreateVMCRFailure indicates that there was a failure to create VM CR.
	ImportOperationCreateVMCRFailure = "CreateVMCRFailure"

	// ImportOperationBootstrapSpecCreationFailure indicates that there was a failure to create the bootstrap spec for the VM CR.
	ImportOperationBootstrapSpecCreationFailure = "BootstrapSpecCreationFailure"

	// ImportOperationVirtualMachineCreationFailureNotFound indicates that the virtual machine resource was not found during creating VM CR.
	ImportOperationVirtualMachineNotFound = "VirtualMachineNotFound"

	// ImportOperationGetVMCRFailure indicates that there was a failure to get VM CR. The real reason is unknown.
	ImportOperationGetVMCRFailure = "GetVMCRFailure"
)

// Condition.Type for Conditions related to rollback in ImportOperation.Status.
const (
	// RollbackVirtualMachineLocationCompleted is set to true only when the VM or VMs have been successfully rolled back
	// to the original location.
	RollbackVirtualMachineLocationCompleted = "RollbackVirtualMachineLocationCompleted"

	// RollbackVirtualMachinePropertyCompleted is set to true only when the VM or VMs have been successfully rolled back
	// to the original property.
	RollbackVirtualMachinePropertyCompleted = "RollbackVirtualMachinePropertyCompleted"

	// RollbackCustomResourceCompleted is set to true only when the custom resource created by the ImportOperation have been
	// successfully cleaned up.
	RollbackCustomResourceCompleted = "RollbackCustomResourceCompleted"

	// RollbackCompleted is set to true only when all other conditions present in the rollback have been set to true.
	RollbackCompleted = "RollbackCompleted"
)

// Condition.Reason for Condition.Type RollbackVirtualMachineLocationCompleted related to rollback in ImportOperation.Status.
const (
	// RollbackVirtualMachineCancelImportTaskFailure means there is a failure to cancel virtual machine import task during rollback.
	RollbackVirtualMachineCancelImportTaskFailure = "RollbackVirtualMachineCancelImportTaskFailure"

	// RollbackVirtualMachineRelocateFailure means there is a failure to relocate virtual machine during rollback.
	RollbackVirtualMachineRelocateFailure = "RollbackVirtualMachineRelocateFailure"

	// RollbackVirtualMachineReconfigureFailure means there is a failure to reconfigure virtual machine during rollback.
	RollbackVirtualMachineReconfigureFailure = "RollbackVirtualMachineReconfigureFailure"
)

// Condition.Reason for Condition.Type RollbackVirtualMachinePropertyCompleted related to rollback in ImportOperation.Status.
const (
	// RollbackVirtualMachinePermissionFailure means there is a failure to restore virtual machine permission during rollback.
	RollbackVirtualMachinePermissionFailure = "RollbackVirtualMachinePermissionFailure"

	// RollbackVirtualMachineManagedByFailure means there is a failure to restore virtual machine managed by field during rollback.
	RollbackVirtualMachineManagedByFailure = "RollbackVirtualMachineManagedByFailure"
)

// Condition.Reason for Condition.Type RollbackCustomResourceCompleted related to rollback in ImportOperation.Status.
const (
	// RollbackSecretCleanupFailure means there is a failure to cleanup secret during rollback.
	RollbackSecretCleanupFailure = "RollbackSecretCleanupFailure"

	// RollbackNetworkBackingCleanupFailure means there is a failure to cleanup network backing during rollback.
	RollbackNetworkBackingCleanupFailure = "RollbackNetworkBackingCleanupFailure"
)

// ProductIDSecretKeySelector references the ProductID value from a Secret resource.
type ProductIDSecretKeySelector struct {
	// Name is the name of the secret.
	Name string `json:"name"`

	// +kubebuilder:default=product_id

	// Key is the key in the secret that specifies the requested data.
	Key string `json:"key"`
}

// CustomizationSysprepUserData contains user data for customizing a Windows guest operating system.
// This struct maps to the UserData key in the sysprep.xml answer file.
type CustomizationSysprepUserData struct {
	// FullName is the full name of the user.
	FullName string `json:"fullName"`

	// OrgName is the name of the organization.
	OrgName string `json:"orgName"`

	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9\p{S}\p{L}]{1,2}|(?:[a-zA-Z0-9\p{S}\p{L}][a-zA-Z0-9-\p{S}\p{L}]{0,13}[a-zA-Z0-9\p{S}\p{L}]))$`

	// ComputerName is the computer name of the Windows virtual machine. It must be 1 to 15 characters in length.
	//
	// Validation Rules:
	// - If the name is 1 or 2 characters long, it can consist of letters (A–Z, a–z), numbers (0–9), or symbols.
	// - If the name is between 3 and 15 characters:
	//   - It must start and end with a letter, number, or symbol (excluding hyphens).
	//   - Middle characters (if any) can be letters, numbers, symbols, or hyphens (-).
	//   - Hyphens are **not** allowed at the beginning or end of the name.
	//
	// - The name must not contain spaces or periods (.).
	//
	// Examples of valid ComputerName values:
	// - "A"
	// - "AB"
	// - "Server1"
	// - "Workstation-01"
	// - "WEB_SERVER"
	// - "Comp-Name"
	//
	// Examples of invalid ComputerName values:
	// - "-"              (single hyphen, not allowed)
	// - "Server-"        (ends with hyphen)
	// - "-Server"        (starts with hyphen)
	// - "Server Name"    (contains spaces)
	// - "Server.Name"    (contains periods)
	// - "ThisNameIsWayTooLongForTheLimit" (exceeds 15 characters)
	ComputerName string `json:"computerName"`

	// +optional

	// ProductID is a valid serial number. Microsoft Sysprep requires that a valid serial number be included in the answer file when mini-setup runs. This serial number is ignored if the original guest operating system was installed using a volume-licensed CD.
	ProductID *ProductIDSecretKeySelector `json:"productID,omitempty"`
}

// NetworkCustomization is used to configure the network identity of the importing virtual machine.
//
// This is required when importing a Windows virtual machine that needs automated network identity configuration via Guest OS customization.
type NetworkCustomization struct {
	// +optional

	// CustomizationSysprepUserData contains the user data for sysprep customization.
	CustomizationSysprepUserData *CustomizationSysprepUserData `json:"customizationSysprepUserData,omitempty"`
}

// +kubebuilder:validation:Enum=Subnet;SubnetSet

// SubnetType defines the type of Subnet
type SubnetType string

const (
	// SubnetTypeSubnet in a VPC represents an independent layer 2 broadcast domain with its associated CIDR and properties like Access mode (network advertisement), DHCP configuration etc.
	SubnetTypeSubnet SubnetType = "Subnet"

	// SubnetTypeSubnetSet is a scalable grouping of VPC subnets sharing the same properties, which will allow auto-scale of networking availability to connect workloads.
	SubnetTypeSubnetSet SubnetType = "SubnetSet"
)

// SubnetInfo defines the information identifying a Subnet.
type SubnetInfo struct {
	// Name corresponds to the name of the Subnet.
	Name string `json:"name"`

	// +kubebuilder:validation:Minimum=0

	// DeviceKey corresponds to the device key of the virtual device of the VM.
	DeviceKey int32 `json:"deviceKey"`

	// +optional

	// Type of the Subnet, indicating whether it is a Subnet or SubnetSet.
	// If the name is unique in the available Subnet and SubnetSet entities,
	// this field is optional.
	Type SubnetType `json:"type,omitempty"`
}

// CommitActionType defines the type of commit action for an import operation.
type CommitActionType string

const (
	// CommitActionWait implies that the import operation will finish its operations in order to
	// be ready to be in the supervisor, it will wait for a user to commit.
	CommitActionWait CommitActionType = "Wait"

	// CommitActionAuto implies that the import operation will finish its operations to be used
	// in Supervisor, it will be automatically committed.
	CommitActionAuto CommitActionType = "Auto"
)

// RollbackActionType defines the type of rollback action for an import operation.
type RollbackActionType string

const (
	// RollbackActionImmediate implies that the import operation will perform a best effort to rollback
	// the workload to its original state prior to the import operation request.
	RollbackActionImmediate RollbackActionType = "Immediate"

	// RollbackActionComplete implies that the import operation will complete the rollback immediately,
	// any ongoing rollback will be given up and be stopped forever.
	RollbackActionComplete RollbackActionType = "Complete"
)

// ControlActionSpec defines the desired control actions for an import operation.
type ControlActionSpec struct {
	// +optional
	// +kubebuilder:default=CommitActionWait

	// CommitAction allows the user to decide what commit action to take for an import operation.

	// The default commit action will be CommitActionWait.
	CommitAction CommitActionType `json:"commitAction,omitempty"`

	// +optional

	// RollbackAction allows the user to decide what rollback action to take for an import operation.

	// If RollbackAction is unset, no rollback action will be taken.
	// The default rollback action will be unset.
	RollbackAction RollbackActionType `json:"rollbackAction,omitempty"`

	// +optional
	// +kubebuilder:default=false

	// PrecheckOnly indicates whether the import operation should perform only the precheck phase.
	//
	// When PrecheckOnly is set to true, the import operation will run prechecks to detect as many potential blockers
	// as possible without altering any infrastructure state, then pause without proceeding further in the workflow.
	// When set to false, the import operation continues with the rest of the workflow after prechecks.
	//
	// PrecheckOnly may be updated from true to false to resume the workflow, but cannot be updated from false to true,
	// as it would be semantically unclear to perform only a precheck after other workflow steps have already executed.
	PrecheckOnly *bool `json:"precheckOnly,omitempty"`
}

// ImportOperationSpec defines the desired state of import operation.
type ImportOperationSpec struct {
	// +kubebuilder:validation:Required

	// VirtualMachineID is the unique identifier of the virtual machine in vSphere to be imported.
	VirtualMachineID string `json:"virtualMachineID"`

	// +optional

	// StorageClass specifies the name of the StorageClass resource used to configure storage-related attributes of the importing virtual machine. If unset, and if the target namespace has only a single StorageClass for VM service, that StorageClass will be used by default.
	//
	// Note: The name of the StorageClass is derived from the vSphere Storage Policy name and conforms to Kubernetes RFC 1123 Label Names. The StorageClass resource in the target namespace is allocated by the vSphere Storage Policy added to the namespace and is subject to StoragePolicyQuota restrictions.
	StorageClass string `json:"storageClass,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=deviceKey

	// List of network device keys to Subnet information specifying the Subnets
	// to which the VM's network devices should be connected.
	//
	// The DeviceKey is the device key of the network device on the
	// VM, and the value is the Subnet information. Each device key must be
	// unique within the list; overlapping or duplicate device keys are not
	// allowed.
	SubnetMappings []SubnetInfo `json:"subnetMappings,omitempty"`

	// +optional

	// NetworkCustomization is used to configure the network identity of the importing virtual machine by assigning one from the Supervisor Workload Network via Guest OS customization.
	NetworkCustomization *NetworkCustomization `json:"networkCustomization,omitempty"`

	// +optional

	// +kubebuilder:validation:Minimum=0

	// TTLSecondsAfterFinished limits the lifetime of an import operation after completion (either Succeeded or Failed). If set, the import operation is eligible for automatic deletion TTLSecondsAfterFinished seconds after it finishes. When being deleted, its lifecycle guarantees (e.g., finalizers) will be honored. If unset, the import operation will not be automatically deleted. If set to zero, the import operation becomes eligible for immediate deletion upon completion.
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// +optional

	// ControlAction specifies the desired control action for an import operation.

	// If unset, the import operation will follow the same workflow as the v1alpha1 behavior:
	// It will proceed with completing the import into Supervisor without pausing for pre-check or commit.
	// If the import operation is deleted before ImportOperationVirtualMachineCreated is set to true,
	// the system will attempt a best-effort rollback to restore the workload to its original state.
	// When ControlAction is set, it implies that the import operation will take action specified by ControlAction.

	// Once ControlAction is unset, it can not be updated.
	// The default control action will be unset, to keep backward compatibility, v1alpha1 API does not support control action.
	ControlAction *ControlActionSpec `json:"controlAction,omitempty"`
}

// +kubebuilder:validation:Enum=ImportRelocate;RollbackRelocate

// TaskType defines the type for monitored task types.
type TaskType string

const (
	// TaskImportRelocate tracks the relocate task type for importing the virtual machine into the target namespace.
	TaskImportRelocate TaskType = "ImportRelocate"

	// TaskRollbackRelocate tracks the relocate task type for rolling back the virtual machine to its original infrastructure.
	TaskRollbackRelocate TaskType = "RollbackRelocate"
)

// TaskMonitoringInfo provides detailed tracking information about a task's execution and progress over time. It maintains key data points such as the task's unique identifier, the intervals at which the task's status is polled, the last time the status was checked, and the task's progress percentage.
type TaskMonitoringInfo struct {
	// Type specifies the type of the task being monitored.
	Type TaskType `json:"type"`

	// TaskID is the unique identifier of the task in vSphere.
	TaskID string `json:"taskID"`

	// PollIntervalSeconds defines the interval, in seconds, between consecutive status retrieval attempts.
	// The controller starts with an initial interval of 1 second. If the `LastProgress` value has not changed since the last poll, the controller increases the interval exponentially.
	PollIntervalSeconds int32 `json:"pollIntervalSeconds"`

	// LastPollTime is the timestamp of the most recent status retrieval for the task.
	LastPollTime metav1.Time `json:"lastPollTime"`

	// LastProgress represents the most recent progress percentage of the task, recorded during the last status poll.
	LastProgress int32 `json:"lastProgress"`
}

// ImportOperationStatus defines the observed state of import operation.
type ImportOperationStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=type

	// Conditions describes the current condition information of the import operation object.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional

	// StartTime describes the time when the import operation controller started processing the import operation. It is represented in RFC3339 format and is in UTC.
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// +optional

	// CompletionTime describes the time when the import operation was completed. It is not guaranteed to be set in a happens-before order across separate ImportOperations. It is represented in RFC3339 format and is in UTC.
	//
	// The value of this field should be equal to the value of the LastTransitionTime for the status condition `Type=ImportOperationCompleted`.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// +optional

	// VirtualMachineName is the name of the imported virtual machine in the target namespace.
	VirtualMachineName string `json:"virtualMachineName,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type

	// TaskMonitor tracks the status of tasks responsible for moving the imported virtual machine to the target infrastructure.
	// Each entry corresponds to a specific task type, and only the most recent task status for each type is retained.
	TaskMonitor []TaskMonitoringInfo `json:"taskMonitor,omitempty"`

	// +optional

	// StateTransitions records the latest virtual machine state change or action for each `TransitionType` performed during the import process. Each `StateTransition` entry corresponds to a specific `TransitionType`, and only the most recent transition for each type is retained.
	StateTransitions []StateTransition `json:"stateTransitions,omitempty"`
}

// GetConditions returns the list of conditions for the import operation.
func (importOp *ImportOperation) GetConditions() []metav1.Condition {
	return importOp.Status.Conditions
}

// SetConditions updates the conditions for the import operation.
func (importOp *ImportOperation) SetConditions(conditions []metav1.Condition) {
	importOp.Status.Conditions = conditions
}

// GetStateTransitions returns the state transitions for this import operation.
func (importOp *ImportOperation) GetStateTransitions() []StateTransition {
	return importOp.Status.StateTransitions
}

// SetStateTransitions sets the state transitions for this import operation.
func (importOp *ImportOperation) SetStateTransitions(transitions []StateTransition) {
	importOp.Status.StateTransitions = transitions
}

// IsDeleteTimeBeforeNow returns true if current time exceeds TTLSecondsAfterFinished + completionTime.
func (importOp *ImportOperation) IsDeleteTimeBeforeNow() bool {
	if importOp.Spec.TTLSecondsAfterFinished == nil {
		return false
	}

	duration := time.Duration(*importOp.Spec.TTLSecondsAfterFinished) * time.Second
	deletionTime := importOp.Status.CompletionTime.Add(duration)
	return deletionTime.Before(time.Now())
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=importop
// +kubebuilder:printcolumn:name="VM Name",type=string,JSONPath=`.status.virtualMachineName`
// +kubebuilder:printcolumn:name="Completed",type=string,JSONPath=`.status.conditions[?(@.type=="Completed")].status`

// ImportOperation represents an import operation for a virtual machine in the API.
type ImportOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImportOperationSpec   `json:"spec,omitempty"`
	Status ImportOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImportOperationList contains a list of ImportOperation resources.
type ImportOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImportOperation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImportOperation{}, &ImportOperationList{})
}
