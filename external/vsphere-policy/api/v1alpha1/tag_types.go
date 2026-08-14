// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// TagReadyReason indicates the Tag has been observed at its current
	// generation, its label mirror matches its spec, and its owner set is
	// non-empty.
	TagReadyReason = "Ready"

	// TagNoOwnersReason indicates the Tag's owner-reference list is empty
	// and the Tag is being deleted.
	TagNoOwnersReason = "NoOwners"
)

// TagType describes the origin/category of a Tag resource.
type TagType string

const (
	// TagTypeSystem indicates the Tag is managed by the system.
	TagTypeSystem TagType = "System"
)

// TagSpec defines the desired state of Tag.
type TagSpec struct {
	// +required
	// +kubebuilder:validation:MinLength=1

	// Key is the label key this tag represents.
	Key string `json:"key"`

	// +required

	// Value is the label value this tag represents. An empty value is a
	// legal Kubernetes label value, so this field has no MinLength.
	Value string `json:"value"`

	// +optional

	// ServerID is the GUID of the target vCenter. Recorded for
	// forward-compatibility; a single vCenter is assumed by this feature.
	ServerID string `json:"serverID,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum=System
	// +kubebuilder:default=System

	// Type describes the origin/category of this Tag.
	Type TagType `json:"type,omitempty"`
}

// TagStatus defines the observed state of Tag.
type TagStatus struct {
	// +optional

	// ID is reserved for the resolved vCenter Tag UUID. This field is not
	// populated by this feature.
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

// GetConditions returns the TagStatus's conditions, allowing TagStatus to
// implement the conditions.Getter interface.
func (t *TagStatus) GetConditions() []metav1.Condition {
	return t.Conditions
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
