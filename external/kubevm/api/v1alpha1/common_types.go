// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// +kubebuilder:validation:Enum=PoweredOn;PoweredOff;Suspended

// PowerState describes a virtual machine's power state.
//
// The values are spelled "PoweredOn" and "PoweredOff" rather than "On" and
// "Off" for two reasons. First, "On", "Off", "Yes", and "No" are boolean
// literals in YAML 1.1, which kubectl follows, so an unquoted
// `powerState: On` parses as the boolean true and is rejected by the API
// server. Second, these spellings match VM Operator's existing
// VirtualMachinePowerState, so the vSphere provider needs no value mapping.
//
// Suspended is a capability-gated value: vSphere and GCP can suspend, but EC2
// only hibernates, and only for hibernation-enabled instances. A provider that
// cannot honor Suspended must reject it at admission rather than silently
// treating it as PoweredOff.
type PowerState string

const (
	// PowerStateOn indicates the VM should be powered on.
	PowerStateOn PowerState = "PoweredOn"

	// PowerStateOff indicates the VM should be powered off.
	PowerStateOff PowerState = "PoweredOff"

	// PowerStateSuspended indicates the VM should be suspended.
	PowerStateSuspended PowerState = "Suspended"
)

// +kubebuilder:validation:Enum=Hard;Soft;TrySoft

// PowerOpMode describes how a power-off or suspend operation is performed.
type PowerOpMode string

const (
	// PowerOpModeHard powers off the VM without involving the guest.
	PowerOpModeHard PowerOpMode = "Hard"

	// PowerOpModeSoft asks the guest to shut down and fails if it does not.
	PowerOpModeSoft PowerOpMode = "Soft"

	// PowerOpModeTrySoft attempts a guest shutdown, then falls back to hard.
	PowerOpModeTrySoft PowerOpMode = "TrySoft"
)

// +kubebuilder:validation:Enum=Pending;Provisioning;Running;Stopping;Stopped;Suspended;Failed;Deleting

// VirtualMachinePhase is a portable, human-readable lifecycle summary derived
// from the provider's reported instance state.
type VirtualMachinePhase string

const (
	VirtualMachinePhasePending      VirtualMachinePhase = "Pending"
	VirtualMachinePhaseProvisioning VirtualMachinePhase = "Provisioning"
	VirtualMachinePhaseRunning      VirtualMachinePhase = "Running"
	VirtualMachinePhaseStopping     VirtualMachinePhase = "Stopping"
	VirtualMachinePhaseStopped      VirtualMachinePhase = "Stopped"
	VirtualMachinePhaseSuspended    VirtualMachinePhase = "Suspended"
	VirtualMachinePhaseFailed       VirtualMachinePhase = "Failed"
	VirtualMachinePhaseDeleting     VirtualMachinePhase = "Deleting"
)

// Condition types reported on a VirtualMachine.
//
// Conditions are where provider-dependent outcomes are reported, and this is a
// deliberate split from admission-time validation.
//
// A field is rejected at admission only when it can never be valid — a
// malformed reference, or a change to an immutable field. Anything whose
// validity depends on the provider, the platform's current capacity, or the
// machine's current state is accepted and then reported here. Two reasons:
//
// The generic API server cannot evaluate provider-dependent validity. The
// object being admitted is a portable VirtualMachine, and its webhook would
// have to know which provider will service it — which would mean importing
// provider code, the one thing this API is built to avoid.
//
// And validating desired state against observed state is the wrong idiom. If a
// field could only be changed while the machine is off, rejecting the edit
// would mean comparing spec against status.powerState, which lags: a user
// powers off, status has not caught up, and the edit is refused for a reason
// that is no longer true. Accepting the edit and reconciling when the machine
// reaches the required state is both correct and the established Kubernetes
// pattern — the same reason a Cluster API cluster can sit provisioning for a
// long time without anything being wrong.
const (
	// VirtualMachineConditionReady summarizes whether the machine is running
	// and usable.
	VirtualMachineConditionReady = "Ready"

	// VirtualMachineConditionInfrastructureReady reports whether the provider
	// object has been adopted and is reporting a healthy backend machine.
	VirtualMachineConditionInfrastructureReady = "InfrastructureReady"

	// VirtualMachineConditionUpToDate reports whether the backend machine
	// matches this object's spec.
	//
	// This is the condition that carries accepted-but-not-yet-applied changes.
	// False with reason RequiresPowerOff means the edit is valid and pending,
	// not refused.
	VirtualMachineConditionUpToDate = "UpToDate"
)

// Condition reasons. These name why a condition is not yet true, and are the
// vocabulary a provider uses instead of failing a request outright.
const (
	// VirtualMachineReasonRequiresPowerOff indicates a spec change was
	// accepted but cannot be applied while the machine is running. The
	// provider applies it the next time the machine is powered off.
	//
	// Several fields are in this category on at least one platform: instance
	// type and bootstrap data can only be changed on a stopped EC2 instance,
	// for example.
	VirtualMachineReasonRequiresPowerOff = "RequiresPowerOff"

	// VirtualMachineReasonUnsupportedByProvider indicates the provider cannot
	// honor a field this API defines.
	//
	// This is the honest reporting path for capability gaps, and it is why
	// such fields are not rejected at admission. A provider that cannot
	// suspend, or cannot supply an external address, says so here rather than
	// silently ignoring the request or failing the write.
	VirtualMachineReasonUnsupportedByProvider = "UnsupportedByProvider"

	// VirtualMachineReasonWaitingForCapacity indicates the platform has no
	// capacity for the request right now and the provider will retry.
	//
	// Explicitly not terminal. EC2's InsufficientInstanceCapacity and GCE's
	// ZONE_RESOURCE_POOL_EXHAUSTED both succeed on retry, sometimes only in a
	// different zone or with a different instance type.
	VirtualMachineReasonWaitingForCapacity = "WaitingForCapacity"

	// VirtualMachineReasonRateLimited indicates the platform is throttling the
	// operation, or enforcing a cooldown between successive changes.
	//
	// EBS volume modification is the clearest case: a second resize of the
	// same volume is refused for a period measured in hours, so an accepted
	// size change can legitimately remain unapplied for a long time.
	VirtualMachineReasonRateLimited = "RateLimited"

	// VirtualMachineReasonPreempted indicates the backend machine was
	// reclaimed by the platform because it was running on spot or preemptible
	// capacity.
	//
	// A normal outcome for such machines rather than a fault. Note that on
	// some platforms reclamation destroys the machine and its disks, so a
	// subsequent reconcile may produce a different backend instance than the
	// one originally created.
	VirtualMachineReasonPreempted = "Preempted"

	// VirtualMachineReasonProviderError indicates the provider failed for a
	// reason that does not fall into the categories above. The condition
	// message carries the platform's own error.
	VirtualMachineReasonProviderError = "ProviderError"
)

// ObjectReference references another object in the same namespace, by group,
// kind, and name. It is the single reference shape used everywhere in this API:
// for the infrastructure object, the boot image, a snapshot, and a network.
//
// Two deliberate omissions.
//
// There is no namespace field. A referent is always in the same namespace as
// the referring VirtualMachine, so a VirtualMachine cannot reach across a
// namespace boundary to adopt an object or read a catalog entry it was not
// granted.
//
// There is no version field. Naming a version here would pin it into every
// referring object, so a provider could not roll its API forward without
// rewriting every VirtualMachine that points at it. Instead the group and kind
// identify the resource and the served version is resolved at runtime — for the
// infrastructure object, from the contract version the provider's CRD
// advertises via a well-known label. This mirrors Cluster API's
// ContractVersionedObjectReference, which made the same move away from a
// versioned reference for the same reason.
//
// Group and kind are required rather than defaulted. Defaulting them would bake
// in an assumption about which group owns a referent — but whether images and
// networks are portable types or provider-owned ones is deliberately still open
// (see the repository README), and a provider must be free to point these at
// its own types. Being explicit costs three lines of YAML and buys that
// freedom.
type ObjectReference struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +required

	// APIGroup of the referent, as a fully qualified domain name — for example
	// "kube-vm.io" or "infrastructure.vsphere.kube-vm.io".
	APIGroup string `json:"apiGroup"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-zA-Z]([-a-zA-Z0-9]*[a-zA-Z0-9])?$`
	// +required

	// Kind of the referent — for example "VSphereVirtualMachine" or
	// "VirtualMachineImage".
	Kind string `json:"kind"`

	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`
	// +required

	// Name of the referent.
	Name string `json:"name"`
}

// SecretKeySelector selects a single key from a Secret in the same namespace.
type SecretKeySelector struct {
	// +kubebuilder:validation:MinLength=1
	// +required

	// Name of the Secret.
	Name string `json:"name"`

	// +kubebuilder:validation:MinLength=1
	// +required

	// Key within the Secret's data.
	Key string `json:"key"`
}

// +kubebuilder:validation:Enum=InternalIP;ExternalIP;InternalDNS;ExternalDNS

// VirtualMachineAddressType describes the reachability scope of an address.
type VirtualMachineAddressType string

const (
	VirtualMachineAddressInternalIP  VirtualMachineAddressType = "InternalIP"
	VirtualMachineAddressExternalIP  VirtualMachineAddressType = "ExternalIP"
	VirtualMachineAddressInternalDNS VirtualMachineAddressType = "InternalDNS"
	VirtualMachineAddressExternalDNS VirtualMachineAddressType = "ExternalDNS"
)

// VirtualMachineAddress is an observed network address for a virtual machine.
type VirtualMachineAddress struct {
	// +required

	// Type describes the reachability scope of this address.
	Type VirtualMachineAddressType `json:"type"`

	// +kubebuilder:validation:MinLength=1
	// +required

	// Address is the IP address or DNS name.
	Address string `json:"address"`
}
