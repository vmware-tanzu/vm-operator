package webconsoleurl

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	SupervisorServiceObjName      = "supervisor-env-props"
	SupervisorServiceObjNamespace = "vmware-system-supervisor-services"

	ProxyAddrServiceName      = "kube-apiserver-lb-svc"
	ProxyAddrServiceNamespace = "kube-system"
)

// ProxyAddress first attempts to get the proxy address through the API Server DNS Names.
// If that is unset, though, fall back to using the virtual IP.
func ProxyAddress(ctx context.Context, r client.Client) (string, error) {
	if !pkgcfg.FromContext(ctx).Features.SimplifiedEnablement {
		return ProxyAddressFromVirtualIP(ctx, r)
	}

	// Attempt to use the API Server DNS Names to get the proxy address.
	proxyAddress, err := ProxyServiceDNSName(ctx, r)
	if err != nil {
		return "", fmt.Errorf("failed to get proxy service URL: %w", err)
	}

	// If no API Server DNS Name exists, fall back to using the virtual IP.
	if len(proxyAddress) == 0 {
		return ProxyAddressFromVirtualIP(ctx, r)
	}

	return proxyAddress, nil
}

// ProxyServiceDNSName retrieves the first API server DNS name using the provided client by
// querying the appplatform CRD, if one exists.
func ProxyServiceDNSName(ctx context.Context, r client.Client) (string, error) {
	proxySvc := &appv1a1.SupervisorProperties{}
	proxySvcObjectKey := client.ObjectKey{Name: SupervisorServiceObjName, Namespace: SupervisorServiceObjNamespace}

	err := r.Get(ctx, proxySvcObjectKey, proxySvc)

	if err != nil {
		return "", fmt.Errorf("failed to get proxy address service  %s: %w", proxySvcObjectKey, err)
	}

	if len(proxySvc.Spec.APIServerDNSNames) == 0 {
		return "", nil
	}

	// Return the first FQDN by default
	return proxySvc.Spec.APIServerDNSNames[0], nil
}

// ProxyAddressFromVirtualIP retrieves the virtual IP, which will be used as the Proxy Address.
func ProxyAddressFromVirtualIP(ctx context.Context, r client.Client) (string, error) {
	proxySvc := &corev1.Service{}
	proxySvcObjectKey := client.ObjectKey{Name: ProxyAddrServiceName, Namespace: ProxyAddrServiceNamespace}
	err := r.Get(ctx, proxySvcObjectKey, proxySvc)
	if err != nil {
		return "", fmt.Errorf("failed to get proxy address service  %s: %w", proxySvcObjectKey, err)
	}
	if len(proxySvc.Status.LoadBalancer.Ingress) == 0 {
		return "", fmt.Errorf("no ingress found for proxy address service %s", proxySvcObjectKey)
	}

	return proxySvc.Status.LoadBalancer.Ingress[0].IP, nil
}
