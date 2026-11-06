// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=OVF;ISO;VM

// ContentLibraryItemType describes the type of library item.
type ContentLibraryItemType string

const (
	// ContentLibraryItemTypeOvf describes an OVF library item.
	ContentLibraryItemTypeOvf ContentLibraryItemType = "OVF"

	// ContentLibraryItemTypeIso describes an ISO library item.
	ContentLibraryItemTypeIso ContentLibraryItemType = "ISO"

	// ContentLibraryItemTypeVM describes a VM library item.
	ContentLibraryItemTypeVM ContentLibraryItemType = "VM"
)

// +kubebuilder:validation:Enum=NotAvailable;Verified;Internal;VerificationFailure;VerificationInProgress;Untrusted

// CertVerificationStatus is a constant for the certificate verification status
// of a content library item.
type CertVerificationStatus string

const (
	// CertVerificationStatusNotAvailable indicates the certificate verification
	// status is not available.
	CertVerificationStatusNotAvailable CertVerificationStatus = "NotAvailable"

	// CertVerificationStatusVerified indicates the library item has been fully
	// validated during importing or file syncing.
	CertVerificationStatusVerified CertVerificationStatus = "Verified"

	// CertVerificationStatusInternal indicates the library item is
	// cloned/created through vCenter.
	CertVerificationStatusInternal CertVerificationStatus = "Internal"

	// CertVerificationStatusVerificationFailure indicates certificate or
	// manifest validation failed on the library item.
	CertVerificationStatusVerificationFailure CertVerificationStatus = "VerificationFailure"

	// CertVerificationStatusVerificationInProgress indicates the library item
	// certificate verification is in progress.
	CertVerificationStatusVerificationInProgress CertVerificationStatus = "VerificationInProgress"

	// CertVerificationStatusUntrusted indicates the certificate used to sign
	// the library item is not trusted.
	CertVerificationStatusUntrusted CertVerificationStatus = "Untrusted"
)

// CertificateVerificationInfo shows the certificate verification status and the
// signing certificate.
type CertificateVerificationInfo struct {
	// +optional

	// Status describes the certificate verification status of the library item.
	Status CertVerificationStatus `json:"status,omitempty"`

	// +optional

	// CertChain describes the signing certificate chain in base64 encoding if
	// the library item is signed.
	CertChain []string `json:"certChain,omitempty"`
}

// FileInfo represents the information of a file in a content library item in
// vCenter.
type FileInfo struct {
	// Name describes the name of the file.
	Name string `json:"name"`

	// +optional

	// SizeInBytes describes the total number of bytes used by the file on disk.
	SizeInBytes resource.Quantity `json:"sizeInBytes,omitempty"`

	// +optional

	// Version describes the version of the library item file.
	// This value is incremented when the file is modified on disk.
	Version string `json:"version,omitempty"`

	// +optional

	// Cached describes whether or not the file is available locally on disk.
	Cached bool `json:"cached,omitempty"`

	// +optional

	// StorageURI describes the fully-qualified path to the file on a datastore,
	// ex. "[my-datastore-1] library-1/item-1/file1.vmdk".
	// This URL is useful for creating a device that is backed by this file
	// (i.e. mounting an ISO file via a virtual CD-ROM device).
	StorageURI string `json:"storageURI,omitempty"`
}

// ContentLibraryItemSpec defines the desired state of a ContentLibraryItem.
type ContentLibraryItemSpec struct {
	// ID describes the unique identifier used to find the library item in
	// vCenter.
	//
	// Please note this value depends on the type of the library to which this
	// item belongs:
	// - LibraryType=ContentLibrary -- ID is a content library item UUID.
	// - LibraryType=Inventory      -- ID is a vSphere VM managed object ID.
	ID string `json:"id"`

	// +required

	// LibraryName describes the name of the library to which this item belongs.
	// Namespace-scoped items belong to namespace-scoped libraries and
	// cluster-scoped items belong to cluster-scoped libraries.
	LibraryName string `json:"libraryName"`
}

// ContentLibraryItemStatus defines the observed state of ContentLibraryItem.
type ContentLibraryItemStatus struct {
	// +optional

	// Name describes the name of the underlying item in vCenter.
	Name string `json:"name,omitempty"`

	// +optional

	// Description is a human-readable description for this library item.
	Description string `json:"description,omitempty"`

	// +optional

	// Version describes a value that tracks changes to the library item's
	// metadata, such as its name or description.
	//
	// Please note, this is optional and not present on all items.
	Version string `json:"version,omitempty"`

	// +optional

	// ContentVersion describes a value that tracks changes to the library
	// item's content.
	//
	// Please note, this is optional and not present on all items.
	ContentVersion string `json:"contentVersion,omitempty"`

	// +optional

	// Type describes the type of the library item.
	Type ContentLibraryItemType `json:"type,omitempty"`

	// +optional

	// SourceID describes a unique piece of information used to identify a
	// library item on the remote side a subscribed library.
	//
	// Please note, this is only applicable to items from subscribed libraries.
	SourceID string `json:"sourceID,omitempty"`

	// +optional

	// SizeInBytes describes the total number of bytes used by the library item
	// on disk.
	SizeInBytes resource.Quantity `json:"sizeInBytes,omitempty"`

	// +optional

	// Cached describes if the library item files are available on disk.
	Cached bool `json:"cached,omitempty"`

	// +optional

	// SecurityCompliance describes the optional security compliance of the
	// library item.
	//
	// Please note, this is optional and not present on all items.
	SecurityCompliance *bool `json:"securityCompliance,omitempty"`

	// +optional

	// CertificateVerificationInfo describes the certificate verification status
	// and signing certificate.
	//
	// Please note, this is optional and not present on all items.
	CertificateVerificationInfo *CertificateVerificationInfo `json:"certificateVerificationInfo,omitempty"`

	// +optional

	// FileInfo describes the files that belong to the library item.
	FileInfo []FileInfo `json:"fileInfo,omitempty"`

	// +optional

	// CreationTime describes when the library item was created.
	CreationTime metav1.Time `json:"creationTime,omitempty"`

	// +optional

	// LastModifiedTime describes when the library item's metadata or content
	// were last modified.
	LastModifiedTime metav1.Time `json:"lastModifiedTime,omitempty"`

	// +optional

	// LastSyncTime describes when the library item was last synchronized.
	//
	// Please note, this is only applicable to items from subscribed libraries.
	LastSyncTime metav1.Time `json:"lastSyncTime,omitempty"`

	// +optional

	// Conditions describes the current condition information of the library
	// item.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (contentLibraryItem ContentLibraryItem) GetConditions() []metav1.Condition {
	return contentLibraryItem.Status.Conditions
}

func (contentLibraryItem *ContentLibraryItem) SetConditions(conditions []metav1.Condition) {
	contentLibraryItem.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,shortName=libitem
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
// +kubebuilder:printcolumn:name="DisplayName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="LibraryName",type="string",JSONPath=".spec.libraryName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Cached",type="boolean",JSONPath=".status.cached"
// +kubebuilder:printcolumn:name="Size",type="string",JSONPath=".status.sizeInBytes"
// +kubebuilder:printcolumn:name="SecurityCompliant",type="boolean",JSONPath=".status.securityCompliance"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ContentLibraryItem is the schema for the content library item API.
// Currently, ContentLibraryItem is immutable to end users.
type ContentLibraryItem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContentLibraryItemSpec   `json:"spec,omitempty"`
	Status ContentLibraryItemStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContentLibraryItemList contains a list of ContentLibraryItem.
type ContentLibraryItemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContentLibraryItem `json:"items"`
}

func (cclItem ClusterContentLibraryItem) GetConditions() []metav1.Condition {
	return cclItem.Status.Conditions
}

func (cclItem *ClusterContentLibraryItem) SetConditions(conditions []metav1.Condition) {
	cclItem.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,shortName=clibitem
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
// +kubebuilder:printcolumn:name="DisplayName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="LibraryName",type="string",JSONPath=".status.libraryName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Cached",type="boolean",JSONPath=".status.cached"
// +kubebuilder:printcolumn:name="Size",type="string",JSONPath=".status.sizeInBytes"
// +kubebuilder:printcolumn:name="SecurityCompliant",type="boolean",JSONPath=".status.securityCompliance"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ClusterContentLibraryItem is the schema for the content library item API at the cluster scope.
// Currently, ClusterContentLibraryItem is immutable to end users.
type ClusterContentLibraryItem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContentLibraryItemSpec   `json:"spec,omitempty"`
	Status ContentLibraryItemStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterContentLibraryItemList contains a list of ClusterContentLibraryItem.
type ClusterContentLibraryItemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterContentLibraryItem `json:"items"`
}

func init() {
	objectTypes = append(
		objectTypes,
		&ContentLibraryItem{},
		&ContentLibraryItemList{},
		&ClusterContentLibraryItem{},
		&ClusterContentLibraryItemList{},
	)
}
