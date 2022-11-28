// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContentLibraryItemType is a constant for the type of a content library item in vCenter.
type ContentLibraryItemType string

// CertVerificationStatus is a constant for the certificate verification status of a content library item in vCenter.
type CertVerificationStatus string

const (
	// ContentLibraryItemTypeOvf indicates an OVF content library item in vCenter.
	ContentLibraryItemTypeOvf = ContentLibraryItemType("Ovf")

	// ContentLibraryItemTypeIso indicates an ISO content library item in vCenter.
	ContentLibraryItemTypeIso = ContentLibraryItemType("Iso")

	// CertVerificationStatusNotAvailable indicates the certificate verification status is not available.
	CertVerificationStatusNotAvailable = CertVerificationStatus("NOT_AVAILABLE")

	// CertVerificationStatusVerified indicates the library item has been fully validated during importing or file syncing.
	CertVerificationStatusVerified = CertVerificationStatus("VERIFIED")

	// CertVerificationStatusInternal indicates the library item is cloned/created through vCenter.
	CertVerificationStatusInternal = CertVerificationStatus("INTERNAL")

	// CertVerificationStatusVerificationFailure indicates certificate or manifest validation failed on the library item.
	CertVerificationStatusVerificationFailure = CertVerificationStatus("VERIFICATION_FAILURE")

	// CertVerificationStatusVerificationInProgress indicates the library item certificate verification is in progress.
	CertVerificationStatusVerificationInProgress = CertVerificationStatus("VERIFICATION_IN_PROGRESS")

	// CertVerificationStatusUntrusted indicates the certificate used to sign the library item is not trusted.
	CertVerificationStatusUntrusted = CertVerificationStatus("UNTRUSTED")
)

// ContentLibraryReference contains the information to locate the content library resource.
type ContentLibraryReference struct {
	// Name is the name of resource being referenced.
	// +required
	Name string `json:"name"`

	// Namespace of the resource being referenced. If empty, cluster scoped resource is assumed.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

type CertificateVerificationInfo struct {
	// Status shows the certificate verification status of the library item.
	Status CertVerificationStatus `json:"status"`

	// CertChain shows the signing certificate in base64 encoding if the library item is signed.
	// +optional
	CertChain []string `json:"certChain,omitempty"`
}

// ContentLibraryItemSpec defines the desired state of a ContentLibraryItem.
type ContentLibraryItemSpec struct {
	// UUID is the identifier which uniquely identifies the library item in vCenter. This field is immutable.
	// +required
	UUID string `json:"uuid"`
}

// ContentLibraryItemStatus defines the observed state of ContentLibraryItem.
type ContentLibraryItemStatus struct {
	// Name specifies the name of the content library item in vCenter specified by the user.
	// +required
	Name string `json:"name"`

	// ContentLibraryRef refers to the ContentLibrary custom resource that this item belongs to.
	// +required
	ContentLibraryRef ContentLibraryReference `json:"contentLibraryRef"`

	// Description is a human-readable description for this library item.
	// +optional
	Description string `json:"description,omitempty"`

	// MetadataVersion indicates the version of the library item metadata.
	// This integer value is incremented when the library item properties such as name or description are changed in vCenter.
	// +required
	MetadataVersion string `json:"metadataVersion"`

	// ContentVersion indicates the version of the library item content.
	// This integer value is incremented when the files comprising the content library item are changed in vCenter.
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

	// SecurityCompliance shows the security compliance of the library item.
	// +optional
	SecurityCompliance *bool `json:"securityCompliance,omitempty"`

	// CertificateVerificationInfo shows the certificate verification status and the signing certificate.
	// +optional
	CertificateVerificationInfo *CertificateVerificationInfo `json:"certificateVerificationInfo,omitempty"`

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

	// Conditions describes the current condition information of the ContentLibraryItem.
	// +optional
	Conditions Conditions `json:"conditions,omitempty"`
}

func (contentLibraryItem *ContentLibraryItem) GetConditions() Conditions {
	return contentLibraryItem.Status.Conditions
}

func (contentLibraryItem *ContentLibraryItem) SetConditions(conditions Conditions) {
	contentLibraryItem.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=clitem
// +kubebuilder:printcolumn:name="vSphereName",type="string",JSONPath=".status.name"
// +kubebuilder:printcolumn:name="ContentLibraryRef",type="string",JSONPath=".status.contentLibraryRef.name"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".status.type"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ContentLibraryItem is the schema for the content library item API.
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

func init() {
	RegisterTypeWithScheme(&ContentLibraryItem{}, &ContentLibraryItemList{})
}
