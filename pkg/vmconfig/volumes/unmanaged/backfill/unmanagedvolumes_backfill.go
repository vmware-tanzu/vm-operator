// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill

import (
	"context"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
	unmanagedvolsutil "github.com/vmware-tanzu/vm-operator/pkg/vmconfig/volumes/unmanaged/util"
)

// Condition is the name of the condition that stores the result.
const Condition = "VirtualMachineUnmanagedVolumesBackfill"

// ErrPendingBackfill is returned from Reconcile to indicate to exit the VM
// reconcile workflow early.
var ErrPendingBackfill = pkgerr.NoRequeueNoErr(
	"has unmanaged volumes pending backfill")

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for backfilling a VM's unmanaged volumes into
// the VM spec.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "unmanagedvolumes-backfill"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

// Reconcile ensures all unmanaged volumes are backfilled into the VM spec.
func Reconcile(
	ctx context.Context,
	_ ctrlclient.Client,
	_ *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	_ *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, nil, nil, vm, moVM, nil)
}

// Reconcile ensures all unmanaged volumes are backfilled into the VM spec.
func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
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

	if pkgcond.IsTrue(vm, Condition) {
		return nil
	}

	// Process each unmanaged disk.
	updatedSpec := updateSpecWithUnmanagedDisks(
		vm,
		unmanagedvolsutil.GetUnmanagedVolumeInfo(vm, moVM))

	if updatedSpec {
		pkgcond.MarkFalse(
			vm,
			Condition,
			"Pending",
			"")
		return ErrPendingBackfill
	}

	pkgcond.MarkTrue(vm, Condition)
	return nil
}

func updateSpecWithUnmanagedDisks(
	vm *vmopv1.VirtualMachine,
	info unmanagedvolsutil.UnmanagedVolumeInfo) bool {

	var addedToSpec bool

	for _, di := range info.Disks {
		// Step 1: Look for existing volume entry with the provided UUID.
		if _, exists := info.Volumes[di.UUID]; !exists {
			// Step 2: If no such entry exists, add one.
			vm.Spec.Volumes = append(
				vm.Spec.Volumes,
				vmopv1.VirtualMachineVolume{
					Name: pkgutil.GeneratePVCName(
						"disk",
						di.UUID,
					),
					VirtualMachineVolumeSource: vmopv1.VirtualMachineVolumeSource{
						PersistentVolumeClaim: &vmopv1.PersistentVolumeClaimVolumeSource{
							PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: pkgutil.GeneratePVCName(
									vm.Name,
									di.UUID,
								),
							},
							UnmanagedVolumeClaim: &vmopv1.UnmanagedVolumeClaimVolumeSource{
								Type: vmopv1.UnmanagedVolumeClaimVolumeTypeFromVM,
								Name: di.UUID,
								UUID: di.UUID,
							},
							ControllerBusNumber: ptr.To(info.Controllers[di.ControllerKey].Bus),
							ControllerType:      info.Controllers[di.ControllerKey].Type,
							UnitNumber:          di.UnitNumber,
						},
					},
				},
			)
			addedToSpec = true
		}
	}

	return addedToSpec
}
