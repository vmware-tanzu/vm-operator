// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill

import (
	"context"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	pkgvol "github.com/vmware-tanzu/vm-operator/pkg/util/volumes"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
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

	info, ok := pkgvol.FromContext(ctx)
	if !ok {
		info = pkgvol.GetVolumeInfoFromVM(vm, moVM)
	}

	// Filter out any FCDs.
	info.Disks = pkgvol.FilterOutFCDs(info.Disks...)
	info.Disks = pkgvol.FilterOutEmptyUUIDOrFilename(info.Disks...)

	// Process each unmanaged disk.
	updatedSpec := updateSpecWithUnmanagedDisks(
		ctx,
		vm,
		info)

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
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	info pkgvol.VolumeInfo) bool {

	var (
		addedToSpec bool
		logger      = pkglog.FromContextOrDefault(ctx)
	)

	logger.Info("Get unmanaged volume info",
		"vm.spec.volumes", vm.Spec.Volumes, "info", info)

	for _, di := range info.Disks {
		// Step 1: Look for existing volume entry with the disk target ID.
		if _, exists := info.Volumes[di.Target.String()]; !exists {
			diskName := pkgutil.GeneratePVCName("disk", di.UUID)

			// Step 2: If no such entry exists, add one.
			newVolSpec := vmopv1.VirtualMachineVolume{
				Name:                diskName,
				ControllerBusNumber: ptr.To(info.Controllers[di.ControllerKey].Bus),
				ControllerType:      info.Controllers[di.ControllerKey].Type,
				UnitNumber:          di.UnitNumber,
			}
			logger.Info("Backfilled unmanaged volume to spec",
				"volume", newVolSpec)
			vm.Spec.Volumes = append(vm.Spec.Volumes, newVolSpec)
			addedToSpec = true
		}
	}

	return addedToSpec
}
