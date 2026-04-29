// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func mutateOnUpdateDefaultNetworkInterfaceType(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	newVM, _ *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI {
		return false, nil
	}
	return SetDefaultNetworkInterfaceTypes(newVM)
}

// SetDefaultNetworkInterfaceTypes sets spec.network.interfaces[].type to
// VirtualMachineNetworkInterfaceTypeVMXNet3 when unset.
func SetDefaultNetworkInterfaceTypes(vm *vmopv1.VirtualMachine) (bool, error) {
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	mutated := false
	for i := range vm.Spec.Network.Interfaces {
		if vm.Spec.Network.Interfaces[i].Type == "" {
			vm.Spec.Network.Interfaces[i].Type = vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
			mutated = true
		}
	}
	return mutated, nil
}
