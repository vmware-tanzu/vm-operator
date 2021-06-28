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

	// VSphereVMCustomizationCondition exposes the status of vSphere VM Customization from within the guest OS, when available.
	VSphereVMCustomizationCondition ConditionType = "VSphereVMCustomization"

	// VSphereVMCustomizationIdleReason (Severity=Info) documents that vSphere VM Customizations were not applied for the VirtualMachine.
	VSphereVMCustomizationIdleReason = "VSphereVMCustomizationIdle"

	// VSphereVMCustomizationPendingReason (Severity=Info) documents that vSphere VM Customization is still pending within the guest OS.
	VSphereVMCustomizationPendingReason = "VSphereVMCustomizationPending"

	// VSphereVMCustomizationRunningReason (Severity=Info) documents that the vSphere VM Customization is now running on the guest OS.
	VSphereVMCustomizationRunningReason = "VSphereVMCustomizationRunning"

	// VSphereVMCustomizationSucceededReason (Severity=Info) documents that the vSphere VM Customization succeeded within the guest OS.
	VSphereVMCustomizationSucceededReason = "VSphereVMCustomizationSucceeded"

	// VSphereVMCustomizationFailedReason (Severity=Error) documents that the vSphere VM Customization failed within the guest OS.
	VSphereVMCustomizationFailedReason = "VSphereVMCustomizationFailed"
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

// Conditions related to the VirtualMachineImages
const (
	// Deprecated
	// VirtualMachineImageOSTypeSupportedCondition denotes that the OS type in the VirtualMachineImage object is
	// supported by VMService. A VirtualMachineImageOsTypeSupportedCondition is marked true:
	// - If OS Type is of Linux Family
	// - If OS Type is supported by hosts in the cluster
	VirtualMachineImageOSTypeSupportedCondition ConditionType = "VirtualMachineImageOSTypeSupported"

	// VirtualMachineImageV1Alpha1CompatibleCondition denotes image compatibility with VMService. VMService expects
	// VirtualMachineImage to be prepared by VMware specifically for VMService v1alpha1.
	VirtualMachineImageV1Alpha1CompatibleCondition ConditionType = "VirtualMachineImageV1Alpha1Compatible"
)

// Condition.Reason for Conditions related to VirtualMachineImages
const (
	// Deprecated
	// VirtualMachineImageOSTypeNotSupportedReason (Severity=Error) documents that OS Type is VirtualMachineImage is
	// not supported.
	VirtualMachineImageOSTypeNotSupportedReason = "VirtualMachineImageOSTypeNotSupported"

	// VirtualMachineImageV1Alpha1NotCompatibleReason (Severity=Error) documents that the VirtualMachine Image
	// is not prepared for VMService consumption.
	VirtualMachineImageV1Alpha1NotCompatibleReason = "VirtualMachineImageV1Alpha1NotCompatible"
)
