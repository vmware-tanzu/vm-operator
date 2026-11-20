// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TransferStatus is a constant that indicates the transfer state of a file.
type TransferStatus string

const (
	// TransferStatusWaiting indicates that the file is waiting to be transferred.
	TransferStatusWaiting TransferStatus = "waiting"

	// TransferStatusTransferring indicates that the data of the file is being transferred.
	TransferStatusTransferring TransferStatus = "transferring"

	// TransferStatusValidating indicates that the file is being validated.
	TransferStatusValidating TransferStatus = "validating"

	// TransferStatusReady indicates that the file has been fully transferred and is ready to be used.
	TransferStatusReady TransferStatus = "ready"

	// TransferStatusError indicates there was an error transferring or validating the file.
	TransferStatusError TransferStatus = "error"
)

// ContentLibraryItemImportRequestSource contains the specification of the source for the import request.
type ContentLibraryItemImportRequestSource struct {
	// +required

	// URL is the endpoint that points to a file that is to be imported as a new Content Library Item in
	// the target vSphere Content Library. If the target item type is ContentLibraryItemTypeOvf, the URL
	// should point to an OVF descriptor file (.ovf), an OVA file (.ova), or an ISO file (.iso). Otherwise,
	// the SourceValid condition will become false in the status.
	URL string `json:"url"`

	// +optional

	// PEM encoded SSL Certificate for this endpoint specified by the URL. It is only used for HTTPS connections.
	// If set, the remote endpoint's SSL certificate is only accepted if it matches this certificate, and no other
	// certificate validation is performed.
	// If unset, the remote endpoint's SSL certificate must be trusted by vSphere trusted root CA certificates,
	// otherwise the SSL certification verification may fail and thus fail the import request.
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

	// +required

	// Value is the checksum value calculated by the specified algorithm.
	Value string `json:"value"`
}

// ContentLibraryItemImportRequestTargetItem contains the specification of the target
// content library item for the import request.
type ContentLibraryItemImportRequestTargetItem struct {
	// Name is the name of the new content library item that will be created in vSphere.
	// If omitted, the content library item will be created with the same name as the name
	// of the image specified in the spec.source.url in the specified vSphere Content Library.
	// If an item with the same name already exists in the specified vSphere Content Library,
	// the TargetValid condition will become false in the status.
	// +optional
	Name string `json:"name,omitempty"`

	// Description is a description for a vSphere Content Library Item.
	// +optional
	Description string `json:"description,omitempty"`

	// Type is the type of the new content library item that will be created in vSphere.
	// Currently only ContentLibraryItemTypeOvf is supported, if it is omitted or other item type
	// is specified, the TargetValid condition will become false in the status. For the item type
	// of ContentLibraryItemTypeOvf, it is required that the default OVF security policy is configured
	// on the target content library for the import request, otherwise the TargetValid condition will
	// become false in the status.
	// +optional
	Type ContentLibraryItemType `json:"type,omitempty"`
}

// ContentLibraryItemImportRequestTarget is the target specification of an import request.
type ContentLibraryItemImportRequestTarget struct {
	// Item contains information about the content library item to which
	// the template will be imported in vSphere.
	// If omitted, the content library item will be created with the same name as the name
	// of the image specified in the spec.source.url in the specified vSphere Content Library.
	// If an item with the same name already exists in the specified vSphere Content Library,
	// the TargetValid condition will become false in the status.
	// +optional
	Item ContentLibraryItemImportRequestTargetItem `json:"item,omitempty"`

	// Library contains information about the library in which the library item
	// will be created in vSphere.
	// +required
	Library LocalObjectRef `json:"library"`
}

// ContentLibraryItemImportRequestSpec defines the desired state of a
// ContentLibraryItemImportRequest.
type ContentLibraryItemImportRequestSpec struct {
	// Source is the source of the import request which includes an external URL
	// pointing to a VM image template.
	// Source and Target will be immutable if the SourceValid and TargetValid conditions are true.
	// +required
	Source ContentLibraryItemImportRequestSource `json:"source"`

	// Target is the target of the import request which includes the content library item
	// information and a ContentLibrary resource.
	// Source and Target will be immutable if the SourceValid and TargetValid conditions are true.
	// +required
	Target ContentLibraryItemImportRequestTarget `json:"target"`

	// TTLSecondsAfterFinished is the time-to-live duration for how long this
	// resource will be allowed to exist once the import operation
	// completes. After the TTL expires, the resource will be automatically
	// deleted without the user having to take any direct action.
	// If this field is unset then the request resource will not be
	// automatically deleted. If this field is set to zero then the request
	// resource is eligible for deletion immediately after it finishes.
	// +optional
	// +kubebuilder:validation:Minimum=0
	TTLSecondsAfterFinished *int64 `json:"ttlSecondsAfterFinished,omitempty"`
}

// ContentLibraryItemFileUploadStatus indicates the upload status of files belonging to the template.
type ContentLibraryItemFileUploadStatus struct {
	// SessionUUID is the identifier that uniquely identifies the file upload session on the library item in vSphere.
	// +required
	SessionUUID types.UID `json:"sessionUUID,omitempty"`

	// FileUploads list the transfer statuses of files being uploaded and tracked by the upload session.
	// +optional
	FileUploads []FileTransferStatus `json:"fileUploads,omitempty"`
}

// FileTransferStatus indicates the transfer status of a file belonging to a library item.
type FileTransferStatus struct {
	// Name specifies the name of the file that is transferred.
	// +required
	Name string `json:"name"`

	// Status indicates the transfer status of the file.
	// +required
	Status TransferStatus `json:"transferStatus"`

	// BytesTransferred indicates the number of bytes of this file that have been received by the server.
	// +optional
	BytesTransferred *int64 `json:"bytesTransferred,omitempty"`

	// Size indicates the file size in bytes as received by the server, this won't be available
	// until the transfer status is ready.
	// +optional
	Size *int64 `json:"size,omitempty"`

	// ErrorMessage describes the details about the transfer error if the transfer status is error.
	// +optional
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// ContentLibraryItemImportRequestStatus defines the observed state of a
// ContentLibraryItemImportRequest.
type ContentLibraryItemImportRequestStatus struct {
	// ItemRef is the reference to the target ContentLibraryItem resource of the import request.
	// If the ContentLibraryItemImportRequest is deleted when the import operation fails or before
	// the Complete condition is set to true, the import operation will be cancelled in vSphere
	// and the corresponding vSphere Content Library Item will be deleted.
	// +optional
	ItemRef *LocalObjectRef `json:"itemRef,omitempty"`

	// CompletionTime represents time when the request was completed.
	// The value of this field should be equal to the value of the
	// LastTransitionTime for the status condition Type=Complete.
	// +optional
	CompletionTime metav1.Time `json:"completionTime,omitempty"`

	// StartTime represents time when the request was acknowledged by the
	// controller.
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`

	// FileUpload indicates the upload status of files belonging to the template.
	// +optional
	FileUploadStatus *ContentLibraryItemFileUploadStatus `json:"fileUploadStatus,omitempty"`

	// Conditions describes the current condition information of the ContentLibraryItemImportRequest.
	// The conditions present will be:
	//   * SourceValid
	//   * TargetValid
	//   * ContentLibraryItemCreated
	//   * TemplateUploaded
	//   * ContentLibraryItemReady
	//   * Complete
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

func (clItemImportRequest *ContentLibraryItemImportRequest) GetConditions() Conditions {
	return clItemImportRequest.Status.Conditions
}

func (clItemImportRequest *ContentLibraryItemImportRequest) SetConditions(conditions Conditions) {
	clItemImportRequest.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=libitemimport
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ContentLibraryRef",type="string",JSONPath=".spec.target.library.name"
// +kubebuilder:printcolumn:name="ContentLibraryItemRef",type="string",JSONPath=".status.itemRef.name"
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
