// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/util/kube/internal"
)

// GetStoragePolicyID returns the storage policy ID for a given StorageClass.
// If no ID is found, an error is returned.
func GetStoragePolicyID(obj storagev1.StorageClass) (string, error) {
	policyID, ok := obj.Parameters[internal.StoragePolicyIDParameter]
	if !ok {
		return "", fmt.Errorf(
			"StorageClass %q does not have '"+
				internal.StoragePolicyIDParameter+"' parameter",
			obj.Name)
	}
	return policyID, nil
}

// MarkEncryptedStorageClass records the provided StorageClass as encrypted.
func MarkEncryptedStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	storageClass storagev1.StorageClass,
	encrypted bool) error {

	var (
		obj = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pkgcfg.FromContext(ctx).PodNamespace,
				Name:      internal.EncryptedStorageClassNamesConfigMapName,
			},
		}
		ownerRef = internal.GetOwnerRefForStorageClass(storageClass)
	)

	// Get the ConfigMap.
	err := k8sClient.Get(ctx, ctrlclient.ObjectKeyFromObject(&obj), &obj)

	if err != nil {
		// Return any error other than 404.
		if !apierrors.IsNotFound(err) {
			return err
		}

		//
		// The ConfigMap was not found.
		//

		if !encrypted {
			// If the goal is to mark the StorageClass as not encrypted, then we
			// do not need to actually create the underlying ConfigMap if it
			// does not exist.
			return nil
		}

		// The ConfigMap does not already exist and the goal is to mark the
		// StorageClass as encrypted, so go ahead and create the ConfigMap and
		// return early.
		obj.OwnerReferences = []metav1.OwnerReference{ownerRef}
		return k8sClient.Create(ctx, &obj)
	}

	//
	// The ConfigMap already exists, so check if it needs to be updated.
	//

	storageClassIsOwner := slices.Contains(obj.OwnerReferences, ownerRef)

	switch {
	case encrypted && storageClassIsOwner:
		// The StorageClass should be marked encrypted, which means it should
		// be in the ConfigMap. Since the StorageClass is currently set in the
		// ConfigMap, we can return early.
		return nil
	case !encrypted && !storageClassIsOwner:
		// The StorageClass should be marked as not encrypted, which means it
		// should not be in the ConfigMap. Since the StorageClass is not
		// currently in the ConfigMap, we can return return early.
		return nil
	}

	// Create the patch used to update the ConfigMap.
	objPatch := ctrlclient.StrategicMergeFrom(obj.DeepCopyObject().(ctrlclient.Object))
	if encrypted {
		// Add the StorageClass as an owner of the ConfigMap.
		obj.OwnerReferences = append(obj.OwnerReferences, ownerRef)
	} else {
		// Remove the StorageClass as an owner of the ConfigMap.
		obj.OwnerReferences = slices.DeleteFunc(
			obj.OwnerReferences,
			func(o metav1.OwnerReference) bool { return o == ownerRef })
	}

	// Patch the ConfigMap with the change.
	return k8sClient.Patch(ctx, &obj, objPatch)
}

// IsEncryptedStorageClass returns true if the provided StorageClass was marked
// as encrypted.
func IsEncryptedStorageClass(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	storageClassName string) (bool, error) {

	var storageClass storagev1.StorageClass
	if err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKey{Name: storageClassName},
		&storageClass); err != nil {

		return false, ctrlclient.IgnoreNotFound(err)
	}

	var (
		obj      corev1.ConfigMap
		ownerRef = internal.GetOwnerRefForStorageClass(storageClass)
	)

	if err := k8sClient.Get(
		ctx,
		ctrlclient.ObjectKey{
			Namespace: pkgcfg.FromContext(ctx).PodNamespace,
			Name:      internal.EncryptedStorageClassNamesConfigMapName,
		},
		&obj); err != nil {

		return false, ctrlclient.IgnoreNotFound(err)
	}

	return slices.Contains(obj.OwnerReferences, ownerRef), nil
}
