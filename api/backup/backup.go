// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

// +kubebuilder:object:generate=true

package backup

import (
	corev1 "k8s.io/api/core/v1"
)

// PVCDiskData contains the backup data of a PVC disk attached to VM.
type PVCDiskData struct {
	// Filename is the datastore path to the virtual disk.
	FileName string
	// PVCName is the name of the PVC backed by the virtual disk.
	PVCName string
	// AccessMode is the access modes of the PVC backed by the virtual disk.
	AccessModes []corev1.PersistentVolumeAccessMode
}

// ClassicDiskData contains the backup data of a classic (static) disk attached
// to VM.
type ClassicDiskData struct {
	// Filename is the datastore path to the virtual disk.
	FileName string
}
