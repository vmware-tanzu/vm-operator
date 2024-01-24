// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package constants

const (
	createdAtPrefix = "vmoperator.vmware.com/created-at-"

	// CreatedAtBuildVersionAnnotationKey is set on VirtualMachine
	// objects when an object's metadata.generation value is 1.
	// The value of this annotation may be empty, indicating the VM was first
	// created before this annotation was used. This information itself is
	// in fact useful, as it still indicates something about the build version
	// at which an object was first created.
	CreatedAtBuildVersionAnnotationKey = createdAtPrefix + "build-version"

	// CreatedAtSchemaVersionAnnotationKey is set on VirtualMachine
	// objects when an object's metadata.generation value is 1.
	// The value of this annotation may be empty, indicating the VM was first
	// created before this annotation was used. This information itself is
	// in fact useful, as it still indicates something about the schema version
	// at which an object was first created.
	CreatedAtSchemaVersionAnnotationKey = createdAtPrefix + "schema-version"
)
