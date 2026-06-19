// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"slices"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	netsetutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/networksettings"
)

// ListNetworkInterfaces lists all the network interfaces for the VM.
func ListNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
) ([]ctrlclient.Object, error) {
	return listInterfacesWithIgnore(vmCtx, client, nil)
}

// ListOrphanedNetworkInterfaces gets the interfaces for a VM that are not
// referenced by the VM Spec.Network.Interfaces.
func ListOrphanedNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	results *NetworkInterfaceResults,
) error {
	// Key on "providerType/name" so that CRs with the same name from different
	// network providers are treated as distinct items.
	expectedInterfaces := sets.Set[string]{}
	for idx := range results.Results {
		r := &results.Results[idx]
		expectedInterfaces.Insert(qualifiedInterfaceKey(r.ObjectProviderType, r.ObjectName))
	}

	interfaces, err := listInterfacesWithIgnore(vmCtx, client, expectedInterfaces)
	if err != nil {
		return err
	}

	results.OrphanedNetworkInterfaces = interfaces
	return nil
}

// qualifiedInterfaceKey returns a unique key for a network interface CR by
// combining its provider type and name. This ensures that CRs with the same
// name from different network providers are treated as distinct items.
func qualifiedInterfaceKey(providerType pkgcfg.NetworkProviderType, name string) string {
	return string(providerType) + "/" + name
}

func listInterfacesWithIgnore(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	ignore sets.Set[string],
) ([]ctrlclient.Object, error) {

	networkTypes, err := netsetutil.GetSupportedProviderTypes(vmCtx, client, vmCtx.VM.Namespace)
	if err != nil {
		return nil, err
	}

	// TODO: Anything v1a2 or later will have the label. Maybe check the created at annotation
	// value and do a more exhaustive list if this is an old VM.
	listOpts := []ctrlclient.ListOption{
		ctrlclient.InNamespace(vmCtx.VM.Namespace),
		ctrlclient.MatchingLabels{VMNameLabel: vmCtx.VM.Name},
	}

	var objs []ctrlclient.Object

	if slices.Contains(networkTypes, pkgcfg.NetworkProviderTypeVDS) {
		var list netopv1alpha1.NetworkInterfaceList
		if err := client.List(vmCtx, &list, listOpts...); err != nil {
			return nil, err
		}

		for i := range list.Items {
			key := qualifiedInterfaceKey(pkgcfg.NetworkProviderTypeVDS, list.Items[i].Name)
			if !ignore.Has(key) && isOwnedBy(vmCtx.VM, &list.Items[i]) {
				objs = append(objs, &list.Items[i])
			}
		}
	}

	if slices.Contains(networkTypes, pkgcfg.NetworkProviderTypeNSXT) {
		var list ncpv1alpha1.VirtualNetworkInterfaceList
		if err := client.List(vmCtx, &list, listOpts...); err != nil {
			return nil, err
		}

		for i := range list.Items {
			key := qualifiedInterfaceKey(pkgcfg.NetworkProviderTypeNSXT, list.Items[i].Name)
			if !ignore.Has(key) && isOwnedBy(vmCtx.VM, &list.Items[i]) {
				objs = append(objs, &list.Items[i])
			}
		}
	}

	if slices.Contains(networkTypes, pkgcfg.NetworkProviderTypeVPC) {
		var list vpcv1alpha1.SubnetPortList
		if err := client.List(vmCtx, &list, listOpts...); err != nil {
			return nil, err
		}

		for i := range list.Items {
			key := qualifiedInterfaceKey(pkgcfg.NetworkProviderTypeVPC, list.Items[i].Name)
			if !ignore.Has(key) && isOwnedBy(vmCtx.VM, &list.Items[i]) {
				objs = append(objs, &list.Items[i])
			}
		}
	}

	return objs, nil
}

func isOwnedBy(vm *vmopv1.VirtualMachine, object ctrlclient.Object) bool {
	for _, ref := range object.GetOwnerReferences() {
		if ref.UID == vm.UID {
			return true
		}
	}
	return false
}
