// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:validation:Enum=Waiting;Transferring;Validating;Ready;Error

// TransferStatus is a constant that indicates the transfer state of a file.
type TransferStatus string

const (
	// TransferStatusWaiting indicates that the file is waiting to be
	// transferred.
	TransferStatusWaiting TransferStatus = "Waiting"

	// TransferStatusTransferring indicates that the data of the file is being
	// transferred.
	TransferStatusTransferring TransferStatus = "Transferring"

	// TransferStatusValidating indicates that the file is being validated.
	TransferStatusValidating TransferStatus = "Validating"

	// TransferStatusReady indicates that the file has been fully transferred
	// and is ready to be used.
	TransferStatusReady TransferStatus = "Ready"

	// TransferStatusError indicates there was an error transferring or
	// validating the file.
	TransferStatusError TransferStatus = "Error"
)

// ContentLibraryItemImportRequestSource contains the specification of the
// source for the import request.
type ContentLibraryItemImportRequestSource struct {
	// URL is the endpoint that points to a file that is to be imported as a new
	// Content Library Item in the target vSphere Content Library. If the target
	// item type is ContentLibraryItemTypeOvf, the URL should point to an OVF
	// descriptor file (.ovf), an OVA file (.ova), or an ISO file (.iso).
	// Otherwise, the SourceValid condition will become false in the status.
	URL string `json:"url"`

	// +optional

	// PEM encoded SSL Certificate for this endpoint specified by the URL. It is
	// only used for HTTPS connections.
	// If set, the remote endpoint's SSL certificate is only accepted if it
	// matches this certificate, and no other certificate validation is
	// performed.
	// If unset, the remote endpoint's SSL certificate must be trusted by
	// vSphere trusted root CA certificates, otherwise the SSL certification
	// verification may fail and thus fail the import request.
	SSLCertificate string `json:"sslCertificate,omitempty"`

	// +optional

	// Checksum contains the checksum algorithm and value calculated for the
	// file specified in the URL. If omitted, the import request will not verify
	// the checksum of the file.
	Checksum *Checksum `json:"checksum,omitempty"`
}

// Checksum contains the checksum value and algorithm used to calculate that
// value.
type Checksum struct {
	// +optional
	// +kubebuilder:validation:Enum=SHA256;SHA512
	// +kubebuilder:default=SHA256

	// Algorithm is the algorithm used to calculate the checksum. Supported
	// algorithms are "SHA256" and "SHA512". If omitted, "SHA256" will be used
	// as the default algorithm.
	Algorithm string `json:"algorithm"`

	// Value is the checksum value calculated by the specified algorithm.
	Value string `json:"value"`
}

// ContentLibraryItemImportRequestTargetItem contains the specification of the
// target content library item for the import request.
type ContentLibraryItemImportRequestTargetItem struct {
	// +optional

	// Name is the name of the new content library item that will be created
	// in vSphere.
	// If omitted, the content library item will be created with the same name
	// as the name of the image specified in the spec.source.url in the
	// specified library.
	// If an item with the same name already exists in the specified library,
	// the TargetValid condition will become false in the
	// status.
	Name string `json:"name,omitempty"`

	// +optional

	// Description is a description for a library item.
	Description string `json:"description,omitempty"`

	// +optional

	// Type is the type of the new library item that will be created.
	//
	// The valid types depend on the type of underlying library:
	// - LibraryType=ContentLibrary -- OVF
	// - LibraryType=ContentLibrary -- ISO
	//
	// If omitted or the type is invalid, the TargetValid condition will be
	// false.
	//
	// For the item type OVF, the default OVF security policy must be configured
	// on the target library, otherwise the TargetValid condition will be false.
	Type ContentLibraryItemType `json:"type,omitempty"`
}

// ContentLibraryItemImportRequestTarget is the target specification of an
// import request.
type ContentLibraryItemImportRequestTarget struct {
	// +optional

	// Item contains information about the library item to which the item will
	// be imported in vSphere.
	//
	// If omitted, the library item will be created with the same name as the
	// name of the image specified in the spec.source.url in the specified
	// library.
	//
	// If an item with the same name already exists in the specified library,
	// the TargetValid condition will be false.
	Item ContentLibraryItemImportRequestTargetItem `json:"item,omitempty"`

	// Library describes the name of the library in which the item will be
	// created.
	LibraryName string `json:"library"`
}

// ContentLibraryItemImportRequestSpec defines the desired state of a
// ContentLibraryItemImportRequest.
type ContentLibraryItemImportRequestSpec struct {
	// Source is the source of the import request which includes an external URL
	// pointing to a VM image template.
	// Source and Target will be immutable if the SourceValid and TargetValid
	// conditions are true.
	Source ContentLibraryItemImportRequestSource `json:"source"`

	// Target is the target of the import request which includes the content
	// library item information and a ContentLibrary resource.
	// Source and Target will be immutable if the SourceValid and TargetValid
	// conditions are true.
	Target ContentLibraryItemImportRequestTarget `json:"target"`

	// +optional
	// +kubebuilder:validation:Minimum=0

	// TTLSecondsAfterFinished is the time-to-live duration for how long this
	// resource will be allowed to exist once the import operation
	// completes. After the TTL expires, the resource will be automatically
	// deleted without the user having to take any direct action.
	// If this field is unset then the request resource will not be
	// automatically deleted. If this field is set to zero then the request
	// resource is eligible for deletion immediately after it finishes.
	TTLSecondsAfterFinished *int64 `json:"ttlSecondsAfterFinished,omitempty"`
}

// ContentLibraryItemFileUploadStatus indicates the upload status of files
// belonging to the template.
type ContentLibraryItemFileUploadStatus struct {
	// SessionUUID is the identifier that uniquely identifies the file upload
	// session on the library item in vSphere.
	SessionUUID types.UID `json:"sessionUUID,omitempty"`

	// +optional

	// FileUploads list the transfer statuses of files being uploaded and
	// tracked by the upload session.
	FileUploads []FileTransferStatus `json:"fileUploads,omitempty"`
}

// FileTransferStatus indicates the transfer status of a file belonging to a
// library item.
type FileTransferStatus struct {
	// Name specifies the name of the file that is transferred.
	Name string `json:"name"`

	// Status indicates the transfer status of the file.
	Status TransferStatus `json:"transferStatus"`

	// +optional

	// BytesTransferred indicates the number of bytes of this file that have
	// been received by the server.
	BytesTransferred *int64 `json:"bytesTransferred,omitempty"`

	// +optional

	// Size indicates the file size in bytes as received by the server.
	// This value will not be available until the transfer status is ready.
	Size *int64 `json:"size,omitempty"`

	// +optional

	// ErrorMessage describes the details about the transfer error if the
	// transfer status is error.
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// ContentLibraryItemImportRequestStatus defines the observed state of a
// ContentLibraryItemImportRequest.
type ContentLibraryItemImportRequestStatus struct {
	// +optional

	// ItemName is the name to the target ContentLibraryItem resource created as
	// a result of the import.
	// If the ContentLibraryItemImportRequest is deleted when the import
	// operation fails or before the Complete condition is set to true, the
	// import operation will be cancelled in vSphere and the corresponding
	// vSphere Content Library Item will be deleted.
	ItemName string `json:"itemName,omitempty"`

	// +optional

	// CompletionTime represents time when the request was completed.
	// The value of this field should be equal to the value of the
	// LastTransitionTime for the status condition Type=Complete.
	CompletionTime metav1.Time `json:"completionTime,omitempty"`

	// +optional

	// StartTime represents time when the request was acknowledged by the
	// controller.
	StartTime metav1.Time `json:"startTime,omitempty"`

	// +optional

	// FileUpload indicates the upload status of files belonging to the template.
	FileUploadStatus *ContentLibraryItemFileUploadStatus `json:"fileUploadStatus,omitempty"`

	// +optional

	// Conditions describes the current condition information of the
	// ContentLibraryItemImportRequest.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (clItemImportRequest ContentLibraryItemImportRequest) GetConditions() []metav1.Condition {
	return clItemImportRequest.Status.Conditions
}

func (clItemImportRequest *ContentLibraryItemImportRequest) SetConditions(conditions []metav1.Condition) {
	clItemImportRequest.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=libitemimport
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="ContentLibraryRef",type="string",JSONPath=".spec.target.libraryName"
// +kubebuilder:printcolumn:name="ContentLibraryItemRef",type="string",JSONPath=".status.itemName"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(.type=='Complete')].status"

// ContentLibraryItemImportRequest defines the information necessary to import a VM image
// template as a ContentLibraryItem to a Content Library in vSphere.
type ContentLibraryItemImportRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ContentLibraryItemImportRequestSpec   `json:"spec,omitempty"`
	Status ContentLibraryItemImportRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ContentLibraryItemImportRequestList contains a list of
// ContentLibraryItemImportRequest resources.
type ContentLibraryItemImportRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ContentLibraryItemImportRequest `json:"items"`
}

func init() {
	objectTypes = append(
		objectTypes,
		&ContentLibraryItemImportRequest{},
		&ContentLibraryItemImportRequestList{},
	)
}
