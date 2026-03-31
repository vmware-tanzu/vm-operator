// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"testing"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func TestSetDefaultNetworkInterfaceTypesOnCreate_DefaultsVMXNet3(t *testing.T) {
	t.Parallel()
	ctx := &pkgctx.WebhookRequestContext{WebhookContext: &pkgctx.WebhookContext{}}
	vm := &vmopv1.VirtualMachine{}
	vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
		Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
	}
	ok, err := SetDefaultNetworkInterfaceTypesOnCreate(ctx, nil, vm)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mutation")
	}
	if vm.Spec.Network.Interfaces[0].Type != vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3 {
		t.Fatalf("got %q", vm.Spec.Network.Interfaces[0].Type)
	}
}

func TestSetDefaultNetworkInterfaceTypesOnUpdate_PreservesOldType(t *testing.T) {
	t.Parallel()
	ctx := &pkgctx.WebhookRequestContext{WebhookContext: &pkgctx.WebhookContext{}}
	newVM := &vmopv1.VirtualMachine{}
	newVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
		Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{{Name: "eth0"}},
	}
	oldVM := &vmopv1.VirtualMachine{}
	oldVM.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
		Interfaces: []vmopv1.VirtualMachineNetworkInterfaceSpec{
			{Name: "eth0", Type: vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV},
		},
	}
	ok, err := SetDefaultNetworkInterfaceTypesOnUpdate(ctx, nil, newVM, oldVM)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected mutation")
	}
	if newVM.Spec.Network.Interfaces[0].Type != vmopv1.VirtualMachineNetworkInterfaceTypeSRIOV {
		t.Fatalf("got %q", newVM.Spec.Network.Interfaces[0].Type)
	}
}
