// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ContentLibraryType is a constant type that indicates the type of a content library in vCenter.
type ContentLibraryType string

const (
	// ContentLibraryTypeLocal indicates a local content library in vCenter.
	ContentLibraryTypeLocal ContentLibraryType = "Local"

	// ContentLibraryTypeSubscribed indicates a subscribed content library in vCenter.
	ContentLibraryTypeSubscribed ContentLibraryType = "Subscribed"
)

// StorageBackingType is a constant type that indicates the type of the storage backing for a content library in vCenter.
type StorageBackingType string

const (
	// StorageBackingTypeDatastore indicates a datastore backed content library in vCenter.
	StorageBackingTypeDatastore StorageBackingType = "Datastore"

	// StorageBackingTypeOther indicates a remote file system backed content library in vCenter.
	// Supports NFS and SMB remote file systems.
	StorageBackingTypeOther StorageBackingType = "Other"
)

// StorageBacking describes the default storage backing which is available for the library.
type StorageBacking struct {
	// Type indicates the type of storage where the content would be stored.
	// +kubebuilder:validation:Enum=Datastore;Other
	// +required
	Type StorageBackingType `json:"type"`

	// DatastoreID indicates the identifier of the datastore used to store the content
	// in the library for the "Datastore" storageType in vCenter.
	// +optional
	DatastoreID string `json:"datastoreID,omitempty"`
}

// SubscriptionInfo defines how the subscribed library synchronizes to a remote source.
type SubscriptionInfo struct {
	// URL of the endpoint where the metadata for the remotely published library is being served.
	// The value from PublishInfo.URL of the published library should be used while creating a subscribed library.
	// +required
	URL string `json:"URL"`

	// OnDemand indicates whether a library item’s content will be synchronized only on demand.
	// +required
	OnDemand bool `json:"onDemand"`

	// AutomaticSync indicates whether the library should participate in automatic library synchronization.
	// +required
	AutomaticSync bool `json:"automaticSync"`
}

// PublishInfo defines how the library is published so that it can be subscribed to by a remote subscribed library.
type PublishInfo struct {
	// Published indicates if the local library is published so that it can be subscribed to by a remote subscribed library.
	// +required
	Published bool `json:"published"`

	// URL to which the library metadata is published by the vSphere Content Library Service.
	// This value can be used to set the SubscriptionInfo.URL property when creating a subscribed library.
	// +required
	URL string `json:"URL"`
}

// ContentLibrarySpec defines the desired state of a ContentLibrary.
type ContentLibrarySpec struct {
	// UUID is the identifier which uniquely identifies the library in vCenter. This field is immutable.
	// +required
	UUID types.UID `json:"uuid"`

	// Writable flag indicates if users can create new library items in this library.
	// +required
	Writable bool `json:"writable"`

	// AllowImport flag indicates if users can import OVF/OVA templates from remote HTTPS URLs
	// as new content library items in this library.
	// +optional
	AllowImport bool `json:"allowImport,omitempty"`
}

// ContentLibraryStatus defines the observed state of ContentLibrary.
type ContentLibraryStatus struct {
	// Name specifies the name of the content library in vCenter.
	// +optional
	Name string `json:"name,omitempty"`

	// Description is a human-readable description for this library in vCenter.
	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Enum=Local;Subscribed
	// +optional
	Type ContentLibraryType `json:"type,omitempty"`

	// StorageBacking indicates the default storage backing available for this library in vCenter.
	// +optional
	StorageBacking *StorageBacking `json:"storageBacking,omitempty"`

	// Version is a number that can identify metadata changes. This value is incremented when the library
	// properties such as name or description are changed in vCenter.
	// +optional
	Version string `json:"version,omitempty"`

	// Published indicates how the library is published so that it can be subscribed to by a remote subscribed library.
	// +optional
	PublishInfo *PublishInfo `json:"publishInfo,omitempty"`

	// SubscriptionInfo defines how the subscribed library synchronizes to a remote source.
	// This field is populated only if Type=Subscribed.
	// +optional
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`

	// SecurityPolicyID defines the security policy applied to this library.
	// Setting this field will make the library secure.
	// +optional
	SecurityPolicyID string `json:"securityPolicyID,omitempty"`

	// CreationTime indicates the date and time when this library was created in vCenter.
	// +optional
	CreationTime metav1.Time `json:"creationTime,omitempty"`

	// LastModifiedTime indicates the date and time when this library was last updated in vCenter.
	// This field is updated only when the library properties are changed. This field is not updated when a library
	// item is added, modified or deleted or its content is changed.
	// +optional
	LastModifiedTime metav1.Time `json:"lastModifiedTime,omitempty"`

	// LastSyncTime indicates the date and time when this library was last synchronized in vCenter.
	// This field applies only if the library is of the "Subscribed" Type.
	// +optional
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`

	// Conditions describes the current condition information of the ContentLibrary.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

func (contentLibrary *ContentLibrary) GetConditions() Conditions {
	return contentLibrary.Status.Conditions
}

func (contentLibrary *ContentLibrary) SetConditions(conditions Conditions) {
	contentLibrary.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=cl
// +kubebuilder:printcolumn:name="vSphereName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
// +kubebuilder:printcolumn:name="Writable",type="boolean",JSONPath=".spec.writable"
// +kubebuilder:printcolumn:name="StorageType",type="string",JSONPath=".status.storageBacking.type"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ContentLibrary is the schema for the content library API.
// Currently, ContentLibrary is immutable to end users.
type ContentLibrary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContentLibrarySpec   `json:"spec,omitempty"`
	Status ContentLibraryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContentLibraryList contains a list of ContentLibrary.
type ContentLibraryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContentLibrary `json:"items"`
}

// ClusterContentLibrarySpec defines the desired state of a ClusterContentLibrary.
type ClusterContentLibrarySpec struct {
	// UUID is the identifier which uniquely identifies the library in vCenter. This field is immutable.
	// +required
	UUID types.UID `json:"uuid"`
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

	Spec   ClusterContentLibrarySpec `json:"spec,omitempty"`
	Status ContentLibraryStatus      `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterContentLibraryList contains a list of ClusterContentLibrary.
type ClusterContentLibraryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterContentLibrary `json:"items"`
}

func init() {
	RegisterTypeWithScheme(
		&ContentLibrary{},
		&ContentLibraryList{},
		&ClusterContentLibrary{},
		&ClusterContentLibraryList{})
}
