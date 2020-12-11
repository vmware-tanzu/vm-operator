// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// Common ConditionTypes used by VM Operator API objects.
const (
	// ReadyCondition defines the Ready condition type that summarizes the operational state of a VM Operator API object.
	ReadyCondition ConditionType = "Ready"
)

// Common Condition.Reason used by VM Operator API objects.
const (
	// DeletingReason (Severity=Info) documents an condition not in Status=True because the underlying object it is currently being deleted.
	DeletingReason = "Deleting"

	// DeletionFailedReason (Severity=Warning) documents an condition not in Status=True because the underlying object
	// encountered problems during deletion. This is a warning because the reconciler will retry deletion.
	DeletionFailedReason = "DeletionFailed"

	// DeletedReason (Severity=Info) documents an condition not in Status=True because the underlying object was deleted.
	DeletedReason = "Deleted"
)
