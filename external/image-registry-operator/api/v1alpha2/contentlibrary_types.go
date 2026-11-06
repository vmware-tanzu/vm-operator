// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=Datastore;Other

// StorageBackingType is a constant type that indicates the type of the storage
// backing for a content library in vCenter.
type StorageBackingType string

const (
	// StorageBackingTypeDatastore indicates a content library backed by a
	// datastore.
	StorageBackingTypeDatastore StorageBackingType = "Datastore"

	// StorageBackingTypeOther indicates a content library backed by an NFS or
	// SMB file system.
	StorageBackingTypeOther StorageBackingType = "Other"
)

// +kubebuilder:validation:Enum=Active;InMaintenance

// State is a constant type that indicates the current state of a content
// library in vCenter.
type State string

const (
	// StateActive indicates the library state when the library should be fully
	// functional, this is the default library state when a library is created.
	StateActive State = "Active"

	// StateInMaintenance indicates the library state when the library is in
	// maintenance. This can happen when the library is storage migrated to a
	// different datastore, in which case content from the library may not be
	// accessible and operations mutating library content will be disallowed.
	StateInMaintenance State = "InMaintenance"
)

// StorageBacking describes the default storage backing which is available for
// the library.
type StorageBacking struct {
	// Type indicates the type of storage where the content would be stored.
	Type StorageBackingType `json:"type"`

	// +optional

	// DatastoreID indicates the identifier of the datastore used to store the
	// content in the library for the "Datastore" storageType in vCenter.
	DatastoreID string `json:"datastoreID,omitempty"`
}

// +kubebuilder:validation:Enum=FromItemID;PreferItemSourceID

// ResourceNamingStrategy represents a naming strategy for item resources in a
// content library in vCenter.
type ResourceNamingStrategy string

const (
	// ResourceNamingStrategyFromItemID indicates the naming strategy that
	// generates the item resource name from the item identifier for items in a
	// content library. This is the default naming strategy if not specified on
	// a content library.
	ResourceNamingStrategyFromItemID ResourceNamingStrategy = "FromItemID"

	// ResourceNamingStrategyPreferItemSourceID indicates the naming strategy
	// that generates the item resource name from the source identifier of the
	// item if it belongs to a subscribed content library, otherwise the item
	// resource name will be generated from the item identifier for items in a
	// content library.
	ResourceNamingStrategyPreferItemSourceID ResourceNamingStrategy = "PreferItemSourceID"
)

// SubscriptionInfo defines how the subscribed library synchronizes to a remote
// source.
type SubscriptionInfo struct {
	// URL of the endpoint where the metadata for the remotely published library
	// is being served.
	// The value from PublishInfo.URL of the published library should be used
	// while creating a subscribed library.
	URL string `json:"url"`

	// OnDemand indicates whether a library item’s content will be synchronized
	// only on demand.
	OnDemand bool `json:"onDemand"`

	// AutomaticSync indicates whether the library should participate in
	// automatic library synchronization.
	AutomaticSync bool `json:"automaticSync"`
}

// PublishInfo defines how the library is published so that it can be subscribed
// to by a remote subscribed library.
type PublishInfo struct {
	// Published indicates if the local library is published so that it can be
	// subscribed to by a remote subscribed library.
	Published bool `json:"published"`

	// URL to which the library metadata is published by the vSphere Content
	// Library Service.
	// This value can be used to set the SubscriptionInfo.URL property when
	// creating a subscribed library.
	URL string `json:"url"`
}

// +kubebuilder:validation:Enum=ContentLibrary;Inventory

// LibraryType defines the types of libraries.
type LibraryType string

const (
	// LibraryTypeContentLibrary describes a classic vCenter content library
	// whose library items are surfaced as ContentLibraryItem resources.
	LibraryTypeContentLibrary LibraryType = "ContentLibrary"

	// LibraryTypeInventory describes a folder in the vCenter inventory whose
	// virtual machines are surfaced as ContentLibraryItem resources.
	LibraryTypeInventory LibraryType = "Inventory"
)

type BaseContentLibrarySpec struct {
	// ID describes the unique identifier used to find the library in vCenter.
	//
	// Please note this value may differ depending on spec.type:
	// - Type=ContentLibrary -- ID is a content library UUID.
	// - Type=Inventory      -- ID is a vSphere folder managed object ID.
	ID string `json:"id"`

	// +optional
	// +kubebuilder:default=ContentLibrary

	// Type describes the type of library.
	//
	// Defaults to ContentLibrary.
	Type LibraryType `json:"libraryType,omitempty"`

	// +optional
	// +kubebuilder:default=FromItemID

	// ResourceNamingStrategy describes the naming strategy for item resources
	// in this content library.
	//
	// This field is immutable and defaults to FromItemID.
	//
	// Please note, this is optional and not present on all libraries.
	ResourceNamingStrategy ResourceNamingStrategy `json:"resourceNamingStrategy,omitempty"`
}

// ContentLibrarySpec defines the desired state of a ContentLibrary.
type ContentLibrarySpec struct {
	BaseContentLibrarySpec `json:",inline"`

	// +optional

	// StorageClass describes the name of the StorageClass used when publishing
	// images to this library.
	//
	// Please note, this is optional and not present on all libraries.
	StorageClass string `json:"storageClass,omitempty"`

	// +optional

	// AllowDelete describes whether or not it is possible to delete items from
	// this library.
	AllowDelete bool `json:"allowDelete,omitempty"`

	// +optional

	// AllowImport describes whether or not it is possible to import remote
	// OVA/OVF/ISO content to this library.
	AllowImport bool `json:"allowImport,omitempty"`

	// +optional

	// AllowPublish describes whether or not it is possible to publish new items
	// to this library.
	AllowPublish bool `json:"allowPublish,omitempty"`
}

// ContentLibraryStatus defines the observed state of ContentLibrary.
type ContentLibraryStatus struct {
	// +optional

	// Name describes the display name for the library.
	Name string `json:"name,omitempty"`

	// +optional

	// Description describes a human-readable description for the library.
	Description string `json:"description,omitempty"`

	// +optional

	// StorageBackings describes the default storage backing for the library.
	StorageBackings []StorageBacking `json:"storageBacking,omitempty"`

	// +optional

	// Version describes an optional value that tracks changes to the library's
	// metadata, such as its name or description.
	//
	// Please note, this is optional and not present on all libraries.
	Version string `json:"version,omitempty"`

	// +optional

	// PublishInfo describes how the library is published.
	//
	// Please note, this is only applicable for published libraries.
	PublishInfo *PublishInfo `json:"publishInfo,omitempty"`

	// +optional

	// SubscriptionInfo describes how the library is subscribed.
	//
	// This field is only present for subscribed libraries.
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`

	// +optional

	// SecurityPolicyID describes the security policy applied to the library.
	//
	// Please note, this is optional and not present on all libraries.
	SecurityPolicyID string `json:"securityPolicyID,omitempty"`

	// +optional

	// State describes the current state of the library.
	State State `json:"state,omitempty"`

	// +optional

	// ServerGUID describes the unique identifier of the vCenter server where
	// the library exists.
	ServerGUID string `json:"serverGUID,omitempty"`

	// +optional

	// CreationTime describes the date and time when this library was created
	// in vCenter.
	CreationTime metav1.Time `json:"creationTime,omitempty"`

	// +optional

	// LastModifiedTime describes the date and time when the library was last
	// updated.
	// This field is updated only when the library properties are changed.
	// This field is not updated when a library item is added, modified,
	// deleted, or its content is changed.
	LastModifiedTime metav1.Time `json:"lastModifiedTime,omitempty"`

	// +optional

	// LastSyncTime describes the date and time when this library was last
	// synchronized.
	//
	// Please note, this is only applicable for subscribed libraries.
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`

	// +optional

	// Conditions describes the current condition information of the library.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (contentLibrary ContentLibrary) GetConditions() []metav1.Condition {
	return contentLibrary.Status.Conditions
}

func (contentLibrary *ContentLibrary) SetConditions(conditions []metav1.Condition) {
	contentLibrary.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=lib;library
// +kubebuilder:printcolumn:name="DisplayName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="AllowPublish",type="boolean",JSONPath=".spec.allowPublish"
// +kubebuilder:printcolumn:name="AllowImport",type="boolean",JSONPath=".spec.allowImport"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ContentLibrary is the schema for the content library API.
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
	BaseContentLibrarySpec `json:",inline"`
}

func (ccl ClusterContentLibrary) GetConditions() []metav1.Condition {
	return ccl.Status.Conditions
}

func (ccl *ClusterContentLibrary) SetConditions(conditions []metav1.Condition) {
	ccl.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,shortName=clib;clibrary
// +kubebuilder:printcolumn:name="DisplayName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterContentLibrary is the schema for the cluster scoped content library
// API.
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
	objectTypes = append(
		objectTypes,
		&ContentLibrary{},
		&ContentLibraryList{},
		&ClusterContentLibrary{},
		&ClusterContentLibraryList{},
	)
}
