// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	storagev1 "k8s.io/api/storage/v1"
)

const (
	// StoragePolicyIDParameter is the name of the parameter set on a
	// StorageClass that indicates the identifier of the underlying
	// storage policy/profile.
	StoragePolicyIDParameter = "storagePolicyID"

	// StorageClassKind is the kind for a StorageClass resource.
	StorageClassKind = "StorageClass"

	// StorageClassGroup is the API group to which a StorageClass resource
	// belongs.
	StorageClassGroup = storagev1.GroupName

	// StorageClassResource is the API resource for a StorageClass.
	StorageClassResource = "storageclasses"

	// StorageClassGroupVersion is the API group and version version for a
	// StorageClass resource.
	StorageClassGroupVersion = StorageClassGroup + "/v1"

	// EncryptedStorageClassNamesConfigMapName is the name of the ConfigMap in
	// the VM Operator pod's namespace that indicates which StorageClasses
	// support encryption by virtue of the OwnerRefs set on the ConfigMap.
	EncryptedStorageClassNamesConfigMapName = "encrypted-storage-class-names"
)
