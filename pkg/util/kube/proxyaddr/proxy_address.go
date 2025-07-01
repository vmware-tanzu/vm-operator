// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package proxyaddr

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
)

const (
	SupervisorServiceObjName      = "supervisor-env-props"
	SupervisorServiceObjNamespace = "vmware-system-supervisor-services"

	ProxyAddrServiceName      = "kube-apiserver-lb-svc"
	ProxyAddrServiceNamespace = "kube-system"
)

// +kubebuilder:rbac:groups=appplatform.vmware.com,resources=supervisorproperties,verbs=get;list;watch

// ProxyAddress first attempts to get the proxy address through the API Server DNS Names.
// If that is unset, though, fall back to using the virtual IP.
func ProxyAddress(ctx context.Context, r client.Client) (string, error) {
	// Attempt to use the API Server DNS Names to get the proxy address.
	proxyAddress, err := proxyServiceDNSName(ctx, r)
	if err != nil {
		return "", fmt.Errorf("failed to get proxy service URL: %w", err)
	}

	// If no API Server DNS Name exists, fall back to using the virtual IP.
	if proxyAddress == "" {
		return proxyAddressFromVirtualIP(ctx, r)
	}

	return proxyAddress, nil
}

// proxyServiceDNSName retrieves the first API server DNS name using the provided client by
// querying the appplatform CRD, if one exists.
func proxyServiceDNSName(ctx context.Context, r client.Client) (string, error) {
	proxySvc := &appv1a1.SupervisorProperties{}
	proxySvcObjectKey := client.ObjectKey{Name: SupervisorServiceObjName, Namespace: SupervisorServiceObjNamespace}

	if err := r.Get(ctx, proxySvcObjectKey, proxySvc); err != nil {
		// If the API is not found at all, then don't return an error
		if apierrors.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get proxy address service %s: %w", proxySvcObjectKey, err)
	}

	if len(proxySvc.Spec.APIServerDNSNames) == 0 {
		return "", nil
	}

	// Return the first FQDN by default
	return proxySvc.Spec.APIServerDNSNames[0], nil
}

// proxyAddressFromVirtualIP retrieves the virtual IP, which will be used as the Proxy Address.
func proxyAddressFromVirtualIP(ctx context.Context, r client.Client) (string, error) {
	proxySvc := &corev1.Service{}
	proxySvcObjectKey := client.ObjectKey{Name: ProxyAddrServiceName, Namespace: ProxyAddrServiceNamespace}

	if err := r.Get(ctx, proxySvcObjectKey, proxySvc); err != nil {
		return "", fmt.Errorf("failed to get proxy address service  %s: %w", proxySvcObjectKey, err)
	}

	if len(proxySvc.Status.LoadBalancer.Ingress) == 0 {
		return "", fmt.Errorf("no ingress found for proxy address service %s", proxySvcObjectKey)
	}

	return proxySvc.Status.LoadBalancer.Ingress[0].IP, nil
}
