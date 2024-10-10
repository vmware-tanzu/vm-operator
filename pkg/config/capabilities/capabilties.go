// Copyright (c) 2024 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.

package capabilities

import (
	"context"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	capv1 "github.com/vmware-tanzu/vm-operator/external/capabilities/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
)

const (
	// ConfigMapName is the name of the capabilities ConfigMap.
	ConfigMapName = "wcp-cluster-capabilities"

	// ConfigMapNamespace is the namespace of the capabilities ConfigMap.
	ConfigMapNamespace = "kube-system"

	// CapabilitiesName is the name of the cluster-scoped Capabilities resource
	// used to discover Supervisor capabilities.
	CapabilitiesName = "supervisor-capabilities"

	// CapabilityKeyBringYourOwnKeyProvider is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyBringYourOwnKeyProvider = "Bring_Your_Own_Key_Provider_Supported"

	// CapabilityKeyTKGMultipleContentLibraries is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyTKGMultipleContentLibraries = "MultipleCL_For_TKG_Supported"

	// CapabilityKeyWorkloadIsolation is the name of capability key
	// defined in the capabilities ConfigMap and/or CRD.
	CapabilityKeyWorkloadIsolation = "Workload_Domain_Isolation_Supported"
)

var (
	// CapabilitiesKey is the ObjectKey for the cluster-scoped Capabilities
	// resource.
	CapabilitiesKey = ctrlclient.ObjectKey{Name: CapabilitiesName}

	// ConfigMapKey is the ObjectKey for the capabilities ConfigMap.
	ConfigMapKey = ctrlclient.ObjectKey{
		Name:      ConfigMapName,
		Namespace: ConfigMapNamespace,
	}
)

// UpdateCapabilities updates the capabilities in the context from either the
// capabilities ConfigMap or cluster-scoped CRD resource.
func UpdateCapabilities(
	ctx context.Context,
	k8sClient ctrlclient.Client) (bool, error) {

	if pkgcfg.FromContext(ctx).Features.SVAsyncUpgrade {
		var obj capv1.Capabilities
		if err := k8sClient.Get(ctx, CapabilitiesKey, &obj); err != nil {
			return false, err
		}
		return UpdateCapabilitiesFeatures(ctx, obj), nil
	}

	var obj corev1.ConfigMap
	if err := k8sClient.Get(ctx, ConfigMapKey, &obj); err != nil {
		return false, err
	}
	return UpdateCapabilitiesFeatures(ctx, obj), nil
}

type updateCapTypes interface {
	map[string]string | corev1.ConfigMap | capv1.Capabilities
}

// UpdateCapabilitiesFeatures updates the features in the context config.
//
// Please note, this call impacts the configuration available to contexts
// throughout the process, not just *this* context or its children.
// Please refer to the pkg/config package for more information on SetContext
// and its behavior.
//
// The return value indicates if any of the features changed.
func UpdateCapabilitiesFeatures[T updateCapTypes](
	ctx context.Context,
	obj T) bool {

	var (
		logger  = logr.FromContextOrDiscard(ctx)
		oldFeat = pkgcfg.FromContext(ctx).Features
	)

	switch tObj := (any)(obj).(type) {
	case map[string]string:
		updateCapabilitiesFeaturesFromMap(ctx, tObj)
	case corev1.ConfigMap:
		updateCapabilitiesFeaturesFromMap(ctx, tObj.Data)
	case capv1.Capabilities:
		updateCapabilitiesFeaturesFromCRD(ctx, tObj)
	}

	if newFeat := pkgcfg.FromContext(ctx).Features; oldFeat != newFeat {
		logger.Info(
			"Updated features from capabilities",
			"diff", cmp.Diff(newFeat, oldFeat))

		return true
	}

	return false
}

func updateCapabilitiesFeaturesFromMap(
	ctx context.Context,
	data map[string]string) {

	pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
		// TKGMultipleCL is unique in that it is a capability but it predates
		// SVAsyncUpgrade.
		config.Features.TKGMultipleCL = isEnabled(data[CapabilityKeyTKGMultipleContentLibraries])

		if config.Features.SVAsyncUpgrade {
			// All other capabilities are gated by SVAsyncUpgrade.
			config.Features.WorkloadDomainIsolation = isEnabled(data[CapabilityKeyWorkloadIsolation])
		}
	})
}

func updateCapabilitiesFeaturesFromCRD(
	ctx context.Context,
	obj capv1.Capabilities) {

	var (
		byok                    *bool
		tkgMultipleCL           *bool
		workloadDomainIsolation *bool
	)

	for capName, capStatus := range obj.Status.Supervisor {
		switch capName {
		case CapabilityKeyBringYourOwnKeyProvider:
			setCap(&byok, capStatus.Activated)
		case CapabilityKeyTKGMultipleContentLibraries:
			setCap(&tkgMultipleCL, capStatus.Activated)
		case CapabilityKeyWorkloadIsolation:
			setCap(&workloadDomainIsolation, capStatus.Activated)
		}
	}

	pkgcfg.SetContext(ctx, func(config *pkgcfg.Config) {
		if byok != nil {
			config.Features.BringYourOwnEncryptionKey = *byok
		}
		if tkgMultipleCL != nil {
			config.Features.TKGMultipleCL = *tkgMultipleCL
		}
		if workloadDomainIsolation != nil {
			config.Features.WorkloadDomainIsolation = *workloadDomainIsolation
		}
	})
}

func setCap(dst **bool, val bool) {
	*dst = &val
}

func isEnabled(v string) bool {
	ok, _ := strconv.ParseBool(v)
	return ok
}
