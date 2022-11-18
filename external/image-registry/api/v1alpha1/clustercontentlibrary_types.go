// Copyright (c) 2022 VMware, Inc. All Rights Reserved.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterContentLibrarySpec defines the desired state of a ClusterContentLibrary.
type ClusterContentLibrarySpec struct {
	// UUID is the identifier which uniquely identifies the library in vCenter. This field is immutable.
	// +required
	UUID string `json:"uuid"`
}

// ClusterContentLibraryStatus defines the observed state of ClusterContentLibrary.
type ClusterContentLibraryStatus struct {
	// Name specifies the name of the content library in vCenter.
	// +required
	Name string `json:"name"`

	// Description is a human-readable description for this library.
	// +optional
	Description string `json:"description,omitempty"`

	// Type indicates the type of a library in vCenter.
	// Possible types are "Local" and "Subscribed".
	// +required
	Type ContentLibraryType `json:"type"`

	// StorageBacking indicates the default storage backing available for this library in vCenter.
	// +required
	StorageBacking StorageBacking `json:"storageBacking"`

	// Version is a number that can identify metadata changes. This integer value is incremented when the library
	// properties such as name or description are changed in vCenter.
	// +required
	Version string `json:"version"`

	// Published indicates how the library is published so that it can be subscribed to by a remote subscribed library.
	// +optional
	PublishInfo *PublishInfo `json:"publishInfo,omitempty"`

	// SubscriptionInfo defines how the subscribed library synchronizes to a remote source.
	// This field is populated only if the library is of the "Subscribed" type.
	// +optional
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`

	// CreationTime indicates the date and time when this library was created.
	// +required
	CreationTime string `json:"creationTime"`

	// LastModifiedTime indicates the date and time when this library was last updated.
	// This field is updated only when the library properties are changed. This field is not updated when a library
	// item is added, modified or deleted or its content is changed.
	// +required
	LastModifiedTime string `json:"lastModifiedTime"`

	// LastSyncTime indicates the date and time when this library was last synchronized.
	// This field applies only if the library is of the "Subscribed" Type.
	// +optional
	LastSyncTime string `json:"lastSyncTime,omitempty"`

	// Conditions describes the current condition information of the ClusterContentLibrary.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

func (ccl *ClusterContentLibrary) GetConditions() Conditions {
	return ccl.Status.Conditions
}

func (ccl *ClusterContentLibrary) SetConditions(conditions Conditions) {
	ccl.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=ccl
// +kubebuilder:printcolumn:name="vSphereName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
// +kubebuilder:printcolumn:name="StorageType",type="string",JSONPath=".status.storageBacking.type"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterContentLibrary is the schema for the cluster scoped content library API.
// Currently, ClusterContentLibrary is immutable to end users.
type ClusterContentLibrary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterContentLibrarySpec   `json:"spec,omitempty"`
	Status ClusterContentLibraryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterContentLibraryList contains a list of ClusterContentLibrary.
type ClusterContentLibraryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterContentLibrary `json:"items"`
}

func init() {
	RegisterTypeWithScheme(&ClusterContentLibrary{}, &ClusterContentLibraryList{})
}
