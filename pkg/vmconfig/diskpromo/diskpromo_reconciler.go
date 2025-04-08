// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package diskpromo

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for a VM's disk promotion state.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "diskpromo"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error) error {

	return nil
}

var doPromote bool

// Reconcile performs an online promotion of any of a VM's linked-clone disks to
// full clones. This reconciler is a no-op for VMs that do not yet exists and/or
// are not powered on.
func (r reconciler) Reconcile(
	ctx context.Context,
	_ ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	_ *vimtypes.VirtualMachineConfigSpec) error {

	if moVM.Config == nil {
		// Skip VMs that are not yet created.
		return nil
	}

	if vm.Status.TaskID != "" {
		// Skip VMs with outstanding tasks.
		return nil
	}

	if vm.Spec.PromoteDisksMode == vmopv1.VirtualMachinePromoteDisksModeDisabled {
		// Skip VMs that do not request promotion.
		pkgcond.Delete(vm, vmopv1.VirtualMachineDiskPromotionSynced)
		return nil
	}

	// Find the VirtualDisks for this VM.
	var (
		childDisks []vimtypes.VirtualDisk
		devices    = object.VirtualDeviceList(moVM.Config.Hardware.Device)
		allDisks   = devices.SelectByType(&vimtypes.VirtualDisk{})
	)

	// Find any classic, file-based disks that have parent backings.
	for i := range allDisks {
		d := allDisks[i].(*vimtypes.VirtualDisk)
		if d.VDiskId == nil { // Skip FCDs
			switch tBack := d.Backing.(type) {
			case *vimtypes.VirtualDiskFlatVer2BackingInfo:
				if tBack.Parent != nil {
					childDisks = append(childDisks, *d)
				}
			case *vimtypes.VirtualDiskSeSparseBackingInfo:
				if tBack.Parent != nil {
					childDisks = append(childDisks, *d)
				}
			case *vimtypes.VirtualDiskSparseVer2BackingInfo:
				if tBack.Parent != nil {
					childDisks = append(childDisks, *d)
				}
			}
		}
	}

	if len(childDisks) == 0 {
		// Skip VMs that do not have any child disks to promote.
		return nil
	}

	switch vm.Spec.PromoteDisksMode {
	case vmopv1.VirtualMachinePromoteDisksModeOnline:
		if moVM.Snapshot != nil && moVM.Snapshot.CurrentSnapshot != nil {
			// Skip VMs that have snapshots.
			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				"")
			// TODO(akutz) Set error condition.
			return nil
		}
		if moVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOn {
			// TODO(akutz) Set pending condition.
			return nil
		}
	case vmopv1.VirtualMachinePromoteDisksModeOffline:
		if moVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOff {
			// Skip VMs that are not powered off.
			// TODO(akutz) Set pending condition.
			return nil
		}
	}

	req := vimtypes.PromoteDisks_Task{
		This:   moVM.Reference(),
		Unlink: true,
		Disks:  childDisks,
	}
	res, err := methods.PromoteDisks_Task(ctx, vimClient, &req)
	if err != nil {
		return fmt.Errorf("failed to call promote disks task: %w", err)
	}

	promoteTask := object.NewTask(vimClient, res.Returnval)

	// Track the task ID.
	vm.Status.TaskID = promoteTask.Reference().Value

	return pkgerr.NoRequeueError{
		Message: "doing online disk promotion",
	}
}
