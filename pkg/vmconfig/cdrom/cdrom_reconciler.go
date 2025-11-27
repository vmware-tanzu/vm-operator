// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package cdrom

import (
	"context"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// Reconcile configures the VM's CD-ROM devices.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

func New() vmconfig.Reconciler {
	return reconciler{}
}

func (r reconciler) Name() string {
	return "cdrom"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}

	vcVM := object.NewVirtualMachine(vimClient, moVM.Reference())

	return virtualmachine.UpdateCdromDeviceChangesWithSharedDisks(
		ctx,
		k8sClient,
		vm,
		vcVM,
		moVM,
		configSpec)
}
