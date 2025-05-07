// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func ListOrphanedNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	client ctrlclient.Client,
	results *NetworkInterfaceResults,
) error {

	networkType := pkgcfg.FromContext(vmCtx).NetworkProviderType
	if networkType == "" {
		return fmt.Errorf("no network provider set")
	}

	// TODO: Anything v1a2 or later will have the label. Maybe check the created at annotation
	// value and do a more exhaustive list if this is an old VM.
	listOpts := []ctrlclient.ListOption{
		ctrlclient.InNamespace(vmCtx.VM.Namespace),
		ctrlclient.MatchingLabels{VMNameLabel: vmCtx.VM.Name},
	}

	expectedInterfaceNames := sets.Set[string]{}
	for idx := range results.Results {
		expectedInterfaceNames.Insert(results.Results[idx].ObjectName)
	}

	var objList []ctrlclient.Object

	switch networkType {
	case pkgcfg.NetworkProviderTypeVDS:
		var list netopv1alpha1.NetworkInterfaceList
		if err := client.List(vmCtx, &list, listOpts...); err != nil {
			return err
		}

		for i := range list.Items {
			if !expectedInterfaceNames.Has(list.Items[i].Name) && isOwnedBy(vmCtx.VM, &list.Items[i]) {
				objList = append(objList, &list.Items[i])
			}
		}

	case pkgcfg.NetworkProviderTypeNSXT:
		var list ncpv1alpha1.VirtualNetworkInterfaceList
		if err := client.List(vmCtx, &list, listOpts...); err != nil {
			return err
		}

		for i := range list.Items {
			if !expectedInterfaceNames.Has(list.Items[i].Name) && isOwnedBy(vmCtx.VM, &list.Items[i]) {
				objList = append(objList, &list.Items[i])
			}
		}

	case pkgcfg.NetworkProviderTypeVPC:
		var list vpcv1alpha1.SubnetPortList
		if err := client.List(vmCtx, &list, listOpts...); err != nil {
			return err
		}

		for i := range list.Items {
			if !expectedInterfaceNames.Has(list.Items[i].Name) && isOwnedBy(vmCtx.VM, &list.Items[i]) {
				objList = append(objList, &list.Items[i])
			}
		}

	case pkgcfg.NetworkProviderTypeNamed:
		// No objects.

	default:
		return fmt.Errorf("unsupported network provider envvar value: %q", networkType)
	}

	results.OrphanedNetworkInterfaces = objList
	return nil
}

func isOwnedBy(vm *vmopv1.VirtualMachine, object ctrlclient.Object) bool {
	for _, ref := range object.GetOwnerReferences() {
		if ref.UID == vm.UID {
			return true
		}
	}
	return false
}
