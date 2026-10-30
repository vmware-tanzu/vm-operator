// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	netsetutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube/networksettings"
)

// ListNetworkInterfaces lists all the network interfaces for the VM.
func ListNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
) ([]ctrlclient.Object, error) {
	return ListOrphanedNetworkInterfaces(vmCtx, client, nil)
}

// ListOrphanedNetworkInterfaces gets the interfaces for a VM that are not
// referenced by the VM Spec.Network.Interfaces.
func ListOrphanedNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	devices []Device,
) ([]ctrlclient.Object, error) {

	managed, _, err := ListAllOrphanedNetworkInterfaces(vmCtx, client, devices)
	return managed, err
}

// ListAllOrphanedNetworkInterfaces gets the interfaces associated with a VM
// that are not referenced by the VM Spec.Network.Interfaces, in a single
// listing pass. The managed interfaces are the ones VM Operator created and
// owns, and should be deleted. The external interfaces are the ones VM Operator
// must only release. External interfaces are only returned when the
// BringYourOwnNetworkInterface capability is enabled.
func ListAllOrphanedNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	devices []Device,
) (managed, external []ctrlclient.Object, err error) {

	// Key on "providerType/name" so that CRs with the same name from different
	// network providers are treated as distinct items.
	expectedInterfaces := sets.Set[string]{}
	for idx := range devices {
		d := &devices[idx]
		expectedInterfaces.Insert(qualifiedInterfaceKey(d.ProviderType, d.ObjectName()))
	}

	networkTypes, err := netsetutil.GetSupportedProviderTypes(vmCtx, client, vmCtx.VM.Namespace)
	if err != nil {
		return nil, nil, err
	}

	objs, err := listVMNetworkInterfaces(vmCtx, client, vmCtx.VM, networkTypes, false)
	if err != nil {
		return nil, nil, err
	}

	withExternal := pkgcfg.FromContext(vmCtx).Features.BringYourOwnNetworkInterface

	for _, o := range objs {
		if expectedInterfaces.Has(qualifiedInterfaceKey(o.providerType, o.obj.GetName())) {
			continue
		}
		switch {
		case IsExternalNetworkInterface(o.obj):
			if withExternal {
				external = append(external, o.obj)
			}
		case isOwnedBy(vmCtx.VM, o.obj):
			managed = append(managed, o.obj)
		}
	}

	return managed, external, nil
}

// listExternalNetworkInterfaces lists all the external network interfaces
// associated with the VM, for the network providers enabled in the cluster.
// The cluster's provider types are used instead of the VM namespace's
// NetworkSettings so that this works while the namespace is being deleted.
// When the cluster's provider types are not known, every provider type is
// listed, skipping any whose CRD is not installed.
func listExternalNetworkInterfaces(
	ctx context.Context,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
) ([]ctrlclient.Object, error) {

	networkTypes := netsetutil.GetClusterSupportedProviderTypesFromConfig(ctx)
	if len(networkTypes) == 0 {
		networkTypes = []pkgcfg.NetworkProviderType{
			pkgcfg.NetworkProviderTypeVDS,
			pkgcfg.NetworkProviderTypeNSXT,
			pkgcfg.NetworkProviderTypeVPC,
		}
	}

	objs, err := listVMNetworkInterfaces(ctx, client, vm, networkTypes, true)
	if err != nil {
		return nil, err
	}

	var external []ctrlclient.Object
	for _, o := range objs {
		if IsExternalNetworkInterface(o.obj) {
			external = append(external, o.obj)
		}
	}

	return external, nil
}

// qualifiedInterfaceKey returns a unique key for a network interface CR by
// combining its provider type and name. This ensures that CRs with the same
// name from different network providers are treated as distinct items.
func qualifiedInterfaceKey(providerType pkgcfg.NetworkProviderType, name string) string {
	return string(providerType) + "/" + name
}

type providerNetworkInterface struct {
	providerType pkgcfg.NetworkProviderType
	obj          ctrlclient.Object
}

// listVMNetworkInterfaces lists the network interface CRs labeled with the
// VM's name for each of the given network provider types. When ignoreNoMatch
// is true, a provider whose CRD is not installed is skipped.
func listVMNetworkInterfaces(
	ctx context.Context,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	networkTypes []pkgcfg.NetworkProviderType,
	ignoreNoMatch bool,
) ([]providerNetworkInterface, error) {

	// TODO: Anything v1a2 or later will have the label. Maybe check the created at annotation
	// value and do a more exhaustive list if this is an old VM.
	listOpts := []ctrlclient.ListOption{
		ctrlclient.InNamespace(vm.Namespace),
		ctrlclient.MatchingLabels{pkgconst.NetworkInterfaceVMNameLabel: vm.Name},
	}

	list := func(l ctrlclient.ObjectList) (bool, error) {
		if err := client.List(ctx, l, listOpts...); err != nil {
			if ignoreNoMatch && meta.IsNoMatchError(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	var objs []providerNetworkInterface

	if slices.Contains(networkTypes, pkgcfg.NetworkProviderTypeVDS) {
		var l netopv1alpha1.NetworkInterfaceList
		if ok, err := list(&l); err != nil {
			return nil, err
		} else if ok {
			for i := range l.Items {
				objs = append(objs, providerNetworkInterface{pkgcfg.NetworkProviderTypeVDS, &l.Items[i]})
			}
		}
	}

	if slices.Contains(networkTypes, pkgcfg.NetworkProviderTypeNSXT) {
		var l ncpv1alpha1.VirtualNetworkInterfaceList
		if ok, err := list(&l); err != nil {
			return nil, err
		} else if ok {
			for i := range l.Items {
				objs = append(objs, providerNetworkInterface{pkgcfg.NetworkProviderTypeNSXT, &l.Items[i]})
			}
		}
	}

	if slices.Contains(networkTypes, pkgcfg.NetworkProviderTypeVPC) {
		var l vpcv1alpha1.SubnetPortList
		if ok, err := list(&l); err != nil {
			return nil, err
		} else if ok {
			for i := range l.Items {
				objs = append(objs, providerNetworkInterface{pkgcfg.NetworkProviderTypeVPC, &l.Items[i]})
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
