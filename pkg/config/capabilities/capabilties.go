// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capabilities

import (
	"context"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	// WCPClusterCapabilitiesConfigMapName is the name of the wcp-cluster-capabilities ConfigMap.
	WCPClusterCapabilitiesConfigMapName = "wcp-cluster-capabilities"

	// WCPClusterCapabilitiesNamespace is the namespace of the wcp-cluster-capabilities
	// ConfigMap.
	WCPClusterCapabilitiesNamespace = "kube-system"

	// TKGMultipleCLCapabilityKey is the name of capability key defined in wcp-cluster-capabilities ConfigMap.
	TKGMultipleCLCapabilityKey = "MultipleCL_For_TKG_Supported"

	// WorkloadIsolationCapabilityKey is the name of capability key defined in wcp-cluster-capabilities ConfigMap.
	WorkloadIsolationCapabilityKey = "Workload_Domain_Isolation_Supported"
)

var (
	// WCPClusterCapabilitiesConfigMapObjKey is the ObjectKey for the capabilities ConfigMap.
	WCPClusterCapabilitiesConfigMapObjKey = client.ObjectKey{
		Name:      WCPClusterCapabilitiesConfigMapName,
		Namespace: WCPClusterCapabilitiesNamespace,
	}
)

// UpdateCapabilitiesFeatures updates the features in the context config.
func UpdateCapabilitiesFeatures(ctx context.Context, data map[string]string) {
	// The SetContext call impacts the configuration available to contexts throughout the process,
	// not just *this* context or its children. Please refer to the pkg/config package for more information
	// on SetContext and its behavior.
	pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
		// TKGMultipleCL is unique in that it is a capability but it predates SVAsyncUpgrade.
		config.Features.TKGMultipleCL = isEnabled(data[TKGMultipleCLCapabilityKey])

		if config.Features.SVAsyncUpgrade {
			// All other capabilities are gated by SVAsyncUpgrade.
			config.Features.WorkloadDomainIsolation = isEnabled(data[WorkloadIsolationCapabilityKey])
		}
	})
}

func isEnabled(v string) bool {
	ok, _ := strconv.ParseBool(v)
	return ok
}
