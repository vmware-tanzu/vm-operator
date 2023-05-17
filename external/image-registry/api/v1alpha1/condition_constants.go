// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// Common ConditionTypes used by Image Registry Operator API objects.
const (
	// ReadyCondition defines the Ready condition type that summarizes the operational state of an Image Registry Operator API object.
	ReadyCondition ConditionType = "Ready"
)

// Common Condition.Reason used by Image Registry Operator API objects.
const (
	// DeletingReason (Severity=Info) documents a condition not in Status=True because the object is currently being deleted.
	DeletingReason = "Deleting"

	// DeletedReason (Severity=Error) documents a condition not in Status=True because the underlying object was deleted.
	DeletedReason = "Deleted"
)

// Condition.Reasons related to ClusterContentLibraryItem or ContentLibraryItem API objects.
const (
	ClusterContentLibraryRefValidationFailedReason = "ClusterContentLibraryRefValidationFailed"
	ContentLibraryRefValidationFailedReason        = "ContentLibraryRefValidationFailed"
	ContentLibraryItemFileUnavailableReason        = "ContentLibraryItemFileUnavailable"
)
