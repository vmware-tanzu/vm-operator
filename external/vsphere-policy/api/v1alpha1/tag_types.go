// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TagDeleteFailedReason indicates the Tag's owner-reference list is empty
	// but deleting the Tag failed.
	TagDeleteFailedReason = "DeleteFailed"
)

// TagType describes the origin/category of a Tag resource.
type TagType string

const (
	// TagTypeSystem indicates the Tag is managed by the system.
	TagTypeSystem TagType = "System"
)

// TagKind identifies the meaning of a concrete backend tag.
type TagKind string

const (
	// TagKindOriginal represents a shared original label tag.
	TagKindOriginal TagKind = "Original"
	// TagKindAffinityTarget represents one affinity policy's target tag.
	TagKindAffinityTarget TagKind = "AffinityTarget"
	// TagKindGroupedAntiAffinityTarget represents one grouped policy target label.
	TagKindGroupedAntiAffinityTarget TagKind = "GroupedAntiAffinityTarget"
	// TagKindGroupedAntiAffinitySelf represents a grouped anti-affinity requester tag.
	TagKindGroupedAntiAffinitySelf TagKind = "GroupedAntiAffinitySelf"
)

// TagMembership specifies which VMs carry a vc tag.
type TagMembership string

const (
	// TagMembershipSourceLabel selects VMs carrying the source label.
	TagMembershipSourceLabel TagMembership = "SourceLabel"
	// TagMembershipPolicyRequesters selects VMs currently requesting the policy.
	TagMembershipPolicyRequesters TagMembership = "PolicyRequesters"
)

// TagLabelPair is a resolved policy target label.
type TagLabelPair struct {
	// Key is the Kubernetes label key.
	// +required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
	// Value is the Kubernetes label value, which may be empty.
	// +required
	Value string `json:"value"`
}

// TagPolicyIdentity holds the canonical inputs of a derived tag's policy hash.
type TagPolicyIdentity struct {
	// Type is the backend affinity policy type.
	// +required
	// +kubebuilder:validation:Enum=VmVmAffinity;VmToVmGroupsAntiAffinity
	Type string `json:"type"`
	// Strictness specifies placement and execution enforcement.
	// +required
	// +kubebuilder:validation:Enum=PreferredDuringPlacementPreferredDuringExecution;RequiredDuringPlacementPreferredDuringExecution;RequiredDuringPlacementRequiredDuringExecution
	Strictness string `json:"strictness"`
	// Topology specifies the policy's scope.
	// +required
	// +kubebuilder:validation:Enum=Host;VSphereZone
	Topology string `json:"topology"`
	// Targets are sorted by key:value and contain no duplicate pairs.
	// +required
	// +listType=atomic
	// +kubebuilder:validation:MinItems=1
	Targets []TagLabelPair `json:"targets"`
}

// TagSpec defines the immutable identity and membership of Tag.
// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="tag identity is immutable"
// +kubebuilder:validation:XValidation:rule="self.kind == 'GroupedAntiAffinitySelf' ? (!has(self.key) && !has(self.value)) : has(self.key)",message="source label is required for original/affinity tags and forbidden for self tags"
type TagSpec struct {
	// Name is the exact backend tag name. The namespace supplies its category.
	// +required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Kind describes the vc tag's role.
	// +required
	// +kubebuilder:validation:Enum=Original;AffinityTarget;GroupedAntiAffinityTarget;GroupedAntiAffinitySelf
	Kind TagKind `json:"kind"`
	// Membership determines which VMs carry this tag, independently of ownership.
	// +required
	// +kubebuilder:validation:Enum=SourceLabel;PolicyRequesters
	Membership TagMembership `json:"membership"`
	// Key is the source label key. It is absent for self tags.
	// +optional
	Key string `json:"key,omitempty"`
	// Value is the source label value. Omission means an empty label value.
	// It is absent for self tags.
	// +optional
	Value string `json:"value,omitempty"`
	// Policy contains policy spec that created this Tag.
	// +optional
	Policy *TagPolicyIdentity `json:"policy,omitempty"`
	// Type describes the origin/category of this Tag. Tags created by VMOperator are System tags.
	// +optional
	// +kubebuilder:validation:Enum=System
	// +kubebuilder:default=System
	Type TagType `json:"type,omitempty"`
}

// TagStatus defines the observed state of Tag.
type TagStatus struct {
	// +optional

	// ID is reserved for the resolved vCenter Tag UUID.
	ID string `json:"id,omitempty"`

	// +optional

	// ObservedGeneration describes the value of the metadata.generation
	// field the last time this object was reconciled by its primary
	// controller.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	// +listType=map
	// +listMapKey=type

	// Conditions describes any conditions associated with this object.
	//
	// The Ready condition will be present once this object has been
	// reconciled.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:storageversion:true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tag",type="string",JSONPath=".spec.name"
// +kubebuilder:printcolumn:name="Kind",type="string",JSONPath=".spec.kind"
// +kubebuilder:printcolumn:name="Key",type="string",JSONPath=".spec.key"
// +kubebuilder:printcolumn:name="Value",type="string",JSONPath=".spec.value"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Tag is the schema for the Tag API and represents the desired state and
// observed status of a Tag resource.
type Tag struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TagSpec   `json:"spec,omitempty"`
	Status TagStatus `json:"status,omitempty"`
}

// GetConditions returns the Tag's conditions, allowing Tag to implement the
// conditions.Getter interface.
func (t *Tag) GetConditions() []metav1.Condition {
	return t.Status.Conditions
}

// SetConditions sets the Tag's conditions, allowing Tag to implement the
// conditions.Setter interface.
func (t *Tag) SetConditions(conditions []metav1.Condition) {
	t.Status.Conditions = conditions
}

// SetConditions sets the TagStatus's conditions, allowing TagStatus to
// implement the conditions.Setter interface.
func (t *TagStatus) SetConditions(conditions []metav1.Condition) {
	t.Conditions = conditions
}

// +kubebuilder:object:root=true

// TagList contains a list of Tag objects.
type TagList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tag `json:"items"`
}

func init() {
	objectTypes = append(objectTypes, &Tag{}, &TagList{})
}
