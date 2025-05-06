// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package diskpromo

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
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

const (
	ReasonTaskError = "DiskPromotionTaskError"
	ReasonPending   = "DiskPromotionPending"

	promoteDisksTaskKey = "VirtualMachine.promoteDisks"
)

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

	if ctx == nil {
		panic("context is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}

	if moVM.Config == nil {
		// Skip VMs that are not yet created.
		return nil
	}

	logger := logr.FromContextOrDiscard(ctx)

	if vm.Status.TaskID != "" {
		ref := vimtypes.ManagedObjectReference{
			Type:  "Task",
			Value: vm.Status.TaskID,
		}

		var task mo.Task
		pc := property.DefaultCollector(vimClient)

		if err := pc.RetrieveOne(ctx, ref, []string{"info"}, &task); err != nil {
			if fault.Is(err, &vimtypes.ManagedObjectNotFound{}) {
				// Tasks go away after 10m of completion
				vm.Status.TaskID = ""
			}
			return err
		}

		logger.Info("Pending task",
			"ID", vm.Status.TaskID,
			"description", task.Info.DescriptionId,
			"state", task.Info.State)

		// From the API doc:
		//   An identifier for this operation. This includes publicly visible internal tasks and
		//   is a lookup in the TaskDescription methodInfo data object.
		// See also:
		//   govc collect -s -json TaskManager:TaskManager description.methodInfo | \
		//     jq '.[] | select(.key == "VirtualMachine.promoteDisks") | .'

		if task.Info.DescriptionId != promoteDisksTaskKey {
			return nil
		}

		switch task.Info.State {
		case vimtypes.TaskInfoStateSuccess:
			vm.Status.TaskID = ""

			pkgcond.MarkTrue(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced)

			return nil
		case vimtypes.TaskInfoStateError:
			vm.Status.TaskID = ""

			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				ReasonTaskError,
				"%s",
				task.Info.Error.LocalizedMessage)

			return nil
		default:
			// Skip VMs with outstanding tasks.
			return nil
		}
	}

	logger.Info("Promote disks", "mode", vm.Spec.PromoteDisksMode)

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

	logger.Info("Promote disks", "total", len(allDisks), "child", len(childDisks))

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
				ReasonPending,
				"Cannot online promote disks when VM has snapshot")
			return nil
		}
		if moVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOn {
			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				ReasonPending,
				"Pending VM powered on")
			return nil
		}
	case vmopv1.VirtualMachinePromoteDisksModeOffline:
		if moVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOff {
			// Skip VMs that are not powered off.
			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				ReasonPending,
				"Pending VM powered off")
			return nil
		}
	}

	obj := object.NewVirtualMachine(vimClient, moVM.Self)
	task, err := obj.PromoteDisks(ctx, true, childDisks)
	if err != nil {
		return fmt.Errorf("failed to call promote disks task: %w", err)
	}

	// Track the task ID.
	vm.Status.TaskID = task.Reference().Value

	return pkgerr.NoRequeueError{
		Message: "doing online disk promotion",
	}
}
