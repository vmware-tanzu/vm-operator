// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networksettings

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

const (
	// networkSettingsName is the well known name of the NetworkSettings
	// in each namespace.
	networkSettingsName = "default"

	// WorkloadNetworkConfigurationName is the well known name of the cluster-scoped
	// WorkloadNetworkConfiguration singleton.
	WorkloadNetworkConfigurationName = "default"
)

var (
	// ErrNetworkSettingsNotFound is returned when the NetworkSettings CR for the
	// namespace does not exist. This CR is supposed to be created "immediately" after
	// namespace creation, so it is an unfortunate race if actually observed and
	// should be transient.
	ErrNetworkSettingsNotFound = errors.New("NetworkSettings has not been created for this namespace")

	// ErrWorkloadNetworkConfigurationNotFound is returned when the cluster-scoped
	// WorkloadNetworkConfiguration CR does not exist.
	ErrWorkloadNetworkConfigurationNotFound = errors.New("WorkloadNetworkConfiguration has not been created")
)

// GetProviderType returns the NetworkProviderType for the given namespace. When the
// PerNamespaceNetworkProvider capability is not enabled, return provider type that is
// specified globally by the NETWORK_PROVIDER envvar. When the capability is enabled
// fetch the NetworkSettings in the namespace and return the corresponding
// NetworkProviderType value.
func GetProviderType(
	ctx context.Context,
	reader ctrlclient.Reader,
	namespace string) (pkgcfg.NetworkProviderType, error) {

	if !pkgcfg.FromContext(ctx).Features.PerNamespaceNetworkProvider {
		return pkgcfg.FromContext(ctx).NetworkProviderType, nil
	}

	ns, err := getNetworkSettings(ctx, reader, namespace)
	if err != nil {
		return "", err
	}

	return NetworkProviderToType(ns.Provider)
}

// GetSupportedProviderTypes returns the supported NetworkProviderTypes for the given
// namespace. In a namespace that underwent network migration, the prior (legacy)
// network provider is also supported.
func GetSupportedProviderTypes(
	ctx context.Context,
	reader ctrlclient.Reader,
	namespace string) ([]pkgcfg.NetworkProviderType, error) {

	t := make([]pkgcfg.NetworkProviderType, 0, 2)

	if !pkgcfg.FromContext(ctx).Features.PerNamespaceNetworkProvider {
		return append(t, pkgcfg.FromContext(ctx).NetworkProviderType), nil
	}

	ns, err := getNetworkSettings(ctx, reader, namespace)
	if err != nil {
		return nil, err
	}

	defaultProvider, err := NetworkProviderToType(ns.Provider)
	if err != nil {
		return nil, err
	}

	t = append(t, defaultProvider)

	if ns.LegacyProvider != "" {
		legacyProvider, err := NetworkProviderToType(ns.LegacyProvider)
		if err != nil {
			return nil, err
		}
		t = append(t, legacyProvider)
	}

	return t, nil
}

// GetClusterSupportedProviderTypes returns the NetworkProviderTypes declared as
// available on the cluster, as specified by the WorkloadNetworkConfiguration
// singleton CR. When the WorkloadNetworkConfiguration capability is disabled,
// it returns the global config NetworkProviderType.
func GetClusterSupportedProviderTypes(
	ctx context.Context,
	reader ctrlclient.Reader) ([]pkgcfg.NetworkProviderType, error) {

	if !pkgcfg.FromContext(ctx).Features.WorkloadNetworkConfiguration {
		return []pkgcfg.NetworkProviderType{
			pkgcfg.FromContext(ctx).NetworkProviderType,
		}, nil
	}

	wnc, err := getWorkloadNetworkConfiguration(ctx, reader)
	if err != nil {
		return nil, err
	}

	return GetClusterSupportedProviderTypesFromWNC(wnc)
}

// GetClusterSupportedProviderTypesFromWNC returns the supported network provider
// types from the WorkloadNetworkConfiguration CR.
func GetClusterSupportedProviderTypesFromWNC(
	wnc *netopv1alpha1.WorkloadNetworkConfiguration) ([]pkgcfg.NetworkProviderType, error) {

	t := make([]pkgcfg.NetworkProviderType, 0, 2)

	for _, entry := range wnc.Spec.Providers {
		pt, err := NetworkProviderToType(entry.Type)
		if err != nil {
			return nil, err
		}
		t = append(t, pt)
	}

	return t, nil
}

// GetClusterSupportedProviderTypesFromConfig gets the cluster supported network
// providers from the Config in the context.
func GetClusterSupportedProviderTypesFromConfig(
	ctx context.Context) []pkgcfg.NetworkProviderType {

	providerTypes := pkgcfg.StringToSlice(pkgcfg.FromContext(ctx).ClusterNetworkProviderTypes)
	if len(providerTypes) == 0 {
		pkglog.FromContextOrDefault(ctx).Info(
			"No cluster network provider types set in context config")
		return nil
	}

	t := make([]pkgcfg.NetworkProviderType, 0, 2)
	for _, p := range providerTypes {
		t = append(t, pkgcfg.NetworkProviderType(p))
	}

	return t
}

func getNetworkSettings(
	ctx context.Context,
	reader ctrlclient.Reader,
	namespace string) (*netopv1alpha1.NetworkSettings, error) {

	var ns netopv1alpha1.NetworkSettings
	err := reader.Get(ctx, ctrlclient.ObjectKey{Name: networkSettingsName, Namespace: namespace}, &ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrNetworkSettingsNotFound
		}
		return nil, err
	}

	return &ns, nil
}

func getWorkloadNetworkConfiguration(
	ctx context.Context,
	reader ctrlclient.Reader) (*netopv1alpha1.WorkloadNetworkConfiguration, error) {

	var wnc netopv1alpha1.WorkloadNetworkConfiguration
	err := reader.Get(ctx, ctrlclient.ObjectKey{Name: WorkloadNetworkConfigurationName}, &wnc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrWorkloadNetworkConfigurationNotFound
		}
		return nil, err
	}

	return &wnc, nil
}

// NetworkProviderToType maps a netopv1alpha1.NetworkProvider value to the
// pkgcfg.NetworkProviderType used throughout vm-operator.
func NetworkProviderToType(p netopv1alpha1.NetworkProvider) (pkgcfg.NetworkProviderType, error) {
	switch p {
	case netopv1alpha1.NetworkProviderVSphereDistributed:
		return pkgcfg.NetworkProviderTypeVDS, nil
	case netopv1alpha1.NetworkProviderNSXTier1:
		return pkgcfg.NetworkProviderTypeNSXT, nil
	case netopv1alpha1.NetworkProviderVPC:
		return pkgcfg.NetworkProviderTypeVPC, nil
	default:
		return "", fmt.Errorf("unknown network provider %q", p)
	}
}

// TypeToNetworkProvider maps a pkgcfg.NetworkProviderType value to the
// netopv1alpha1.NetworkProvider used by the NetworkSettings CR.
func TypeToNetworkProvider(t pkgcfg.NetworkProviderType) (netopv1alpha1.NetworkProvider, error) {
	switch t {
	case pkgcfg.NetworkProviderTypeVDS:
		return netopv1alpha1.NetworkProviderVSphereDistributed, nil
	case pkgcfg.NetworkProviderTypeNSXT:
		return netopv1alpha1.NetworkProviderNSXTier1, nil
	case pkgcfg.NetworkProviderTypeVPC:
		return netopv1alpha1.NetworkProviderVPC, nil
	default:
		return "", fmt.Errorf("unknown network provider type %q", t)
	}
}
