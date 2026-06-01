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
)

const defaultNetworkSettingsName = "default"

var (
	// ErrNetworkSettingsNotFound is returned when the NetworkSettings CR for the
	// namespace does not exist. This CR is supposed to be created "immediately" after
	// namespace creation, so it is an unfortunate race if actually observed and
	// should be transient.
	ErrNetworkSettingsNotFound = errors.New("NetworkSettings has not been created for this namespace")
)

// GetProviderType returns the NetworkProviderType for the given namespace. When the
// PerNamespaceNetworkProvider capability is not enabled, return provide type that is
// specified by the NETWORK_PROVIDER envvar. When the capability is enabled fetch the
// NetworkSettings in the namespace and return the corresponding NetworkProviderType
// value.
func GetProviderType(
	ctx context.Context,
	reader ctrlclient.Reader,
	namespace string) (pkgcfg.NetworkProviderType, error) {

	if !pkgcfg.FromContext(ctx).Features.PerNamespaceNetworkProvider {
		return pkgcfg.FromContext(ctx).NetworkProviderType, nil
	}

	var ns netopv1alpha1.NetworkSettings
	err := reader.Get(ctx, ctrlclient.ObjectKey{Name: defaultNetworkSettingsName, Namespace: namespace}, &ns)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", ErrNetworkSettingsNotFound
		}
		return "", err
	}

	return providerToType(ns.Provider)
}

// providerToType maps a netopv1alpha1.NetworkProvider value to the
// pkgcfg.NetworkProviderType used throughout vm-operator.
func providerToType(p netopv1alpha1.NetworkProvider) (pkgcfg.NetworkProviderType, error) {
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
