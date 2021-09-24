// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package instancevm

const (
	// PVCsAnnotationKey is annotation key to store the list of PVC names.
	PVCsAnnotationKey = "vmoperator.vmware.com/instance-storage-pvcs"

	// PVCsAnnotationValueSeparator is the field separator between multiple PVC names in annotation value.
	PVCsAnnotationValueSeparator = "|"
)
