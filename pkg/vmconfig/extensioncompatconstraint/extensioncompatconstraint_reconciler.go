// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extensioncompatconstraint

import (
	"context"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for the VM's extension compatibility
// constraint set.
func New() vmconfig.Reconciler {
	return reconciler{}
}

func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

func (r reconciler) Name() string { return "extensioncompatconstraint" }

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

// Reconcile compares the VM's current extension compatibility constraint set
// against the desired 6 INVARIANTs and, if they differ, sets the full desired
// set on the ConfigSpec. Since a reconfigure is a full-set-replace, this one
// path self-heals late-binding (VMs created before this capability was
// enabled), post-upgrade drift (the desired set changed), and
// post-snapshot-revert staleness (revert restores ConfigInfo.managedBy via
// the VMX delta chain but leaves the VCDB-backed constraint rows untouched).
func (r reconciler) Reconcile(
	ctx context.Context,
	_ ctrlclient.Client,
	_ *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}

	// No live VM config yet (e.g. during create before the VM exists in
	// vSphere) -- nothing to compare against.
	if moVM.Config == nil {
		return nil
	}

	virtualmachine.UpdateConfigSpecExtensionCompatibilityConstraint(moVM.Config, configSpec)

	return nil
}
