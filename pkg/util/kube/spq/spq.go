// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spq

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	storageResourceQuotaStrPattern = ".storageclass.storage.k8s.io/"
	storageClassParamPolicyID      = "storagePolicyID"

	// StoragePolicyQuotaKind is the name of the StoragePolicyQuota kind.
	StoragePolicyQuotaKind = "StoragePolicyQuota"

	// StoragePolicyUsageKind is the name of the StoragePolicyUsage kind.
	StoragePolicyUsageKind = "StoragePolicyUsage"

	// StoragePolicyQuotaExtensionName is the name of the storage policy
	// extension service provided by VM Service.
	StoragePolicyQuotaExtensionName = "vmservice.cns.vsphere.vmware.com"
)

// StoragePolicyUsageName returns the name of the StoragePolicyUsage
// resource for a given StorageClass resource name.
func StoragePolicyUsageName(storageClassName string) string {
	return storageClassName + "-vm-usage"
}

// GetStorageClassesForPolicy returns the StorageClass resources that reference
// the provided policy ID for a given namespace.
func GetStorageClassesForPolicy(
	ctx context.Context,
	k8sClient client.Client,
	namespace, id string) ([]storagev1.StorageClass, error) {

	var obj storagev1.StorageClassList
	if err := k8sClient.List(ctx, &obj); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	var matches []storagev1.StorageClass
	for i := range obj.Items {
		o := obj.Items[i]
		if id == o.Parameters[storageClassParamPolicyID] {
			ok, err := IsStorageClassInNamespace(
				ctx,
				k8sClient,
				namespace,
				o.Name)
			if err != nil {
				if _, ok := err.(NotFoundInNamespace); !ok {
					return nil, err
				}
			}
			if ok {
				matches = append(matches, o)
			}
		}
	}

	return matches, nil
}

// GetStoragePolicyIDFromClass returns the storage policy ID for the named
// storage class.
func GetStoragePolicyIDFromClass(
	ctx context.Context,
	k8sClient client.Client,
	name string) (string, error) {

	var obj storagev1.StorageClass
	if err := k8sClient.Get(
		ctx,
		client.ObjectKey{Name: name},
		&obj); err != nil {

		return "", err
	}

	return obj.Parameters[storageClassParamPolicyID], nil
}

type NotFoundInNamespace struct {
	Namespace    string
	StorageClass string
}

func (e NotFoundInNamespace) String() string {
	return fmt.Sprintf(
		"Storage policy is not associated with the namespace %s",
		e.Namespace)
}

func (e NotFoundInNamespace) Error() string {
	return e.String()
}

// GetStorageClassInNamespace returns the named storage class if it is applied
// to the provided namespace. If the storage class does not exist, a NotFound
// error is returned. If the storage class exists, but is not available in the
// specified namespace, a NotFoundInNamespace error is returned.
func GetStorageClassInNamespace(
	ctx context.Context,
	k8sClient client.Client,
	namespace, name string) (storagev1.StorageClass, error) {

	var sc storagev1.StorageClass
	if err := k8sClient.Get(
		ctx,
		client.ObjectKey{Name: name},
		&sc); err != nil {

		return storagev1.StorageClass{}, err
	}

	ok, err := IsStorageClassInNamespace(ctx, k8sClient, namespace, sc.Name)
	if ok {
		return sc, nil
	}

	return storagev1.StorageClass{}, err
}

// IsStorageClassInNamespace returns true if the provided storage class is
// available in the provided namespace.
func IsStorageClassInNamespace(
	ctx context.Context,
	k8sClient client.Client,
	namespace, name string) (bool, error) {

	if pkgcfg.FromContext(ctx).Features.PodVMOnStretchedSupervisor {
		var obj spqv1.StoragePolicyQuotaList
		if err := k8sClient.List(
			ctx,
			&obj,
			client.InNamespace(namespace)); err != nil {

			return false, err
		}

		for i := range obj.Items {
			for j := range obj.Items[i].Status.SCLevelQuotaStatuses {
				qs := obj.Items[i].Status.SCLevelQuotaStatuses[j]
				if qs.StorageClassName == name {
					return true, nil
				}
			}
		}

	} else {
		var obj corev1.ResourceQuotaList
		if err := k8sClient.List(
			ctx,
			&obj,
			client.InNamespace(namespace)); err != nil {

			return false, err
		}

		prefix := name + storageResourceQuotaStrPattern
		for i := range obj.Items {
			for name := range obj.Items[i].Spec.Hard {
				if strings.HasPrefix(name.String(), prefix) {
					return true, nil
				}
			}
		}
	}

	return false, NotFoundInNamespace{
		Namespace:    namespace,
		StorageClass: name,
	}
}
