package webconsoleurl

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	appv1a1 "github.com/vmware-tanzu/vm-operator/external/appplatform/api/v1alpha1"
)

const (
	SupervisorServiceObjName      = "supervisor-env-props"
	SupervisorServiceObjNamespace = "vmware-system-supervisor-services"
)

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
