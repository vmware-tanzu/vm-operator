package webconsoleurl

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/external/appplatform/api/vmw_v1alpha1"
)

const (
	supervisorServiceObjName      = "supervisor-env-props"
	supervisorServiceObjNamespace = "vmware-system-supervisor-services"
)

// ProxyServiceDNSName retrieves the first API server DNS name using the provided client by
// querying the appplatform CRD, if one exists.
func ProxyServiceDNSName(ctx context.Context, r client.Client) (string, error) {
	proxySvc := &vmw_v1alpha1.SupervisorProperties{}
	proxySvcObjectKey := client.ObjectKey{Name: supervisorServiceObjName, Namespace: supervisorServiceObjNamespace}

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
