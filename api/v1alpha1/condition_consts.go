// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// Common ConditionTypes used by VM Operator API objects.
const (
	// ReadyCondition defines the Ready condition type that summarizes the operational state of a VM Operator API object.
	ReadyCondition ConditionType = "Ready"
)

// Conditions and condition Reasons for the VirtualMachine object.

const (
	// VirtualMachinePrereqReadyCondition documents that all of a VirtualMachine's prerequisites declared in the spec
	// (e.g. VirtualMachineClass) are satisfied.
	VirtualMachinePrereqReadyCondition ConditionType = "VirtualMachinePrereqReady"

	// VirtualMachineClassBindingNotFoundReason (Severity=Error) documents a missing VirtualMachineClassBinding for the
	// VirtualMachineClass specified in the VirtualMachineSpec.
	VirtualMachineClassBindingNotFoundReason = "VirtualMachineClassBindingNotFound"

	// VirtualMachineClassNotFoundReason (Severity=Error) documents that the VirtualMachineClass specified in the VirtualMachineSpec
	// is not available.
	VirtualMachineClassNotFoundReason = "VirtualMachineClassNotFound"

	// ContentSourceBindingNotFoundReason (Severity=Error) documents a missing ContentSourceBinding for the
	// VirtualMachineImage specified in the VirtualMachineSpec.
	ContentSourceBindingNotFoundReason = "ContentSourceBindingNotFound"

	// ContentLibraryProviderNotFoundReason (Severity=Error) documents that the ContentLibraryProvider corresponding to a VirtualMachineImage
	// is not available.
	ContentLibraryProviderNotFoundReason = "ContentLibraryProviderNotFound"

	// VirtualMachineImageNotFoundReason (Severity=Error) documents that the VirtualMachineImage specified in the VirtualMachineSpec
	// is not available.
	VirtualMachineImageNotFoundReason = "VirtualMachineImageNotFound"

	// VirtualMachineImageNotReadyReason (Severity=Error) documents that the VirtualMachineImage specified in the VirtualMachineSpec
	// is not ready.
	VirtualMachineImageNotReadyReason = "VirtualMachineImageNotReady"
)

const (
	// GuestCustomizationCondition exposes the status of guest customization from within the guest OS, when available.
	GuestCustomizationCondition ConditionType = "GuestCustomization"

	// GuestCustomizationIdleReason (Severity=Info) documents that guest customizations were not applied for the VirtualMachine.
	GuestCustomizationIdleReason = "GuestCustomizationIdle"

	// GuestCustomizationPendingReason (Severity=Info) documents that guest customization is still pending within the guest OS.
	GuestCustomizationPendingReason = "GuestCustomizationPending"

	// GuestCustomizationRunningReason (Severity=Info) documents that the guest customization is now running on the guest OS.
	GuestCustomizationRunningReason = "GuestCustomizationRunning"

	// GuestCustomizationSucceededReason (Severity=Info) documents that the guest customization succeeded within the guest OS.
	GuestCustomizationSucceededReason = "GuestCustomizationSucceeded"

	// GuestCustomizationFailedReason (Severity=Error) documents that the guest customization failed within the guest OS.
	GuestCustomizationFailedReason = "GuestCustomizationFailed"
)

const (
	// VirtualMachineToolsCondition exposes the status of VMware Tools running in the guest OS, when available.
	VirtualMachineToolsCondition ConditionType = "VirtualMachineTools"

	// VirtualMachineToolsNotRunningReason (Severity=Error) documents that VMware Tools is not running.
	VirtualMachineToolsNotRunningReason = "VirtualMachineToolsNotRunning"

	// VirtualMachineToolsRunningReason (Severity=Info) documents that VMware Tools is running.
	VirtualMachineToolsRunningReason = "VirtualMachineToolsRunning"
)

// Common Condition.Reason used by VM Operator API objects.
const (
	// DeletingReason (Severity=Info) documents a condition not in Status=True because the underlying object it is currently being deleted.
	DeletingReason = "Deleting"

	// DeletionFailedReason (Severity=Warning) documents a condition not in Status=True because the underlying object
	// encountered problems during deletion. This is a warning because the reconciler will retry deletion.
	DeletionFailedReason = "DeletionFailed"

	// DeletedReason (Severity=Info) documents a condition not in Status=True because the underlying object was deleted.
	DeletedReason = "Deleted"
)

// Conditions related to the VirtualMachineImages.
const (
	// Deprecated
	// VirtualMachineImageOSTypeSupportedCondition denotes that the OS type in the VirtualMachineImage object is
	// supported by VMService. A VirtualMachineImageOsTypeSupportedCondition is marked true:
	// - If OS Type is of Linux Family
	// - If OS Type is supported by hosts in the cluster.
	VirtualMachineImageOSTypeSupportedCondition ConditionType = "VirtualMachineImageOSTypeSupported"

	// VirtualMachineImageV1Alpha1CompatibleCondition denotes image compatibility with VMService. VMService expects
	// VirtualMachineImage to be prepared by VMware specifically for VMService v1alpha1.
	VirtualMachineImageV1Alpha1CompatibleCondition ConditionType = "VirtualMachineImageV1Alpha1Compatible"

	// VirtualMachineImageSyncedCondition denotes that the image is synced with the vSphere content library item
	// that contains the source of this image's information.
	VirtualMachineImageSyncedCondition ConditionType = "VirtualMachineImageSynced"

	// VirtualMachineImageProviderReadyCondition denotes readiness of the VirtualMachineImage provider.
	VirtualMachineImageProviderReadyCondition ConditionType = "VirtualMachineImageProviderReady"
)

// Condition.Reason for Conditions related to VirtualMachineImages.
const (
	// Deprecated
	// VirtualMachineImageOSTypeNotSupportedReason (Severity=Error) documents that OS Type is VirtualMachineImage is
	// not supported.
	VirtualMachineImageOSTypeNotSupportedReason = "VirtualMachineImageOSTypeNotSupported"

	// VirtualMachineImageV1Alpha1NotCompatibleReason (Severity=Error) documents that the VirtualMachineImage
	// is not prepared for VMService consumption.
	VirtualMachineImageV1Alpha1NotCompatibleReason = "VirtualMachineImageV1Alpha1NotCompatible"

	// VirtualMachineImageNotSyncedReason (Severity=Error) documents that the VirtualMachineImage is not synced with
	// the vSphere content library item that contains the source of this image's information.
	VirtualMachineImageNotSyncedReason = "VirtualMachineImageNotSynced"

	// VirtualMachineImageProviderNotReadyReason (Severity=Error) documents that the VirtualMachineImage provider
	// is not in ready state.
	VirtualMachineImageProviderNotReadyReason = "VirtualMachineImageProviderNotReady"
)

// Condition.Reason for Conditions related to VirtualMachinePublishRequest.
const (
	// SourceVirtualMachineNotExistReason documents that the source VM of
	// the VirtualMachinePublishRequest doesn't exist.
	SourceVirtualMachineNotExistReason = "SourceVirtualMachineNotExist"

	// SourceVirtualMachineNotCreatedReason documents that the source VM of
	// the VirtualMachinePublishRequest hasn't been created.
	SourceVirtualMachineNotCreatedReason = "SourceVirtualMachineNotCreated"

	// TargetContentLibraryNotExistReason documents that the target content
	// library of the VirtualMachinePublishRequest doesn't exist.
	TargetContentLibraryNotExistReason = "TargetContentLibraryNotExist"

	// TargetContentLibraryNotWritableReason documents that the target content
	// library of the VirtualMachinePublishRequest isn't writable.
	TargetContentLibraryNotWritableReason = "TargetContentLibraryNotWritable"

	// TargetItemAlreadyExistsReason documents that in the target content
	// library, there is already an item, which has the same name with the
	// VirtualMachinePublishRequest's target item.
	TargetItemAlreadyExistsReason = "TargetItemAlreadyExists"

	// TargetVirtualMachineImageNotFoundReason documents that the expected
	// VirtualMachineImage resource which is corresponding to VirtualMachinePublishRequest
	// target item is not found in the cluster.
	TargetVirtualMachineImageNotFoundReason = "VirtualMachineImageNotFound"

	// UploadTaskQueuedReason documents that the VM publish task is in queued status.
	UploadTaskQueuedReason = "Queued"

	// UploadingReason documents that the VM publish task is in running status
	// and the published item is uploading to the target location.
	UploadingReason = "Uploading"

	// UploadFailureReason documents that uploading published item to the
	// target location failed.
	UploadFailureReason = "UploadFailure"

	// HasNotBeenUploadedReason documents that the VirtualMachinePublishRequest
	// hasn't completed because the published item hasn't been uploaded
	// to the target location.
	HasNotBeenUploadedReason = "HasNotBeenUploaded"

	// ImageUnavailableReason documents that the VirtualMachinePublishRequest
	// hasn't been completed because the expected VirtualMachineImage resource
	// hasn't been available yet.
	ImageUnavailableReason = "ImageUnavailable"
)
