// Copyright (c) 2022 VMware, Inc. All Rights Reserved.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterContentLibraryItemSpec defines the desired state of a ClusterContentLibraryItem.
type ClusterContentLibraryItemSpec struct {
	// UUID is the identifier which uniquely identifies the library item in vCenter. This field is immutable.
	// +required
	UUID string `json:"uuid"`
}

// ClusterContentLibraryItemStatus defines the observed state of ClusterContentLibraryItem.
type ClusterContentLibraryItemStatus struct {
	// ItemName specifies the name of the content library item in vCenter.
	// +required
	Name string `json:"name"`

	// Description is a human-readable description for this library item.
	// +optional
	Description string `json:"description,omitempty"`

	// ClusterContentLibraryRef is the name of the ClusterContentLibrary resource that this item belongs to.
	// +required
	ClusterContentLibraryRef string `json:"clusterContentLibraryRef"`

	// MetadataVersion indicates the version of the library item metadata.
	// This value is incremented when the library item properties such as name or description are changed in vCenter.
	// +required
	MetadataVersion string `json:"metadataVersion"`

	// ContentVersion indicates the version of the library item content.
	// This value is incremented when the files comprising the content library item are changed in vCenter.
	// +required
	ContentVersion string `json:"contentVersion"`

	// Type string indicates the type of the library item in vCenter.
	// Possible types are "Ovf" and "Iso".
	// +required
	Type ContentLibraryItemType `json:"type"`

	// Size indicates the library item size in bytes
	// +optional
	Size int32 `json:"size,omitempty"`

	// Cached indicates if the library item files are on disk in vCenter.
	// +required
	// +kubebuilder:default=false
	Cached bool `json:"cached"`

	// Ready denotes that the library item is ready to be used.
	// +required
	// +kubebuilder:default=false
	Ready bool `json:"ready"`

	// CreationTime indicates the date and time when this library item was created.
	// +required
	CreationTime string `json:"creationTime"`

	// LastModifiedTime indicates the date and time when this library item was last updated.
	// This field is updated when the library item properties are changed or the file content is changed.
	// +required
	LastModifiedTime string `json:"lastModifiedTime"`

	// LastSyncTime indicates the date and time when this library item was last synchronized.
	// This field applies only to subscribed library items.
	// +optional
	LastSyncTime string `json:"lastSyncTime,omitempty"`

	// Conditions describes the current condition information of the ClusterContentLibraryItem.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

func (cclItem *ClusterContentLibraryItem) GetConditions() Conditions {
	return cclItem.Status.Conditions
}

func (cclItem *ClusterContentLibraryItem) SetConditions(conditions Conditions) {
	cclItem.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=cclitem
// +kubebuilder:printcolumn:name="vSphereName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="ClusterContentLibraryRef",type="string",JSONPath=".status.clusterContentLibraryRef"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterContentLibraryItem is the schema for the content library item API at the cluster scope.
// Currently, ClusterContentLibraryItem are immutable to end users.
type ClusterContentLibraryItem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterContentLibraryItemSpec   `json:"spec,omitempty"`
	Status ClusterContentLibraryItemStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterContentLibraryItemList contains a list of ClusterContentLibraryItem.
type ClusterContentLibraryItemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterContentLibraryItem `json:"items"`
}

func init() {
	RegisterTypeWithScheme(&ClusterContentLibraryItem{}, &ClusterContentLibraryItemList{})
}
