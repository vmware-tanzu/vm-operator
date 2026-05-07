// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

func mutateOnUpdateDefaultNetworkInterfaceType(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	newVM, _ *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI {
		return false, nil
	}
	return SetDefaultNetworkInterfaceTypesOnUpdate(newVM)
}

// SetDefaultNetworkInterfaceTypesOnUpdate sets spec.network.interfaces[].type to
// VirtualMachineNetworkInterfaceTypeVMXNet3 when unset.
func SetDefaultNetworkInterfaceTypesOnUpdate(vm *vmopv1.VirtualMachine) (bool, error) {
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	vmFeatureVersion := vmopv1util.ParseFeatureVersion(
		vm.Annotations[pkgconst.UpgradedToFeatureVersionAnnotationKey])

	mutated := false
	// The interface Type is backfilled during schema upgrade, so don't set
	// the default until after that is done. The validation webhook's
	// validateFieldsDuringSchemaUpgrade() prevents Type changes during the
	// upgrade window, which complements this gate.
	if vmFeatureVersion.Has(vmopv1util.FeatureVersionTelcoVMServiceAPI) {
		for i := range vm.Spec.Network.Interfaces {
			if vm.Spec.Network.Interfaces[i].Type == "" {
				vm.Spec.Network.Interfaces[i].Type = vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
				mutated = true
			}
		}
	}

	return mutated, nil
}
