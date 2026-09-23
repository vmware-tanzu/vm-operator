// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Finalizer holds a ContainerMachine until its container has been removed.
//
// Without it the container leaks: the core deletes the provider object and
// waits, so nothing else stands between a deleted VirtualMachine and a
// container the local runtime still knows about.
const Finalizer = "infrastructure.kube-vm.io/containermachine"

// AnnotationKey is the annotation a ContainerMachine carries to name the
// VirtualMachine it belongs to. Adoption only happens when that VirtualMachine
// ALSO names this object in spec.infrastructureRef — a one-sided reference
// links nothing. See docs/implementing-a-provider.md in the kubevm module for
// the full explanation of why both sides are required.
const AnnotationKey = "kube-vm.io/virtual-machine"

// ContainerRuntime selects the local container engine this provider drives.
//
// +kubebuilder:validation:Enum=docker;podman
type ContainerRuntime string

const (
	ContainerRuntimeDocker ContainerRuntime = "docker"
	ContainerRuntimePodman ContainerRuntime = "podman"
)

// ContainerMachineSpec is the container-specific half of a machine.
//
// Image and PowerState are resolved by the controller from the portable
// VirtualMachine that owns this object, following the same rule
// external/kubevm-provider-aws uses: a person submits this object with those
// two fields empty, and the controller fills them in from the parent on every
// reconcile. Runtime is the one field with no portable equivalent — nothing in
// VirtualMachineSpec says "docker" or "podman" — so it is the one field a
// person sets directly on THIS object at creation, and the controller never
// touches it. See implementing-a-provider.md, "Fields with no portable
// equivalent", for why that split is deliberate rather than an oversight.
type ContainerMachineSpec struct {
	// +optional

	// Image is the OCI image reference to run, resolved from the portable
	// object's boot disk image reference (spec.bootDisk.source.image.name),
	// read verbatim the same way external/kubevm-provider-aws reads an AMI
	// id from that field.
	//
	// Cannot change once status.containerID is set: a running container
	// keeps the image it was created from, exactly as a running EC2 instance
	// keeps its AMI. Until then the controller keeps this in step with the
	// portable object.
	Image string `json:"image,omitempty"`

	// +kubebuilder:validation:Enum=docker;podman
	// +kubebuilder:default=docker
	// +optional

	// Runtime selects the local container engine. Unlike Image and
	// PowerState, this has no counterpart on the portable VirtualMachine —
	// no reference platform in this contract has a notion of "docker vs.
	// podman" — so it is written once, by whoever creates this object, and
	// the controller never overwrites it.
	Runtime ContainerRuntime `json:"runtime,omitempty"`

	// +kubebuilder:validation:Enum=PoweredOn;PoweredOff
	// +optional

	// PowerState is the power state resolved from the portable object.
	//
	// Mutable, like on every other provider: it is the one thing about a
	// machine that is meant to change after creation. Suspended is not
	// accepted: neither docker nor podman has a suspend-to-disk primitive
	// this provider implements, so a request for it is reported through the
	// UpToDate condition instead of silently mapped onto "stopped".
	PowerState string `json:"powerState,omitempty"`
}

// The address type this provider reports, from the contract's vocabulary.
// Only InternalIP: a container's address is only ever reachable from inside
// the docker/podman network it was created in.
const AddressInternalIP = "InternalIP"

// ContainerMachineAddress is one network address of a machine.
type ContainerMachineAddress struct {
	// Interface names the network interface inside the container this
	// address belongs to. The contract treats this field as optional; this
	// provider always sets it, since the docker/podman network the address
	// came from is already known when it is recorded.
	// +optional
	Interface string `json:"interface,omitempty"`

	// +kubebuilder:validation:Enum=InternalIP
	// +required
	Type string `json:"type"`

	// +kubebuilder:validation:MinLength=1
	// +required
	Address string `json:"address"`
}

// ContainerMachineStatus is what the core reads back.
//
// Every field here sits at a path the contract fixes; see
// external/kubevm/controller/internal/contract/contract.go. The core reads
// this object as unstructured, by path, knowing no container-specific field
// name, so a value this provider can see but the core cannot is worthless.
type ContainerMachineStatus struct {
	// +optional
	// +listType=atomic
	Addresses []ContainerMachineAddress `json:"addresses,omitempty"`

	// +kubebuilder:validation:Enum=PoweredOn;PoweredOff
	// +optional

	// PowerState is the container's observed power state.
	//
	// ABSENT, not empty-string and not a guess, while the container is
	// between steady states (created but not yet started, or in the middle
	// of stopping). An absent path is explicitly not an error under the
	// contract.
	PowerState string `json:"powerState,omitempty"`

	// +optional

	// ProviderID is a globally unique identifier for the machine, formatted
	// container://<runtime>/<containerID>.
	ProviderID string `json:"providerID,omitempty"`

	// +optional

	// ProviderMetadata carries observed facts with no portable equivalent,
	// such as which runtime actually created the container and the
	// deterministic container name this provider uses. The core copies it
	// wholesale and never reads a value back into a decision.
	ProviderMetadata map[string]string `json:"providerMetadata,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type

	// Conditions carry InfrastructureReady and UpToDate.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +kubebuilder:validation:Minimum=0
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional

	// ContainerID is the full container ID once created, kept independently
	// of ProviderID because the latter is a formatted contract string and
	// this is the raw value this provider's own reconcile logic keys off of.
	ContainerID string `json:"containerID,omitempty"`
}

// Condition types this provider reports, fixed by the contract.
const (
	ConditionInfrastructureReady = "InfrastructureReady"
	ConditionUpToDate            = "UpToDate"
)

// Condition reasons this provider reports.
const (
	// ReasonNotAdopted means no VirtualMachine names this object, or the
	// link is not mutual yet.
	ReasonNotAdopted = "NotAdopted"

	// ReasonAlreadyOwned means another VirtualMachine owns it already.
	ReasonAlreadyOwned = "AlreadyOwned"

	// ReasonInvalidConfiguration means something asked for cannot exist —
	// for example, no boot image was named.
	ReasonInvalidConfiguration = "InvalidConfiguration"

	// ReasonUnsupportedByProvider means something asked for is valid but
	// this provider cannot express it (for example, a Suspended power
	// state).
	ReasonUnsupportedByProvider = "UnsupportedByProvider"

	// ReasonRuntimeError means the docker/podman CLI invocation itself
	// failed.
	ReasonRuntimeError = "RuntimeError"

	// ReasonProvisioning means the container is being created.
	ReasonProvisioning = "Provisioning"

	// ReasonRunning means the container exists and is running.
	ReasonRunning = "Running"

	// ReasonStopped says the container is stopped, as asked.
	ReasonStopped = "Stopped"

	// ReasonPowerChanging says the container is being started or stopped to
	// match the power state asked for.
	ReasonPowerChanging = "PowerChanging"

	// ReasonContainerGone means the runtime no longer knows this container:
	// it was removed, by this provider or by someone else.
	ReasonContainerGone = "ContainerGone"
)

// +kubebuilder:metadata:labels="kube-vm.io/v1alpha1=v1alpha1"
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cm,categories=kubevm
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.powerState`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="ProviderID",type=string,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ContainerMachine is one Docker or Podman container, backing one portable
// VirtualMachine.
//
// It exists to give external/kubevm/docs/implementing-a-provider.md a
// runnable example: bootstrapping a new provider from nothing, with no
// hypervisor or cloud account required to try it.
type ContainerMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	Spec ContainerMachineSpec `json:"spec,omitempty"`

	// +optional
	Status ContainerMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContainerMachineList is a list of ContainerMachine.
type ContainerMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ContainerMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ContainerMachine{}, &ContainerMachineList{})
}
