// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VirtualMachineGroupPublishRequestConditionMembersOwnerRefReady indicates
	// that all the members of VirtualMachineGroupPublishRequest have owner reference
	// set.
	VirtualMachineGroupPublishRequestConditionMembersOwnerRefReady = "MembersOwnerRefReady"

	// VirtualMachineGroupPublishRequestConditionComplete is the Type for a
	// VirtualMachineGroupPublishRequest resource's status condition.
	//
	// The condition's status is set to true only when all other conditions
	// present on the resource have a truthy status.
	VirtualMachineGroupPublishRequestConditionComplete = "Complete"
)

// VirtualMachineGroupPublishRequestSource is the source of a publication request,
// typically a VirtualMachineGroup resource.
type VirtualMachineGroupPublishRequestSource struct {
	// +optional

	// Name is the name of the referenced object.
	//
	// If omitted this value defaults to the name of the
	// VirtualMachineGroupPublishRequest resource.
	Name string `json:"name,omitempty"`

	// +optional
	// +kubebuilder:default=vmoperator.vmware.com/v1alpha1

	// APIVersion is the API version of the referenced object.
	APIVersion string `json:"apiVersion,omitempty"`

	// +optional
	// +kubebuilder:default=VirtualMachineGroup

	// Kind is the kind of referenced object.
	Kind string `json:"kind,omitempty"`
}

// VirtualMachineGroupPublishRequestTarget is the target of a publication request,
// typically a ContentLibrary resource.
//
// Customized name of published target is not supported. The controller
// will use spec.source.name + "-image" as the name of the published item.
type VirtualMachineGroupPublishRequestTarget struct {
	// +optional

	// Description is the description assigned to all published targets that is
	// included in the VM Group.
	// If omitted, a default description will be assigned. "This image is
	// published through a virtual machine group publish request."
	Description string `json:"description,omitempty"`

	// +optional

	// Location contains information about the location to which to publish
	// all the virtual machines in the virtual machine group.
	Location VirtualMachineGroupPublishRequestTargetLocation `json:"location,omitempty"`
}

// VirtualMachineGroupPublishRequestTargetLocation is the location part of a
// publication request's target.
type VirtualMachineGroupPublishRequestTargetLocation struct {
	// +optional

	// Name is the name of the referenced object.
	//
	// Please note an error will be returned if this field is not
	// set in a namespace that lacks a default publication target.
	//
	// A default publication target is a resource with an API version
	// equal to spec.target.location.apiVersion, a kind equal to
	// spec.target.location.kind, and has the label
	// "imageregistry.vmware.com/default".
	Name string `json:"name,omitempty"`

	// +optional
	// +kubebuilder:default=imageregistry.vmware.com/v1alpha1

	// APIVersion is the API version of the referenced object.
	APIVersion string `json:"apiVersion,omitempty"`

	// +optional
	// +kubebuilder:default=ContentLibrary

	// Kind is the kind of referenced object.
	Kind string `json:"kind,omitempty"`
}

// VirtualMachineGroupPublishRequestSpec defines the desired state of a
// VirtualMachineGroupPublishRequest.
//
// All the fields in this spec are optional. This is especially useful when a
// DevOps persona wants to publish a VM Group without doing anything more than
// applying a VirtualMachineGroupPublishRequest resource that has the same name
// as said VMs in the same namespace as said VMs in VM Group.
type VirtualMachineGroupPublishRequestSpec struct {
	// +optional

	// Source is the source of the publication request,
	// ex. a VirtualMachineGroup resource.
	//
	// If this value is omitted then the publication controller checks to
	// see if there is a resource with the same name as this
	// VirtualMachineGroupPublishRequest resource, an API version equal to
	// spec.source.apiVersion, and a kind equal to spec.source.kind. If such
	// a resource exists, then it is the source of the publication.
	Source VirtualMachineGroupPublishRequestSource `json:"source,omitempty"`

	// +optional

	// Target is the target of the publication request, ex. items
	// description and the location of ContentLibrary resource.
	//
	// The controller uses spec.source.name + "-image" as the name of
	// the published item.
	//
	// When this value is omitted, controller attempts to identify the
	// target location by matching a resource with an API version equal to
	// spec.target.location.apiVersion, a kind equal to spec.target.location.kind,
	// w/ the label "imageregistry.vmware.com/default".
	//
	// Please note that while optional, if a VirtualMachineGroupPublishRequest sans
	// target information is applied to a namespace without a default
	// publication target, then the VirtualMachineGroupPublishRequest resource
	// will be marked in error.
	Target VirtualMachineGroupPublishRequestTarget `json:"target,omitempty"`

	// +optional
	// +kubebuilder:validation:Minimum=0

	// TTLSecondsAfterFinished is the time-to-live duration for how long this
	// resource will be allowed to exist once the publication operation
	// completes. After the TTL expires, the resource will be automatically
	// deleted without the user having to take any direct action.
	// This will be passed into each VirtualMachinePublishRequestSpec.
	//
	// If this field is unset then the request resource will not be
	// automatically deleted. If this field is set to zero then the request
	// resource is eligible for deletion immediately after it finishes.
	TTLSecondsAfterFinished *int64 `json:"ttlSecondsAfterFinished,omitempty"`
}

// VirtualMachineGroupPublishRequestStatus defines the observed state of a
// VirtualMachineGroupPublishRequest.
type VirtualMachineGroupPublishRequestStatus struct {
	// +optional

	// SourceRef is the reference to the source of the publication request,
	// ex. a VirtualMachineGroup resource.
	SourceRef *VirtualMachineGroupPublishRequestSource `json:"sourceRef,omitempty"`

	// +optional

	// TargetRef is the reference to the target of the publication request,
	// ex. item information and a ContentLibrary resource.
	TargetRef *VirtualMachineGroupPublishRequestTarget `json:"targetRef,omitempty"`

	// +optional

	// CompletionTime represents time when the request was completed. It is not
	// guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	//
	// The value of this field should be equal to the value of the
	// LastTransitionTime for the status condition Type=Complete.
	CompletionTime metav1.Time `json:"completionTime,omitempty"`

	// +optional

	// StartTime represents time when the request was acknowledged by the
	// controller. It is not guaranteed to be set in happens-before order
	// across separate operations. It is represented in RFC3339 form and is
	// in UTC.
	StartTime metav1.Time `json:"startTime,omitempty"`

	// +optional

	// ImageNames is a list of names of the VirtualMachineImage resources that
	// are eventually realized in the same namespace as the VM Group and publication
	// request after the publication operation completes.
	//
	// This field will not be set until the VirtualMachineImage resources are
	// realized.
	ImageNames []string `json:"imageName,omitempty"`

	// +optional

	// Ready is set to true only when all VMs in VM group have been published
	// successfully and the new VirtualMachineImage resources are ready.
	//
	// Readiness is determined by waiting until there is status condition
	// Type=Complete and ensuring it and all other status conditions present
	// have a Status=True. The conditions present will be:
	//
	//   * MembersOwnerRefReady
	//   * Complete
	Ready bool `json:"ready,omitempty"`

	// +optional

	// Conditions is a list of the latest, available observations of the
	// request's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmgrouppub
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachineGroupPublishRequest defines the information necessary to publish a
// VirtualMachineGroup as a VirtualMachineImage to an image registry.
type VirtualMachineGroupPublishRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineGroupPublishRequestSpec   `json:"spec,omitempty"`
	Status VirtualMachineGroupPublishRequestStatus `json:"status,omitempty"`
}

func (vmgrouppub *VirtualMachineGroupPublishRequest) GetConditions() []metav1.Condition {
	return vmgrouppub.Status.Conditions
}

func (vmgrouppub *VirtualMachineGroupPublishRequest) SetConditions(conditions []metav1.Condition) {
	vmgrouppub.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// VirtualMachineGroupPublishRequestList contains a list of
// VirtualMachineGroupPublishRequest resources.
type VirtualMachineGroupPublishRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachineGroupPublishRequest `json:"items"`
}

func init() {
	objectTypes = append(objectTypes,
		&VirtualMachineGroupPublishRequest{},
		&VirtualMachineGroupPublishRequestList{},
	)
}
