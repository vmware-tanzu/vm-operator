// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Finalizer holds an AWSMachine until its EC2 instance is terminated.
//
// Without it the instance leaks: the core deletes the provider object and
// waits, so nothing else stands between a deleted VirtualMachine and an
// instance that goes on billing with nothing in the cluster naming it.
const Finalizer = "infrastructure.kube-vm.io/awsmachine"

// AWSMachineSpec is the EC2-specific half of a machine.
//
// A person submits this EMPTY. Every field below is written by the controller,
// resolved from the portable VirtualMachine that owns this object.
//
// That the controller writes spec at all is a deliberate exception to the rule
// that controllers write only status, and the provider contract asks for it in
// terms: a reader running `kubectl get -o yaml` has to see what the machine
// will actually do, because other controllers read this object and GitOps
// diffing, backup, restore and audit all assume the spec is truthful.
//
// Duplication across the seam is therefore expected HERE and nowhere else.
// These are delegated facts, authored on the portable object and reflected
// down; the user never writes them and the portable object always wins.
type AWSMachineSpec struct {
	// +kubebuilder:validation:Pattern=`^ami-[0-9a-f]{8,17}$`
	// +optional

	// ImageID is the AMI this machine booted from, resolved from the portable
	// object's boot disk image reference.
	//
	// Cannot change once status.launchRequested is set. Until then the
	// controller keeps it in step with the portable object, so a value
	// written here by hand is replaced on the next reconcile.
	ImageID string `json:"imageID,omitempty"`

	// +optional

	// InstanceType is the EC2 instance type, resolved from the portable
	// object's instance type name.
	//
	// Cannot change once status.launchRequested is set. EC2 can resize a
	// stopped instance, but this provider does not implement that, so the
	// type an instance launched with is the type it keeps. A later change on
	// the portable object is reported through UpToDate rather than written
	// here -- writing it would make the spec name a size the machine is not
	// running.
	InstanceType string `json:"instanceType,omitempty"`

	// +kubebuilder:validation:Pattern=`^subnet-[0-9a-f]{8,17}$`
	// +optional

	// Subnet is where the instance was placed, resolved from the portable
	// object's first network interface when it names one.
	//
	// Empty means the portable object named no network and EC2 selected from
	// the account's default VPC. Cannot change once status.launchRequested
	// is set: a subnet pins the availability zone, and moving a running
	// instance between zones is not an edit.
	Subnet string `json:"subnet,omitempty"`

	// +optional

	// PublicIP records whether the instance should have an externally
	// reachable address, copied from the portable object's first network
	// interface on every reconcile.
	//
	// Nil means the portable object expressed no preference, which leaves the
	// subnet's own MapPublicIpOnLaunch setting in charge -- and in a default
	// VPC that means the instance gets one. So nil is not "no public address",
	// it is "we did not say", and the two differ.
	//
	// Mutable, like power state: EC2 can add or remove the public address on
	// a running instance's primary network interface, so a change on the
	// portable object is applied, and a hand-edit here reverts. Whether a
	// machine is reachable from the internet is the one fact here with a
	// security consequence, so it is recorded on the object rather than left
	// readable only on the parent.
	PublicIP *bool `json:"publicIP,omitempty"`

	// +kubebuilder:validation:Enum=PoweredOn;PoweredOff;Suspended
	// +optional

	// PowerState is the power state resolved from the portable object.
	//
	// Persisted rather than held in memory, so that reading this object shows
	// what the machine will actually do, and mutable because power state is
	// the one thing about a machine that is meant to change.
	//
	// Suspended is accepted here and can never be applied: EC2 has no
	// suspended state, and hibernation must be enabled when an instance is
	// launched. A request for it is reported through the UpToDate condition
	// and the machine is left alone.
	PowerState string `json:"powerState,omitempty"`
}

// The address types this provider reports, from the contract's vocabulary.
const (
	AddressInternalIP  = "InternalIP"
	AddressExternalIP  = "ExternalIP"
	AddressInternalDNS = "InternalDNS"
	AddressExternalDNS = "ExternalDNS"
)

// AWSMachineAddress is one network address of a machine.
type AWSMachineAddress struct {
	// +kubebuilder:validation:Enum=InternalIP;ExternalIP;InternalDNS;ExternalDNS
	// +required

	// Type distinguishes an address reachable only inside the network from
	// one reachable outside it.
	Type string `json:"type"`

	// +kubebuilder:validation:MinLength=1
	// +required

	// Address is the address itself.
	//
	// Never empty: the portable object this is copied onto requires at least
	// one character, and a value it refuses would stall the whole status
	// mirror rather than just this entry.
	Address string `json:"address"`
}

// AWSMachineStatus is what the core reads back.
//
// Every field here sits at a path the contract fixes. The core reads this
// object as unstructured, by path, knowing no AWS field name -- so a value
// this provider can see but the core cannot is worthless, and these names are
// not ours to choose.
type AWSMachineStatus struct {
	// +optional
	// +listType=atomic

	// Addresses are the machine's network addresses.
	Addresses []AWSMachineAddress `json:"addresses,omitempty"`

	// +kubebuilder:validation:Enum=PoweredOn;PoweredOff;Suspended
	// +optional

	// PowerState is the machine's observed power state.
	//
	// ABSENT while the instance is between steady states. That is not an
	// omission: an absent path is explicitly not an error under the contract,
	// and reporting a guess produces a UI that flickers between values the
	// machine was never in.
	PowerState string `json:"powerState,omitempty"`

	// +optional

	// ProviderID is a globally unique identifier for the machine, formatted
	// aws:///<availabilityZone>/<instanceID> to match Cluster API.
	ProviderID string `json:"providerID,omitempty"`

	// +optional

	// ProviderMetadata carries observed facts with no portable equivalent.
	//
	// The core copies it wholesale and never reads a value back into a
	// decision, so keys can be added without negotiating a schema change.
	ProviderMetadata map[string]string `json:"providerMetadata,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type

	// Conditions carry InfrastructureReady and UpToDate.
	//
	// Not Ready: the core reads the literal string "InfrastructureReady",
	// because a provider may already reserve "Ready" for its own,
	// differently-scoped meaning.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional

	// ObservedGeneration is the spec generation this status reflects.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional

	// LaunchRequested is when this provider first asked EC2 for the
	// instance, recorded before the call so a launch whose reply is lost is
	// still known to have been attempted.
	LaunchRequested *metav1.Time `json:"launchRequested,omitempty"`
}

// Condition types this provider reports, fixed by the contract.
const (
	// ConditionInfrastructureReady reports whether the instance exists and
	// has settled in the power state asked for.
	ConditionInfrastructureReady = "InfrastructureReady"

	// ConditionUpToDate reports whether everything asked for has been
	// applied.
	ConditionUpToDate = "UpToDate"
)

// Condition reasons this provider reports.
const (
	// ReasonNotAdopted means no VirtualMachine names this object.
	ReasonNotAdopted = "NotAdopted"

	// ReasonAlreadyOwned means another VirtualMachine owns it already.
	ReasonAlreadyOwned = "AlreadyOwned"

	// ReasonInvalidConfiguration means something asked for cannot exist.
	ReasonInvalidConfiguration = "InvalidConfiguration"

	// ReasonUnsupportedByProvider means something asked for is valid but
	// EC2 cannot express it.
	ReasonUnsupportedByProvider = "UnsupportedByProvider"

	// ReasonUnauthorized means an IAM permission is missing.
	ReasonUnauthorized = "Unauthorized"

	// ReasonWaitingForCapacity means EC2 has no room right now.
	ReasonWaitingForCapacity = "WaitingForCapacity"

	// ReasonProvisioning means the machine is being created.
	ReasonProvisioning = "Provisioning"

	// ReasonRunning means the machine exists and is running.
	ReasonRunning = "Running"

	// ReasonStopped says the instance is stopped, as asked.
	ReasonStopped = "Stopped"

	// ReasonPowerChanging says the instance is being started or stopped to
	// match the power state asked for.
	ReasonPowerChanging = "PowerChanging"

	// ReasonDeleting means the machine is being terminated.
	ReasonDeleting = "Deleting"

	// ReasonNotObserved means the portable object's intent has not been
	// compared with the machine on this pass: either nothing is adopted yet,
	// or the instance is gone.
	ReasonNotObserved = "NotObserved"

	// ReasonInstanceGone means EC2 no longer has the instance: it was
	// terminated, by this provider or by someone else, or EC2 has forgotten
	// it. Distinct from InvalidConfiguration because nothing about the
	// object is wrong.
	ReasonInstanceGone = "InstanceGone"
)

// The contract version this provider satisfies. Emitted as a CRD label from a
// marker rather than hand-added, because `make manifests` regenerates this
// file wholesale -- a hand-edit survives exactly until the next generation,
// and is then gone silently.
//
// Inert today: the core resolves a provider's version through the RESTMapper's
// preferred version and reads no label. Declared because the porting guide's
// Open Items records that the key "is not yet defined as a constant in this
// module", so this states intent ahead of a mechanism.
// ---------------------------------------------------------------------------

// The spec is written by the controller, not submitted by a user. This rule
// makes that structural rather than a convention: a create carrying any spec
// field is rejected, while the controller's later write -- which is an update
// -- is untouched.
//
// optionalOldSelf is what makes this expressible. Without it the rule is a
// transition rule and is skipped entirely on create, which is the one moment
// it needs to run. It must sit HERE, on the object, not on spec or on a
// field: at field scope "no old value" cannot distinguish a create from the
// controller filling an absent field, so the rule rejects our own write. The
// object always exists on an update, so at this scope oldSelf is present for
// every update and absent only at create. Measured, not assumed.
//
// WARNING: adding +kubebuilder:default to ANY AWSMachineSpec field breaks
// this. Defaulting runs before validation, so a defaulted field makes the
// spec non-empty at create and every ordinary `spec: {}` submission starts
// failing. There are deliberately no defaults on that struct.
//
// The first rule takes over once the launch is requested: from then on it
// fixes image, type and subnet, so a value can be filled in before the launch
// but not changed or removed after it. The two compose.
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.status) || !has(oldSelf.status.launchRequested) || (((has(self.spec) && has(self.spec.imageID)) ? self.spec.imageID : '') == ((has(oldSelf.spec) && has(oldSelf.spec.imageID)) ? oldSelf.spec.imageID : '') && ((has(self.spec) && has(self.spec.instanceType)) ? self.spec.instanceType : '') == ((has(oldSelf.spec) && has(oldSelf.spec.instanceType)) ? oldSelf.spec.instanceType : '') && ((has(self.spec) && has(self.spec.subnet)) ? self.spec.subnet : '') == ((has(oldSelf.spec) && has(oldSelf.spec.subnet)) ? oldSelf.spec.subnet : ''))",message="imageID, instanceType and subnet are fixed once the launch has been requested"
// +kubebuilder:validation:XValidation:rule="oldSelf.hasValue() ? true : (!has(self.spec) || (!has(self.spec.imageID) && !has(self.spec.instanceType) && !has(self.spec.subnet) && !has(self.spec.publicIP) && !has(self.spec.powerState)))",optionalOldSelf=true,message="AWSMachine.spec is written by the controller from the portable VirtualMachine; submit it empty"
// +kubebuilder:metadata:labels="kube-vm.io/v1alpha1=v1alpha1"
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=awsm,categories=kubevm
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.powerState`
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.instanceType`
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AWSMachine is one EC2 instance, backing one portable VirtualMachine.
type AWSMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional

	// Spec is empty on submission and filled in by the controller.
	Spec AWSMachineSpec `json:"spec,omitempty"`

	// +optional

	// Status is what the core reads back.
	Status AWSMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AWSMachineList is a list of AWSMachine.
type AWSMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// Items are the machines.
	Items []AWSMachine `json:"items"`
}

// init registers these types with the scheme builder.
func init() {
	SchemeBuilder.Register(&AWSMachine{}, &AWSMachineList{})
}
