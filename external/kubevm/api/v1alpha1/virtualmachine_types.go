// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InstanceTypeSpec selects a virtual machine's compute sizing.
//
// Exactly one of Name or Resources should be set. Name is the portable default,
// because a named sizing profile is the one model every target platform has.
// Resources is an optional provider capability for platforms that support
// arbitrary sizing; a provider that cannot honor it rejects it at admission
// rather than silently rounding to a nearby profile.
type InstanceTypeSpec struct {
	// +optional

	// Name references a named sizing profile curated by an administrator — a
	// VirtualMachineClass on vSphere, a machine type on GCP, an instance type
	// on EC2.
	//
	// Accelerators are requested through this profile. A GPU-bearing profile
	// yields a GPU-bearing VM. There is deliberately no separate accelerator
	// field in this version of the API: on vSphere and EC2 the accelerator is
	// inseparable from the class or instance type, so a standalone field would
	// collapse back into profile selection for most providers. A portable
	// per-VM accelerator field is expected once Dynamic Resource Allocation
	// offers a shape that holds across providers.
	Name string `json:"name,omitempty"`

	// +optional

	// Resources describes inline, arbitrary sizing for providers that support
	// it (for example GCP custom machine types).
	Resources *ResourceSpec `json:"resources,omitempty"`
}

// ResourceSpec is inline compute sizing.
type ResourceSpec struct {
	// +kubebuilder:validation:Minimum=1
	// +optional

	// CPUs is the number of virtual CPUs presented to the guest.
	CPUs int32 `json:"cpus,omitempty"`

	// +optional

	// Memory is the guest-visible memory, for example "8Gi".
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// DiskSource describes where a disk's initial contents come from.
//
// Exactly one of Image, Snapshot, or Blank should be set.
type DiskSource struct {
	// +optional

	// Image references an entry in a provider-resolved image catalog. The
	// generic layer carries the reference, not the platform's image identity,
	// because a GCP image self-link is meaningless to vSphere. The referenced
	// kind may be a portable image type or one the provider owns; the reference
	// names it explicitly rather than assuming either.
	Image *ObjectReference `json:"image,omitempty"`

	// +optional

	// Snapshot references a snapshot to clone from.
	Snapshot *ObjectReference `json:"snapshot,omitempty"`

	// +optional

	// Blank requests an empty disk. Only meaningful for data disks.
	Blank bool `json:"blank,omitempty"`
}

// BootDiskSpec describes the primary (boot) disk.
type BootDiskSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="boot disk source is immutable once the disk exists"
	// +required

	// Source is the image or snapshot the boot disk is created from.
	//
	// Immutable. Once the disk exists it is a disk, not a reference to the
	// artifact it was made from, so repointing this cannot mean anything short
	// of replacing the machine. Every reference platform agrees: the image is
	// a launch-time-only parameter.
	Source DiskSource `json:"source"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self >= oldSelf",message="boot disk size cannot be reduced"
	// +optional

	// SizeGiB grows the boot disk beyond the source's own size.
	//
	// Grow-only, and now enforced rather than only documented. Growth is not
	// necessarily immediate: EC2 imposes a cooldown between successive
	// modifications of one volume, and the guest filesystem must be extended
	// separately. Providers report that lag through the UpToDate condition
	// rather than by rejecting the edit.
	SizeGiB *int64 `json:"sizeGiB,omitempty"`

	// +optional

	// StorageClass is a provider-interpreted storage selector — a vSphere
	// storage policy, a GCP or EC2 disk type.
	StorageClass string `json:"storageClass,omitempty"`

	// +optional

	// DeleteOnTermination deletes the boot disk when the VM is deleted.
	//
	// Present here as well as on DiskSpec because every reference platform
	// makes this settable on the boot disk specifically — GCE's
	// disks[0].autoDelete, EC2's root BlockDeviceMappings[].Ebs
	// .DeleteOnTermination — and retaining a boot disk after the machine is
	// gone is an ordinary workflow: forensics, re-attaching it to a
	// replacement, or capturing an image from it.
	DeleteOnTermination *bool `json:"deleteOnTermination,omitempty"`
}

// DiskSpec describes an additional (data) disk.
type DiskSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +required

	// Name identifies this disk within the VirtualMachine. Must be unique
	// among the VM's disks.
	Name string `json:"name"`

	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="disk source is immutable once the disk exists"
	// +required

	// Source is the image, snapshot, or blank volume backing this disk.
	// Immutable, for the same reason as BootDiskSpec.Source.
	Source DiskSource `json:"source"`

	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self >= oldSelf",message="disk size cannot be reduced"
	// +optional

	// SizeGiB is the requested disk size. Grow-only, as for the boot disk.
	SizeGiB *int64 `json:"sizeGiB,omitempty"`

	// +optional

	// StorageClass is a provider-interpreted storage selector.
	StorageClass string `json:"storageClass,omitempty"`

	// +optional

	// DeleteOnTermination deletes this disk when the VM is deleted.
	DeleteOnTermination *bool `json:"deleteOnTermination,omitempty"`
}

// NetworkSpec describes a virtual machine's guest network configuration.
//
// Interfaces are nested here rather than sitting directly on the spec because
// some network settings are properties of the guest as a whole, not of any one
// interface: a host name is singular, and a resolver list is conventionally
// system-wide. Giving those a home alongside the interface list keeps a single
// concern in a single place, and leaves room to add further guest-wide settings
// without further widening the top-level spec. VM Operator arrived at the same
// shape in its spec.network.
//
// Every field here is deliberately one the API itself can render into guest
// network configuration, which is why each has a translation on more than one
// backend: on vSphere through guest customization, and elsewhere through
// cloud-init, whose network-config schema carries the same three concepts.
// Settings that only one platform can honor are not promoted here — they belong
// on that provider's infrastructure object.
type NetworkSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="hostName is immutable"
	// +optional

	// HostName is the host name assigned to the guest. When omitted, the
	// provider uses the VirtualMachine's name.
	//
	// Immutable. GCE fixes the instance host name at create time, and while
	// vSphere can change it through guest customization, allowing the edit
	// would make the field silently non-portable. Marked immutable for now
	// because relaxing an immutability constraint later is a compatible
	// change and adding one is not.
	HostName *string `json:"hostName,omitempty"`

	// +kubebuilder:validation:MaxItems=3
	// +optional
	// +listType=atomic

	// Nameservers is the list of DNS server addresses configured in the guest.
	// It applies to every interface that does not specify its own. When
	// omitted, resolvers come from whatever the attached networks supply,
	// typically over DHCP.
	//
	// This pairs with an interface's Addresses field: a statically addressed
	// interface receives no resolvers from DHCP, so a VM that sets Addresses
	// generally has to set this too.
	Nameservers []string `json:"nameservers,omitempty"`

	// +optional
	// +listType=atomic

	// SearchDomains is the DNS search list configured in the guest. As with
	// Nameservers, it applies to every interface that does not specify its own.
	SearchDomains []string `json:"searchDomains,omitempty"`

	// +kubebuilder:validation:MaxItems=10
	// +optional
	// +listType=map
	// +listMapKey=name

	// Interfaces are the VM's network interfaces. When omitted, the provider
	// attaches a single interface to the namespace's default network.
	Interfaces []NetworkInterfaceSpec `json:"interfaces,omitempty"`
}

// NetworkInterfaceSpec describes one network interface.
//
// The shape of an interface is portable; the network it attaches to is not, so
// Network is a reference into whatever object the provider uses to model a
// network.
type NetworkInterfaceSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +required

	// Name identifies this interface within the VirtualMachine. Must be unique
	// among the VM's interfaces.
	Name string `json:"name"`

	// +optional

	// Network references the provider's network object. When omitted, the
	// provider selects a default network for the namespace.
	Network *ObjectReference `json:"network,omitempty"`

	// +optional
	// +listType=atomic

	// Addresses assigns specific IP addresses to this interface, as bare
	// addresses without a prefix length — for example "10.0.1.5", not
	// "10.0.1.5/24".
	//
	// The prefix is deliberately absent because on the cloud platforms it is a
	// property of the attached network rather than of the interface: EC2 takes
	// a bare PrivateIpAddress whose mask comes from the subnet, and GCE takes
	// a bare networkIP that must fall inside the regional subnetwork. A
	// caller-supplied prefix would be either redundant or contradictory.
	//
	// This is not mutually exclusive with the DHCP fields. On the cloud
	// platforms an address set here is reserved through the platform API and
	// then handed to the guest over DHCP, so the two operate together; a
	// provider that instead configures the guest directly may ignore the DHCP
	// fields. Whether an address is requested and how it reaches the guest are
	// separate questions.
	Addresses []string `json:"addresses,omitempty"`

	// +optional

	// DHCP4 requests an IPv4 address via DHCP. Defaults to true when no static
	// IPv4 address is supplied.
	DHCP4 *bool `json:"dhcp4,omitempty"`

	// +optional

	// DHCP6 requests an IPv6 address via DHCP.
	DHCP6 *bool `json:"dhcp6,omitempty"`

	// +optional

	// PublicIP requests an externally reachable address. A provider that
	// cannot supply one rejects this at admission.
	PublicIP *bool `json:"publicIP,omitempty"`
}

// CloudInitSpec supplies cloud-init data to the guest.
//
// Both fields are Secret references rather than inline strings, so bootstrap
// data — which routinely contains credentials — stays in Secrets and out of the
// VirtualMachine object.
type CloudInitSpec struct {
	// +optional

	// UserData selects the cloud-init user-data document.
	UserData *SecretKeySelector `json:"userData,omitempty"`

	// +optional

	// NetworkData selects the cloud-init network-config document.
	NetworkData *SecretKeySelector `json:"networkData,omitempty"`
}

// BootstrapSpec describes guest initialization.
//
// cloud-init is the portable baseline: every target platform can deliver it,
// whether through guest customization, instance user-data, or a metadata
// service. Note this is a baseline rather than a universal — Windows guests on
// EC2 are configured by EC2Launch v2, not cloud-init, so they are not reachable
// through this path today. Other engines (Ignition, Sysprep) are additive and
// deliberately not defined until a provider needs one.
//
// There is exactly one bootstrap channel, deliberately. An earlier draft also
// carried a free-form metadata map, on the reasoning that several platforms
// expose a metadata service. But on those platforms user-data *is* a metadata
// key — GCE delivers it as metadata.items[] keyed "user-data" — so the two were
// not two features. They were one wire with two doors, and the second door was
// strictly worse: inline on this object instead of Secret-backed, and with no
// defined precedence against the first. The same duplication applied to
// startup-script and ssh-keys, which restate CloudInit and SSHPublicKeys.
//
// The one thing that map did which this does not is offer a runtime
// guest-readable key/value channel — configuration a guest reads from a
// metadata service continuously rather than once at boot. That is a different
// concept and does not belong under bootstrap. It is also not portable yet:
// EC2 has no user-settable instance metadata map, and its nearest equivalent
// surfaces instance *tags* through IMDS, so on EC2 the concept collides with
// Tags rather than standing apart from it. If it is wanted later it needs its
// own field, its own justification, and a resolution against Tags first.
type BootstrapSpec struct {
	// +optional

	// CloudInit is the portable bootstrap path.
	CloudInit *CloudInitSpec `json:"cloudInit,omitempty"`
}

// SchedulingSpec carries placement economics.
type SchedulingSpec struct {
	// +optional

	// Spot requests preemptible/spot capacity. A provider with no such notion
	// rejects a true value at admission rather than silently running the VM at
	// full price.
	Spot *bool `json:"spot,omitempty"`
}

// VirtualMachineSpec is the portable, provider-agnostic description of a
// virtual machine.
type VirtualMachineSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="infrastructureRef is immutable"
	// +required

	// InfrastructureRef points at the provider object that carries
	// platform-specific configuration and acts as the duck-typed status
	// source.
	//
	// Immutable. This reference is the machine's identity: it names the object
	// holding the backend VM. Repointing it would abandon a running machine
	// while claiming ownership of another, so replacing it means deleting this
	// VirtualMachine and creating a new one.
	//
	// The user creates that object alongside this VirtualMachine; the core
	// controller adopts it by setting an owner reference and manages its
	// lifecycle. The core does not author the provider's spec.
	InfrastructureRef ObjectReference `json:"infrastructureRef"`

	// +kubebuilder:default=PoweredOn
	// +optional

	// PowerState is the desired power state. Providers reconcile toward it.
	//
	// Modeled as declarative desired state rather than as a run strategy,
	// because that is the one power model shared by vSphere, GCP, and EC2.
	// Automatic restart is intentionally not folded into this field.
	PowerState PowerState `json:"powerState,omitempty"`

	// +kubebuilder:default=TrySoft
	// +optional

	// PowerOffMode controls whether a power-off or suspend involves the guest.
	PowerOffMode PowerOpMode `json:"powerOffMode,omitempty"`

	// +optional

	// InstanceType selects compute sizing, including any accelerator.
	InstanceType *InstanceTypeSpec `json:"instanceType,omitempty"`

	// +optional

	// BootDisk describes the primary disk and the image it comes from.
	BootDisk *BootDiskSpec `json:"bootDisk,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=name

	// Disks are additional data disks.
	Disks []DiskSpec `json:"disks,omitempty"`

	// +optional

	// Network describes the VM's guest network configuration: settings that
	// apply to the guest as a whole, plus the per-interface list.
	Network *NetworkSpec `json:"network,omitempty"`

	// +optional

	// Bootstrap describes guest initialization.
	Bootstrap *BootstrapSpec `json:"bootstrap,omitempty"`

	// +optional
	// +listType=atomic

	// SSHPublicKeys is inline public-key material that every provider can
	// honor through cloud-init. A reference to a platform's named key-pair
	// registry is provider-specific and belongs on the infrastructure object.
	SSHPublicKeys []string `json:"sshPublicKeys,omitempty"`

	// +optional

	// FailureDomain pins the VM to a zone or availability zone. The value is
	// an opaque, provider-interpreted string.
	FailureDomain *string `json:"failureDomain,omitempty"`

	// +optional

	// Scheduling carries placement economics such as spot capacity.
	Scheduling *SchedulingSpec `json:"scheduling,omitempty"`

	// +kubebuilder:validation:MaxProperties=50
	// +optional

	// Tags is a set of free-form, provider-interpreted key/value labels,
	// mapped to the platform's own labelling mechanism: vSphere tags (which
	// are category/tag pairs), EC2 resource tags, or GCP labels. Kubernetes
	// labels remain the primary metadata channel for anything the control
	// plane reads.
	//
	// Key/value rather than a bare list, because all three reference
	// platforms model this as key/value and none as a flat list. A list would
	// force every provider to synthesize keys, and on GCP a bare list is
	// specifically the shape of network tags — the firewall target selector —
	// which is not what this field means.
	//
	// This is not firewall attachment: security groups and firewall-targeting
	// network tags are object-reference shaped and belong on the
	// infrastructure object.
	//
	// Providers apply the stricter of their own constraints and reject values
	// they cannot represent. Platform limits differ sharply — EC2 reserves the
	// "aws:" key prefix and caps keys at 128 and values at 256 characters,
	// while GCP labels must be lowercase RFC1035 — so a portable value is one
	// that satisfies all of them.
	Tags map[string]string `json:"tags,omitempty"`
}

// VirtualMachineStatus is the observed state of a virtual machine.
//
// Every field here other than Conditions and ObservedGeneration is derived from
// the infrastructure object through the duck-typed provider contract.
type VirtualMachineStatus struct {
	// +optional

	// Phase is a portable lifecycle summary derived from the provider's
	// reported instance state.
	Phase VirtualMachinePhase `json:"phase,omitempty"`

	// +optional

	// Ready mirrors the infrastructure object's readiness.
	Ready bool `json:"ready,omitempty"`

	// +optional

	// PowerState is the observed power state, which may differ from the
	// desired state in spec while a change is converging.
	PowerState PowerState `json:"powerState,omitempty"`

	// +optional

	// ProviderID is the platform-unique identifier for this VM, copied from
	// the infrastructure object.
	ProviderID string `json:"providerID,omitempty"`

	// +optional
	// +listType=atomic

	// Addresses are the VM's observed network addresses.
	Addresses []VirtualMachineAddress `json:"addresses,omitempty"`

	// +optional

	// FailureReason is a terse, machine-readable cause for a provisioning
	// failure.
	//
	// This is deliberately not documented as terminal. Several of the failures
	// a provider reports here are retryable or even routine — EC2's
	// InsufficientInstanceCapacity succeeds on retry in another zone, and a
	// spot interruption is expected behavior rather than a fault — so treating
	// the field as an absorbing state would be wrong. Whether a given failure
	// is retryable belongs in Conditions, which can carry a reason and change
	// back. Cluster API is removing its equivalent fields for the same reason,
	// and these two may follow.
	FailureReason *string `json:"failureReason,omitempty"`

	// +optional

	// FailureMessage is a human-readable description of a provisioning
	// failure. See FailureReason on why this is not a terminal state.
	FailureMessage *string `json:"failureMessage,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type

	// Conditions describes the VM's current state, including
	// InfrastructureReady mirrored from the infrastructure object.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional

	// ObservedGeneration is the spec generation most recently reconciled.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional

	// ProviderMetadata is a free-form, provider-authored map for observed
	// facts that have no portable equivalent.
	//
	// This is the only unstructured field in the API, and it is deliberately
	// on status rather than spec: providers can surface extra detail without
	// negotiating a schema change, and the core controller never reads it back
	// into a reconcile decision.
	ProviderMetadata map[string]string `json:"providerMetadata,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:shortName=vm;vms,categories=kubevm
// +kubebuilder:printcolumn:name="Power",type=string,JSONPath=`.status.powerState`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Provider-ID",type=string,priority=1,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Primary-IP",type=string,priority=1,JSONPath=`.status.addresses[?(@.type=="InternalIP")].address`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VirtualMachine is a portable, provider-agnostic virtual machine.
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +optional

	Spec VirtualMachineSpec `json:"spec,omitempty"`

	// +optional

	Status VirtualMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachineList is a list of VirtualMachine objects.
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachine{}, &VirtualMachineList{})
}
