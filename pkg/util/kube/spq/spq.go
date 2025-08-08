// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package spq

import (
	"context"
	"fmt"
	"strings"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	spqv1 "github.com/vmware-tanzu/vm-operator/external/storage-policy-quota/api/v1alpha2"
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
	StoragePolicyQuotaExtensionName = "vmware-system-vmop-webhook-service"

	// ValidatingWebhookConfigName is the name of the ValidatingWebhookConfiguration
	// containing correct CA Bundle used by StoragePolicyUsage.
	ValidatingWebhookConfigName = "vmware-system-vmop-validating-webhook-configuration"
	WebhookConfigAnnotationKey  = "cert-manager.io/inject-ca-from"
	SecretCertKey               = "ca.crt"

	// VirtualMachineKind is the name of the VirtualMachine kind.
	VirtualMachineKind = "VirtualMachine"

	// VirtualMachineSnapshotKind is the name of the VirtualMachineSnapshot kind.
	VirtualMachineSnapshotKind = "VirtualMachineSnapshot"
)

// StoragePolicyUsageNameForVM returns the name of the StoragePolicyUsage
// resource with a given StorageClass name for VirtualMachine resources.
func StoragePolicyUsageNameForVM(storageClassName string) string {
	return storageClassName + "-vm-usage"
}

// StoragePolicyUsageNameForVMSnapshot returns the name of the StoragePolicyUsage
// resource with a given StorageClass name for VirtualMachineSnapshot resources.
func StoragePolicyUsageNameForVMSnapshot(storageClassName string) string {
	return storageClassName + "-vmsnapshot-usage"
}

// GetStorageClassesForPolicy returns the StorageClass resources that reference
// the provided policy ID for a given namespace.
func GetStorageClassesForPolicy(
	ctx context.Context,
	k8sClient client.Client,
	namespace, id string) ([]storagev1.StorageClass, error) {

	var obj storagev1.StorageClassList
	if err := k8sClient.List(ctx, &obj); err != nil {
		return nil, err
	}

	var matches []storagev1.StorageClass
	for i := range obj.Items {
		o := obj.Items[i]
		if id == o.Parameters[storageClassParamPolicyID] {
			ok, err := IsStorageClassInNamespace(
				ctx,
				k8sClient,
				&o,
				namespace)
			if err != nil {
				return nil, err
			}
			if ok {
				matches = append(matches, o)
			}
		}
	}

	return matches, nil
}

// GetStorageClassesForPolicyQuota returns the StorageClass resources that reference
// the provided StoragePolicyQuota. These are StorageClasses that have been associated
// with the namespace.
func GetStorageClassesForPolicyQuota(
	ctx context.Context,
	k8sClient client.Client,
	spq *spqv1.StoragePolicyQuota) ([]storagev1.StorageClass, error) {

	var obj storagev1.StorageClassList
	if err := k8sClient.List(ctx, &obj); err != nil {
		return nil, err
	}

	var matches []storagev1.StorageClass
	for i := range obj.Items {
		o := obj.Items[i]
		if spq.Spec.StoragePolicyId == o.Parameters[storageClassParamPolicyID] {
			matches = append(matches, o)
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

	ok, err := IsStorageClassInNamespace(ctx, k8sClient, &sc, namespace)
	if err != nil {
		return storagev1.StorageClass{}, err
	}

	if !ok {
		return storagev1.StorageClass{}, NotFoundInNamespace{
			Namespace:    namespace,
			StorageClass: name,
		}
	}

	return sc, nil
}

// IsStorageClassInNamespace returns true if the provided storage class is
// available in the provided namespace.
func IsStorageClassInNamespace(
	ctx context.Context,
	k8sClient client.Client,
	sc *storagev1.StorageClass,
	namespace string) (bool, error) {

	if pkgcfg.FromContext(ctx).Features.PodVMOnStretchedSupervisor {
		policyID := sc.Parameters[storageClassParamPolicyID]
		if policyID == "" {
			return false, fmt.Errorf("StorageClass does not have policy ID")
		}

		var obj spqv1.StoragePolicyQuotaList
		if err := k8sClient.List(
			ctx,
			&obj,
			client.InNamespace(namespace)); err != nil {

			return false, err
		}

		for i := range obj.Items {
			if obj.Items[i].Spec.StoragePolicyId == policyID {
				return true, nil
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

		prefix := sc.Name + storageResourceQuotaStrPattern
		for i := range obj.Items {
			for name := range obj.Items[i].Spec.Hard {
				if strings.HasPrefix(name.String(), prefix) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// GetWebhookCABundle returns the CA Bundle used for validating webhooks. It attempts to fetch the CA Bundle
// from the ValidatingWebhookConfiguration first before attempting to fetch from the certificate Secret.
func GetWebhookCABundle(ctx context.Context, k8sClient client.Client) ([]byte, error) {
	webhookConfiguration := &admissionv1.ValidatingWebhookConfiguration{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: ValidatingWebhookConfigName}, webhookConfiguration); err != nil {
		return nil, err
	}

	var caBundle []byte
	for _, webhook := range webhookConfiguration.Webhooks {
		if len(webhook.ClientConfig.CABundle) > 0 {
			caBundle = webhook.ClientConfig.CABundle
			break
		}
	}

	if len(caBundle) == 0 {
		certSecretNamespaceName, ok := webhookConfiguration.Annotations[WebhookConfigAnnotationKey]
		if !ok || certSecretNamespaceName == "" {
			return nil, fmt.Errorf("unable to get annotation for key: %s", WebhookConfigAnnotationKey)
		}

		namespaceAndName := strings.Split(certSecretNamespaceName, "/")
		if len(namespaceAndName) != 2 {
			return nil, fmt.Errorf("unable to get namespace and name for key: %s", WebhookConfigAnnotationKey)
		}

		certSecret := &corev1.Secret{}
		key := client.ObjectKey{Namespace: namespaceAndName[0], Name: namespaceAndName[1]}
		if err := k8sClient.Get(ctx, key, certSecret); err != nil {
			return nil, err
		}

		caBundle, ok = certSecret.Data[SecretCertKey]
		if !ok || len(caBundle) == 0 {
			return nil, fmt.Errorf("unable to get CA bundle from secret; data[%s] not found", SecretCertKey)
		}
	}

	return caBundle, nil
}
