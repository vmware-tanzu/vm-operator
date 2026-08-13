// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=Fixed;Regex;Glob
type MatchType string

const (
	// MatchTypeFixed matches the exact value.
	MatchTypeFixed MatchType = "Fixed"

	// MatchTypeRegex matches the value using a regular expression.
	MatchTypeRegex MatchType = "Regex"

	// MatchTypeGlob matches the value using a glob pattern.
	MatchTypeGlob MatchType = "Glob"
)

type VirtualMachineConfigPolicyExtraConfigKey struct {
	// +optional
	// +kubebuilder:default=Glob

	// Type describes the type of match to use.
	// Defaults to Glob.
	Type MatchType `json:"type"`

	// +required

	// Key is the extra config key to match.
	Key string `json:"key"`
}

type VirtualMachineConfigPolicyExtraConfigSpec struct {
	// +optional
	// +listType=map
	// +listMapKey=key

	// Allowed describes the list of allowed extra config keys.
	//
	// If Allowed is non-empty, then a key *must* match one of the values in
	// the list.
	//
	// Denied takes precedent over Allowed.
	Allowed []VirtualMachineConfigPolicyExtraConfigKey `json:"allowed,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=key

	// Denied describes the list of denied extra config keys.
	//
	// If Denied is non-empty, then a key *must not* match one of the values in
	// the list.
	//
	// Denied takes precedent over Allowed.
	Denied []VirtualMachineConfigPolicyExtraConfigKey `json:"denied,omitempty"`
}

// +kubebuilder:validation:Enum=ConfigTarget;Disabled

type VirtualMachineConfigPolicySyncMode string

const (
	// VirtualMachineConfigPolicySyncModeConfigTarget indicates that the policy
	// should reflect the maximum, allowed values from the vSphere cluster's
	// ConfigTarget behind the Zone to which this policy applies.
	VirtualMachineConfigPolicySyncModeConfigTarget VirtualMachineConfigPolicySyncMode = "ConfigTarget"

	// VirtualMachineConfigPolicySyncModeDisabled indicates that the policy
	// should not automatically reflect the information from the associated
	// vSphere cluster.
	VirtualMachineConfigPolicySyncModeDisabled VirtualMachineConfigPolicySyncMode = "Disabled"
)

// +kubebuilder:validation:Enum=Allow;Deny

type VirtualMachineConfigPolicyMode string

const (
	// VirtualMachineConfigPolicyModeAllow indicates that the policy
	// should reflect the maximum, allowed values from the vSphere Cluster's
	// ConfigTarget behind the Zone to which this policy applies.
	VirtualMachineConfigPolicyModeAllow VirtualMachineConfigPolicyMode = "Allow"

	// VirtualMachineConfigPolicyModeDeny indicates that the policy
	// should deny deploying any VM that does not adhere to the Zone's policy.
	VirtualMachineConfigPolicyModeDeny VirtualMachineConfigPolicyMode = "Deny"
)

// +kubebuilder:validation:Enum=AsConfig;AsPolicy

type VirtualMachineConfigPolicyVMClassMode string

const (
	// VirtualMachineConfigPolicyVMClassModeAsConfig indicates that the
	// policy should treat VMs deployed from VM classes no different than VMs
	// that do not use VM classes. In this mode, a VM deployed from a VM class
	// must still be allowed by a zone's policy.
	VirtualMachineConfigPolicyVMClassModeAsConfig VirtualMachineConfigPolicyVMClassMode = "AsConfig"

	// VirtualMachineConfigPolicyVMClassModeAsPolicy indicates that the
	// policy should adhere to vSphere <=9.1 behavior, allowing VMs that
	// reference a VM class to be created regardless of what the policy for a
	// given zone states.
	VirtualMachineConfigPolicyVMClassModeAsPolicy VirtualMachineConfigPolicyVMClassMode = "AsPolicy"
)

// VirtualMachineConfigPolicySpec defines the desired state of a
// VirtualMachineConfigPolicy.
type VirtualMachineConfigPolicySpec struct {
	// +required

	// Zone is the name of the zones.topology.tanzu.vmware.com resource to which
	// this policy applies.
	Zone string `json:"zone"`

	// +optional
	// +kubebuilder:default=ConfigTarget

	// SyncMode describes whether a policy is automatically synchronized with
	// its corresponding vSphere cluster or is managed manually.
	//
	//   * ConfigTarget -- Automatically synchronize the policy using the max
	//                     resources and options from the policy's corresponding
	//                     vSphere cluster's ConfigTarget.
	//   * Disabled     -- This policy is manually managed and should not be
	//                     automatically synced.
	//
	// Defaults to "ConfigTarget".
	SyncMode VirtualMachineConfigPolicySyncMode `json:"syncMode,omitempty"`

	// +optional
	// +kubebuilder:default=Allow

	// CreateMode describes how the policy behaves when creating a new
	// workload resource:
	//
	//   * Allow -- All workloads are allowed.
	//   * Deny  -- All workloads must adhere to the policy.
	//
	// Defaults to "Allow".
	CreateMode VirtualMachineConfigPolicyMode `json:"createMode,omitempty"`

	// +optional
	// +kubebuilder:default=Allow

	// UpdateMode describes how the policy behaves when updating an existing
	// workload resource:
	//
	//   * Allow -- All workloads are allowed.
	//   * Deny  -- All workloads must adhere to the policy.
	//
	// Defaults to "Allow".
	UpdateMode VirtualMachineConfigPolicyMode `json:"updateMode,omitempty"`

	// +optional
	// +kubebuilder:default=Allow

	// PowerOnMode describes how the policy behaves when powering on an existing
	// workload resource:
	//
	//   * Allow -- All workloads are allowed.
	//   * Deny  -- All workloads must adhere to the policy.
	//
	// Defaults to "Allow".
	PowerOnMode VirtualMachineConfigPolicyMode `json:"powerOnMode,omitempty"`

	// +optional
	// +kubebuilder:default=AsPolicy

	// VMClassMode describes how the policy interacts with VM classes:
	//
	//   * AsPolicy -- The policy does not apply to VMs deployed from a VM
	//                 class. This is the default behavior since it was the
	//                 default behavior for vSphere <=9.1.
	//   * AsConfig -- The policy applies to VMs deployed from a VM class as
	//                 well.
	//
	// Defaults to "AsPolicy".
	VMClassMode VirtualMachineConfigPolicyVMClassMode `json:"vmClassMode,omitempty"`

	// +optional

	// NumCPUCores describes the number of CPU cores available to run a
	// virtual machine.
	NumCPUCores *IntRange `json:"numCPUCores,omitempty"`

	// +optional

	// NumNUMANodes describes the total number of NUMA nodes available.
	// A non-zero maximum value indicates NUMA alignment is supported.
	NumNUMANodes *IntRange `json:"numNUMANodes,omitempty"`

	// +optional

	// NumSimultaneousThreads describes the number of simultaneous threads
	// available.
	// A non-zero maximum value indicates HT/SMT is supported.
	NumSimultaneousThreads *IntRange `json:"numSimultaneousThreads,omitempty"`

	// +optional
	// Memory describes the amount of memory available to run a virtual machine.

	Memory *ResourceQuantityRange `json:"memory,omitempty"`

	// +optional

	// SMCPresent describes the presence of the System Management
	// Controller (Apple hardware).
	SMCPresent bool `json:"smcPresent,omitempty"`

	// +optional

	// SEVSupported describes whether AMD SEV is supported.
	SEVSupported bool `json:"sevSupported,omitempty"`

	// +optional

	// SEVSNPSupported describes whether AMD SEV-SNP is supported.
	SEVSNPSupported bool `json:"sevSnpSupported,omitempty"`

	// +optional

	// TDXSupported describes whether Intel TDX is supported.
	TDXSupported bool `json:"tdxSupported,omitempty"`

	// +optional

	// ExtraConfig describes the allowed/denied extra config keys.
	ExtraConfig *VirtualMachineConfigPolicyExtraConfigSpec `json:"extraConfig,omitempty"`

	// +optional

	// LatencySensitivityLevels describes the supported latency sensitivity
	// levels.
	LatencySensitivityLevels []LatencySensitivityLevel `json:"latencySensitivityLevels,omitempty"`

	// +optional

	// CPULockedToMaxSupported describes whether CPU locking to the max
	// is supported.
	CPULockedToMaxSupported bool `json:"cpuLockedToMaxSupported,omitempty"`

	// +optional

	// MemoryLockedToMaxSupported describes whether memory locking to the max
	// is supported.
	MemoryLockedToMaxSupported bool `json:"memoryLockedToMaxSupported,omitempty"`

	// +optional

	// HugePagesSupported describes whether huge pages are supported.
	HugePagesSupported bool `json:"hugePagesSupported,omitempty"`

	// +optional

	// IOMMUSupported describes whether IOMMU is supported.
	IOMMUSupported bool `json:"iommuSupported,omitempty"`

	// +optional

	// RSSSupported describes whether Receive Side Scaling (RSS) is supported.
	// RSS enables RSS on the vNIC, allowing the guest OS to distribute incoming
	// traffic across multiple vCPU cores rather than relying on a single core,
	// which is a major bottleneck.
	RSSSupported bool `json:"rssSupported,omitempty"`

	// +optional

	// UDPRSSSupported describes whether RSS for UDP traffic is supported.
	UDPRSSSupported bool `json:"udpRSSSupported,omitempty"`

	// +optional

	// LargeReceiveOffloadSupported describes whether Large Receive Offload
	// (LRO) is supported.
	LROSupported bool `json:"lroSupported,omitempty"`

	// +optional

	// TxRxThreadModels describes the supported transmit/receive models.
	TxRxThreadModels []TxRxThreadModel `json:"txRxThreadModels,omitempty"`

	// HardwareVersions describes the range of supported hardware vesions.
	HardwareVersions *HardwareVersionRange `json:"hardwareVersions,omitempty"`

	ConfigTargetDevices `json:",inline"`
}

// VirtualMachineConfigPolicyStatus defines the observed state of a
// VirtualMachineConfigPolicy.
type VirtualMachineConfigPolicyStatus struct {
	// +optional

	// ObservedGeneration describes the observed state of the
	// metadata.generation field at the time this object was last
	// reconciled by its primary controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional

	// Conditions describes the observed state of any conditions
	// associated with this object.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmconfigpolicy
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status

// VirtualMachineConfigPolicy is the schema for the VirtualMachineConfigPolicy API.
type VirtualMachineConfigPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec describes the desired state of the VirtualMachineConfigPolicy.
	Spec VirtualMachineConfigPolicySpec `json:"spec,omitempty"`

	// Status describes the observed state of the VirtualMachineConfigPolicy.
	Status VirtualMachineConfigPolicyStatus `json:"status,omitempty"`
}

// GetConditions returns the status conditions for the VirtualMachineConfigPolicy.
func (p VirtualMachineConfigPolicy) GetConditions() []metav1.Condition {
	return p.Status.Conditions
}

// SetConditions sets the status conditions for the VirtualMachineConfigPolicy.
func (p *VirtualMachineConfigPolicy) SetConditions(conditions []metav1.Condition) {
	p.Status.Conditions = conditions
}

// GetConditions returns the conditions for the VirtualMachineConfigPolicyStatus.
func (p VirtualMachineConfigPolicyStatus) GetConditions() []metav1.Condition {
	return p.Conditions
}

// SetConditions sets the conditions for the VirtualMachineConfigPolicyStatus.
func (p *VirtualMachineConfigPolicyStatus) SetConditions(conditions []metav1.Condition) {
	p.Conditions = conditions
}

// +kubebuilder:object:root=true

// VirtualMachineConfigPolicyList contains a list of VirtualMachineConfigPolicy
// objects.
type VirtualMachineConfigPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineConfigPolicy `json:"items"`
}

func init() {
	objectTypes = append(
		objectTypes,
		&VirtualMachineConfigPolicy{},
		&VirtualMachineConfigPolicyList{})
}
