// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
)

func mutateOnCreateDefaultNetworkInterfaceType(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.VMExtraConfig {
		return false, nil
	}
	return SetDefaultNetworkInterfaceTypesOnCreate(ctx, client, vm)
}

func mutateOnUpdateDefaultNetworkInterfaceType(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	newVM, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.VMExtraConfig {
		return false, nil
	}
	return SetDefaultNetworkInterfaceTypesOnUpdate(ctx, client, newVM, oldVM)
}

// SetDefaultNetworkInterfaceTypesOnCreate sets spec.network.interfaces[].type on
// create when unset, preferring types from the VM class ConfigSpec ethernet
// devices, otherwise VirtualMachineNetworkInterfaceTypeVMXNet3.
func SetDefaultNetworkInterfaceTypesOnCreate(
	ctx *pkgctx.WebhookRequestContext,
	client ctrlclient.Client,
	vm *vmopv1.VirtualMachine) (bool, error) {

	return virtualmachine.FillEmptyNetworkInterfaceTypesFromClass(ctx, client, vm)
}

// SetDefaultNetworkInterfaceTypesOnUpdate sets spec.network.interfaces[].type
// when the new object leaves it empty: it copies the prior value, or defaults
// to VirtualMachineNetworkInterfaceTypeVMXNet3.
func SetDefaultNetworkInterfaceTypesOnUpdate(
	_ *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	newVM, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if newVM.Spec.Network == nil || len(newVM.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	mutated := false
	for i := range newVM.Spec.Network.Interfaces {
		if newVM.Spec.Network.Interfaces[i].Type != "" {
			continue
		}
		t := vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3
		if oldVM.Spec.Network != nil && i < len(oldVM.Spec.Network.Interfaces) &&
			oldVM.Spec.Network.Interfaces[i].Type != "" {

			t = oldVM.Spec.Network.Interfaces[i].Type
		}
		newVM.Spec.Network.Interfaces[i].Type = t
		mutated = true
	}
	return mutated, nil
}
