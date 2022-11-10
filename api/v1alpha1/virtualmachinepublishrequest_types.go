// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VirtualMachinePublishRequestConditionSuccess is the Success value for
	// conditions related to a VirtualMachinePublishRequest resource.
	VirtualMachinePublishRequestConditionSuccess = "Success"

	// VirtualMachinePublishRequestConditionSourceValid is the Type for a
	// VirtualMachinePublishRequest resource's status condition.
	//
	// The condition's status is set to true only when the information
	// that describes the source side of the publication has been validated.
	VirtualMachinePublishRequestConditionSourceValid = "SourceValid"

	// VirtualMachinePublishRequestConditionTargetValid is the Type for a
	// VirtualMachinePublishRequest resource's status condition.
	//
	// The condition's status is set to true only when the information
	// that describes the target side of the publication has been
	// validated.
	VirtualMachinePublishRequestConditionTargetValid = "TargetValid"

	// VirtualMachinePublishRequestConditionUploaded is the Type for a
	// VirtualMachinePublishRequest resource's status condition.
	//
	// The condition's status is set to true only when the VM being
	// published has been successfully uploaded.
	VirtualMachinePublishRequestConditionUploaded = "Uploaded"

	// VirtualMachinePublishRequestConditionImageAvailable is the Type for a
	// VirtualMachinePublishRequest resource's status condition.
	//
	// The condition's status is set to true only when a new
	// VirtualMachineImage resource has been realized from the published
	// VM.
	VirtualMachinePublishRequestConditionImageAvailable = "ImageAvailable"

	// VirtualMachinePublishRequestConditionComplete is the Type for a
	// VirtualMachinePublishRequest resource's status condition.
	//
	// The condition's status is set to true only when all other conditions
	// present on the resource have a truthy status.
	VirtualMachinePublishRequestConditionComplete = "Complete"
)

// VirtualMachinePublishRequestSource is the source of a publication request,
// typically a VirtualMachine resource.
type VirtualMachinePublishRequestSource struct {
	// Name is the name of the referenced object.
	//
	// If omitted this value defaults to the name of the
	// VirtualMachinePublishRequest resource.
	//
	// +optional
	Name string `json:"name,omitempty"`

	// APIVersion is the API version of the referenced object.
	//
	// +kubebuilder:default=vmoperator.vmware.com/v1alpha1
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind is the kind of referenced object.
	//
	// +kubebuilder:default=VirtualMachine
	// +optional
	Kind string `json:"kind,omitempty"`
}

// VirtualMachinePublishRequestTargetItem is the item part of a
// publication request's target.
type VirtualMachinePublishRequestTargetItem struct {
	// Name is the name of the published object.
	//
	// If the spec.target.location.apiVersion equals
	// imageregistry.vmware.com/v1alpha1 and the spec.target.location.kind
	// equals ContentLibrary, then this should be the name that will
	// show up in vCenter Content Library, not the custom resource name
	// in the namespace.
	//
	// If omitted then the controller will use spec.source.name + "-image".
	//
	// +optional
	Name string `json:"name,omitempty"`

	// Description is the description to assign to the published object.
	//
	// +optional
	Description string `json:"description,omitempty"`
}

// VirtualMachinePublishRequestTargetLocation is the location part of a
// publication request's target.
type VirtualMachinePublishRequestTargetLocation struct {
	// Name is the name of the referenced object.
	//
	// Please note an error will be returned if this field is not
	// set in a namespace that lacks a default publication target.
	//
	// A default publication target is a resource with an API version
	// equal to spec.target.location.apiVersion, a kind equal to
	// spec.target.location.kind, and has the label
	// "imageregistry.vmware.com/default".
	//
	// +optional
	Name string `json:"name,omitempty"`

	// APIVersion is the API version of the referenced object.
	//
	// +kubebuilder:default=imageregistry.vmware.com/v1alpha1
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind is the kind of referenced object.
	//
	// +kubebuilder:default=ContentLibrary
	// +optional
	Kind string `json:"kind,omitempty"`
}

// VirtualMachinePublishRequestTarget is the target of a publication request,
// typically a ContentLibrary resource.
type VirtualMachinePublishRequestTarget struct {
	// Item contains information about the name of the object to which
	// the VM is published.
	//
	// Please note this value is optional and if omitted, the controller
	// will use spec.source.name + "-image" as the name of the published
	// item.
	//
	// +optional
	Item VirtualMachinePublishRequestTargetItem `json:"item,omitempty"`

	// Location contains information about the location to which to publish
	// the VM.
	//
	// +optional
	Location VirtualMachinePublishRequestTargetLocation `json:"location,omitempty"`
}

// VirtualMachinePublishRequestSpec defines the desired state of a
// VirtualMachinePublishRequest.
//
// All the fields in this spec are optional. This is especially useful when a
// DevOps persona wants to publish a VM without doing anything more than
// applying a VirtualMachinePublishRequest resource that has the same name
// as said VM in the same namespace as said VM.
type VirtualMachinePublishRequestSpec struct {
	// Source is the source of the publication request, ex. a VirtualMachine
	// resource.
	//
	// If this value is omitted then the publication controller checks to
	// see if there is a resource with the same name as this
	// VirtualMachinePublishRequest resource, an API version equal to
	// spec.source.apiVersion, and a kind equal to spec.source.kind. If such
	// a resource exists, then it is the source of the publication.
	//
	// +optional
	Source VirtualMachinePublishRequestSource `json:"source,omitempty"`

	// Target is the target of the publication request, ex. item
	// information and a ContentLibrary resource.
	//
	// If this value is omitted, the controller uses spec.source.name + "-image"
	// as the name of the published item. Additionally, when omitted the
	// controller attempts to identify the target location by matching a
	// resource with an API version equal to spec.target.location.apiVersion, a
	// kind equal to spec.target.location.kind, w/ the label
	// "imageregistry.vmware.com/default".
	//
	// Please note that while optional, if a VirtualMachinePublishRequest sans
	// target information is applied to a namespace without a default
	// publication target, then the VirtualMachinePublishRequest resource
	// will be marked in error.
	//
	// +optional
	Target VirtualMachinePublishRequestTarget `json:"target,omitempty"`

	// TTLSecondsAfterFinished is the time-to-live duration for how long this
	// resource will be allowed to exist once the publication operation
	// completes. After the TTL expires, the resource will be automatically
	// deleted without the user having to take any direct action.
	//
	// If this field is unset then the request resource will not be
	// automatically deleted. If this field is set to zero then the request
	// resource is eligible for deletion immediately after it finishes.
	//
	// +optional
	TTLSecondsAfterFinished *int64 `json:"ttlSecondsAfterFinished,omitempty"`
}

// VirtualMachinePublishRequestStatus defines the observed state of a
// VirtualMachinePublishRequest.
type VirtualMachinePublishRequestStatus struct {
	// SourceRef is the reference to the source of the publication request,
	// ex. a VirtualMachine resource.
	//
	// +optional
	SourceRef *VirtualMachinePublishRequestSource `json:"sourceRef,omitempty"`

	// TargetRef is the reference to the target of the publication request,
	// ex. item information and a ContentLibrary resource.
	//
	//
	// +optional
	TargetRef *VirtualMachinePublishRequestTarget `json:"targetRef,omitempty"`

	// CompletionTime represents time when the request was completed. It is not
	// guaranteed to be set in happens-before order across separate operations.
	// It is represented in RFC3339 form and is in UTC.
	//
	// The value of this field should be equal to the value of the
	// LastTransitionTime for the status condition Type=Complete.
	//
	// +optional
	CompletionTime metav1.Time `json:"completionTime,omitempty"`

	// StartTime represents time when the request was acknowledged by the
	// controller. It is not guaranteed to be set in happens-before order
	// across separate operations. It is represented in RFC3339 form and is
	// in UTC.
	//
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`

	// Attempts represents the number of times the request to publish the VM
	// has been attempted.
	//
	// +optional
	Attempts int64 `json:"attempts,omitempty"`

	// LastAttemptTime represents the time when the latest request was sent.
	//
	// +optional
	LastAttemptTime metav1.Time `json:"lastAttemptTime,omitempty"`

	// ImageName is the name of the VirtualMachineImage resource that is
	// eventually realized in the same namespace as the VM and publication
	// request after the publication operation completes.
	//
	// This field will not be set until the VirtualMachineImage resource
	// is realized.
	//
	// +optional
	ImageName string `json:"imageName,omitempty"`

	// Ready is set to true only when the VM has been published successfully
	// and the new VirtualMachineImage resource is ready.
	//
	// Readiness is determined by waiting until there is status condition
	// Type=Complete and ensuring it and all other status conditions present
	// have a Status=True. The conditions present will be:
	//
	//   * SourceValid
	//   * TargetValid
	//   * Uploaded
	//   * ImageAvailable
	//   * Complete
	//
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Conditions is a list of the latest, available observations of the
	// request's current state.
	//
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

func (vmpr VirtualMachinePublishRequest) getCondition(
	conditionType ConditionType,
) *Condition {

	for i := range vmpr.Status.Conditions {
		c := &vmpr.Status.Conditions[i]
		if c.Type == conditionType {
			return c
		}
	}

	return nil
}

// IsSourceValid returns true if there is a status condition of Type=SourceValid
// and Status=True.
func (vmpr VirtualMachinePublishRequest) IsSourceValid() bool {
	c := vmpr.getCondition(VirtualMachinePublishRequestConditionSourceValid)
	if c != nil && c.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

// IsTargetValid returns true if there is a status condition of
// Type=TargetValid and Status=True.
func (vmpr VirtualMachinePublishRequest) IsTargetValid() bool {
	c := vmpr.getCondition(VirtualMachinePublishRequestConditionTargetValid)
	if c != nil && c.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

// IsComplete returns true if there is a status condition of Type=Complete
// and Status=True and the resource's status.completionTime is non-zero.
//
// It is recommended to use the MarkComplete function in order to mark
// a VirtualMachinePublishRequest as complete since the function ensures
// the status.completionTime field is properly set.
//
// This condition is set only when the operation has failed or the
// VirtualMachineImage resource has been realized.
func (vmpr VirtualMachinePublishRequest) IsComplete() bool {
	c := vmpr.getCondition(VirtualMachinePublishRequestConditionComplete)
	if c != nil && c.Status == corev1.ConditionTrue {
		return !vmpr.Status.CompletionTime.IsZero()
	}
	return false
}

// IsImageAvailable returns true if there is a status condition of
// Type=ImageAvailable and Status=True.
func (vmpr VirtualMachinePublishRequest) IsImageAvailable() bool {
	c := vmpr.getCondition(VirtualMachinePublishRequestConditionImageAvailable)
	if c != nil && c.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

// IsUploaded returns true if there is a status condition of Type=Uploaded
// and Status=True.
//
// This condition is set when the publication has finished uploading the
// VM to the image registry and is waiting on the VirtualMachineImage
// resource to be realized.
func (vmpr VirtualMachinePublishRequest) IsUploaded() bool {
	c := vmpr.getCondition(VirtualMachinePublishRequestConditionUploaded)
	if c != nil && c.Status == corev1.ConditionTrue {
		return true
	}
	return false
}

func (vmpr *VirtualMachinePublishRequest) markCondition(
	conditionType ConditionType,
	status corev1.ConditionStatus,
	args ...string,
) {
	var (
		message string
		reason  string
	)
	if len(args) > 0 {
		reason = args[0]
	}
	if len(args) > 1 {
		message = args[1]
	}
	if reason == "" && status == corev1.ConditionTrue {
		reason = VirtualMachinePublishRequestConditionSuccess
	} else {
		reason = string(status)
	}

	c := vmpr.getCondition(conditionType)
	if c != nil {
		c.Message = message
		c.Reason = reason
		c.Status = status
		// Only update the LastTransitionTime if something actually changed.
		if c.Message != message || c.Reason != reason || c.Status != status {
			c.LastTransitionTime = metav1.NewTime(time.Now().UTC())
		}
		return
	}
	vmpr.Status.Conditions = append(
		vmpr.Status.Conditions,
		Condition{
			Type:               conditionType,
			Message:            message,
			Reason:             reason,
			Status:             status,
			LastTransitionTime: metav1.NewTime(time.Now().UTC()),
		},
	)
}

// MarkComplete adds or updates the status condition Type=Complete
// sets the condition's Status field to the provided value.
//
// This function also updates the status.completionTime field to
// the current time in UTC. Please note this field is updated every
// time this function is called. If this function is called with a
// status of anything other than True, the value of status.completionTIme
// is zeroed.
//
// This function also sets status.ready=true IFF the provided status
// is True and all other conditions present in the resource have a
// status=True.
//
// The variadic parameter args may be used to set the condition's
// reason and message. If len(args) > 0 then args[0] is used as the
// reason. If len(args) > 1 then args[1] is used as the message.
//
// If no reason is provided and status=True, then reason is set to
// to "Success," otherwise string(status).
//
// If no message is provided then it is set to an empty string.
func (vmpr *VirtualMachinePublishRequest) MarkComplete(
	status corev1.ConditionStatus,
	args ...string,
) {
	switch status {
	case corev1.ConditionTrue:
		vmpr.Status.CompletionTime = metav1.NewTime(time.Now().UTC())
	case corev1.ConditionFalse, corev1.ConditionUnknown:
		vmpr.Status.CompletionTime = metav1.Time{}
	}

	vmpr.markCondition(
		VirtualMachinePublishRequestConditionComplete,
		status,
		args...,
	)

	// At this point, if all conditions have a Status=True then
	// the VirtualMachinePublishRequest resource should be marked
	// ready.
	for _, c := range vmpr.Status.Conditions {
		if c.Status != corev1.ConditionTrue {
			vmpr.Status.Ready = false
			return
		}
	}
	vmpr.Status.Ready = true
}

// MarkSourceValid adds or updates a status condition of
// Type=SourceValid and sets the condition's Status field to the
// provided value.
//
// The variadic parameter args may be used to set the condition's
// reason and message. If len(args) > 0 then args[0] is used as the
// reason. If len(args) > 1 then args[1] is used as the message.
//
// If no reason is provided and status=True, then reason is set to
// to "Success," otherwise string(status).
//
// If no message is provided then it is set to an empty string.
func (vmpr *VirtualMachinePublishRequest) MarkSourceValid(
	status corev1.ConditionStatus,
	args ...string,
) {
	vmpr.markCondition(
		VirtualMachinePublishRequestConditionSourceValid,
		status,
		args...,
	)
}

// MarkTargetValid adds or updates a status condition of
// Type=TargetValid and sets the condition's Status field to the
// provided value.
//
// The variadic parameter args may be used to set the condition's
// reason and message. If len(args) > 0 then args[0] is used as the
// reason. If len(args) > 1 then args[1] is used as the message.
//
// If no reason is provided and status=True, then reason is set to
// to "Success," otherwise string(status).
//
// If no message is provided then it is set to an empty string.
func (vmpr *VirtualMachinePublishRequest) MarkTargetValid(
	status corev1.ConditionStatus,
	args ...string,
) {
	vmpr.markCondition(
		VirtualMachinePublishRequestConditionTargetValid,
		status,
		args...,
	)
}

// MarkImageAvailable adds or updates a status condition of
// Type=ImageAvailable and sets the condition's Status field to the
// provided value.
//
// The variadic parameter args may be used to set the condition's
// reason and message. If len(args) > 0 then args[0] is used as the
// reason. If len(args) > 1 then args[1] is used as the message.
//
// If no reason is provided and status=True, then reason is set to
// to "Success," otherwise string(status).
//
// If no message is provided then it is set to an empty string.
func (vmpr *VirtualMachinePublishRequest) MarkImageAvailable(
	status corev1.ConditionStatus,
	args ...string,
) {
	vmpr.markCondition(
		VirtualMachinePublishRequestConditionImageAvailable,
		status,
		args...,
	)
}

// MarkUploaded adds or updates a status condition of Type=Uploaded and
// sets the condition's Status field to the provided value.
//
// The variadic parameter args may be used to set the condition's
// reason and message. If len(args) > 0 then args[0] is used as the
// reason. If len(args) > 1 then args[1] is used as the message.
//
// If no reason is provided and status=True, then reason is set to
// to "Success," otherwise string(status).
//
// If no message is provided then it is set to an empty string.
func (vmpr *VirtualMachinePublishRequest) MarkUploaded(
	status corev1.ConditionStatus,
	args ...string,
) {
	vmpr.markCondition(
		VirtualMachinePublishRequestConditionUploaded,
		status,
		args...,
	)
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=vmpub
// +kubebuilder:storageversion
// +kubebuilder:subresource:status

// VirtualMachinePublishRequest defines the information necessary to publish a
// VirtualMachine as a VirtualMachineImage to an image registry.
type VirtualMachinePublishRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachinePublishRequestSpec   `json:"spec,omitempty"`
	Status VirtualMachinePublishRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VirtualMachinePublishRequestList contains a list of
// VirtualMachinePublishRequest resources.
type VirtualMachinePublishRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachinePublishRequest `json:"items"`
}

func init() {
	RegisterTypeWithScheme(
		&VirtualMachinePublishRequest{},
		&VirtualMachinePublishRequestList{},
	)
}
