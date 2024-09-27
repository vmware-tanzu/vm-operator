// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

// GetOwnerRefForStorageClass returns an OwnerRef for the provided StorageClass.
func GetOwnerRefForStorageClass(
	storageClass storagev1.StorageClass) metav1.OwnerReference {

	return metav1.OwnerReference{
		APIVersion: StorageClassGroupVersion,
		Kind:       StorageClassKind,
		Name:       storageClass.Name,
		UID:        storageClass.UID,
	}
}

// GetEncryptedStorageClassRefs returns a list of the OwnerRef objects for
// StorageClasses marked as encrypted.
func GetEncryptedStorageClassRefs(
	ctx context.Context,
	k8sClient ctrlclient.Client) ([]metav1.OwnerReference, error) {

	var obj corev1.ConfigMap
	if err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKey{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      EncryptedStorageClassNamesConfigMapName,
		},
		&obj); err != nil {

		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	var refs []metav1.OwnerReference
	for i := range obj.OwnerReferences {
		ref := obj.OwnerReferences[i]
		if ref.Kind == StorageClassKind &&
			ref.APIVersion == StorageClassGroupVersion {

			refs = append(refs, ref)
		}
	}

	return refs, nil
}
