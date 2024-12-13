// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package spq

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

func ValidatingWebhookConfigurationToStoragePolicyQuotaMapper(
	ctx context.Context,
	k8sClient client.Client) handler.MapFunc {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("client is nil")
	}

	return func(ctx context.Context, o client.Object) []reconcile.Request {
		if ctx == nil {
			panic("context is nil")
		}
		if o == nil {
			panic("object is nil")
		}

		obj, ok := o.(*admissionv1.ValidatingWebhookConfiguration)
		if !ok {
			panic(fmt.Sprintf("object is %T", o))
		}

		// If this is not the ValidatingWebhookConfiguration that we care about,
		// then just return
		if obj.Name != ValidatingWebhookConfigName {
			return nil
		}

		logger := logr.FromContextOrDiscard(ctx).WithValues("name", o.GetName())
		logger.V(4).Info("Reconciling all VirtualMachine StoragePolicyUsages with updated CA Bundle")

		caBundle, err := GetWebhookCABundle(ctx, k8sClient)
		if err != nil {
			logger.Error(err, "Failed to get CA bundle from ValidatingWebhookConfiguration")
			return nil
		}

		spuList := &spqv1.StoragePolicyUsageList{}
		if err = k8sClient.List(ctx, spuList); err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Failed to list StoragePolicyUsages for reconciliation due to ValidatingWebhookConfiguration watch")
			}
			return nil
		}
		// Populate reconcile requests for StoragePolicyQuotas whose StoragePolicyUsage
		// CA Bundle differs from that of the ValidatingWebhookConfiguration
		var requests []reconcile.Request
		for _, spu := range spuList.Items {
			// We only care about a VM's StoragePolicyUsage with an outdated CA Bundle
			if spu.Spec.ResourceExtensionName == StoragePolicyQuotaExtensionName && !bytes.Equal(caBundle, spu.Spec.CABundle) {
				// Get the owning object for this StoragePolicyUsage as we want to
				// reconcile the StoragePolicyQuota in order to update the StoragePolicyUsage
				// CA Bundle
				var owningObj *metav1.OwnerReference
				for i := range spu.OwnerReferences {
					owner := &spu.OwnerReferences[i]
					if owner.Kind == StoragePolicyQuotaKind {
						owningObj = owner
					}
				}

				if owningObj == nil {
					logger.Error(err, "Failed to get the OwnerReference for StoragePolicyUsage %v", client.ObjectKeyFromObject(&spu))
					continue
				}

				requests = append(requests, reconcile.Request{
					NamespacedName: client.ObjectKey{
						Name:      owningObj.Name,
						Namespace: spu.Namespace,
					},
				})
			}
		}

		if len(requests) > 0 {
			logger.V(4).Info("Reconciling VirtualMachine StoragePolicyUsages due to ValidatingWebhookConfiguration watch")
		}

		return requests
	}
}
