// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	// ReadyConditionType is the Ready condition type that summarizes the
	// operational state of an API.
	ReadyConditionType = "Ready"
)

// ManagedObjectID is a unique ID used to identify a managed object on a given
// vSphere instance
type ManagedObjectID struct {
	// +required

	// ObjectID is the object's ID.
	ObjectID string `json:"objectID"`

	// +optional

	// ServerID is the ID of the server to which the object belongs.
	ServerID string `json:"serverID,omitempty"`
}
