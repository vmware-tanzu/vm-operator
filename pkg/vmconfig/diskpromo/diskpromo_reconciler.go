// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package diskpromo

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

var ErrPromoteDisks = pkgerr.NoRequeueNoErr("promoting disks")

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

const (
	ReasonTaskError = "DiskPromotionTaskError"
	ReasonPending   = "DiskPromotionPending"
	ReasonRunning   = "DiskPromotionRunning"

	PromoteDisksTaskKey = "VirtualMachine.promoteDisks"
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

// Reconcile configures the VM's disk promotion settings.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	_ *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, nil)
}

// Reconcile performs an online promotion of any of a VM's linked-clone disks to
// full clones. This reconciler is a no-op for VMs that do not yet exists and/or
// are not powered on.
//
//nolint:gocyclo
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

	var (
		runningTaskInfo []vimtypes.TaskInfo
		logger          = pkgutil.FromContextOrDefault(ctx)
	)

	// Check if the VM has any outstanding tasks.
	for _, t := range pkgctx.GetVMRecentTasks(ctx) {

		// If there is a task that is already running then do not promote the
		// disks right now.
		if t.State == vimtypes.TaskInfoStateRunning {
			if t.DescriptionId == PromoteDisksTaskKey {
				pkgcond.MarkFalse(
					vm,
					vmopv1.VirtualMachineDiskPromotionSynced,
					ReasonRunning,
					"%s",
					"Promotion is running")
				return nil
			}

			runningTaskInfo = append(runningTaskInfo, t)
		} else if t.DescriptionId == PromoteDisksTaskKey {

			// If the task is an online promote disk task, then check if it is
			// completed or in error.
			switch t.State {

			case vimtypes.TaskInfoStateError:
				pkgcond.MarkFalse(
					vm,
					vmopv1.VirtualMachineDiskPromotionSynced,
					ReasonTaskError,
					"%s",
					t.Error.LocalizedMessage)
				return nil

			case vimtypes.TaskInfoStateSuccess:
				pkgcond.MarkTrue(
					vm,
					vmopv1.VirtualMachineDiskPromotionSynced)
				return nil
			}
		}
	}

	const defaultMode = vmopv1.VirtualMachinePromoteDisksModeOnline

	promoMode := vm.Spec.PromoteDisksMode
	if promoMode == "" {
		// Default to online promote.
		promoMode = defaultMode
		logger.Info("Empty promotion mode, using default mode",
			"defaultMode", defaultMode)
	}

	// Allow the mode to be overridden via an env var.
	if v := pkgcfg.FromContext(ctx).PromoteDisksMode; v != "" {
		promoMode = vmopv1.VirtualMachinePromoteDisksMode(v)
		logger.Info("Got promo mode from env", "mode", promoMode)
	}

	// Validate the promotion mode.
	switch promoMode {
	case vmopv1.VirtualMachinePromoteDisksModeOnline,
		vmopv1.VirtualMachinePromoteDisksModeOffline,
		vmopv1.VirtualMachinePromoteDisksModeDisabled:
		// No-op
	default:
		logger.Info("Invalid promotion mode, using default mode",
			"invalidMode", promoMode,
			"defaultMode", defaultMode)
		promoMode = defaultMode
	}

	logger = logger.WithValues("mode", promoMode)

	logger.V(4).Info("Finding candidates for disk promotion")

	if promoMode == vmopv1.VirtualMachinePromoteDisksModeDisabled {
		// Skip VMs that do not request promotion.
		pkgcond.Delete(vm, vmopv1.VirtualMachineDiskPromotionSynced)
		return nil
	}

	// Find the VirtualDisks for this VM.
	var (
		childDisks       []vimtypes.VirtualDisk
		devices          = object.VirtualDeviceList(moVM.Config.Hardware.Device)
		allDisks         = devices.SelectByType(&vimtypes.VirtualDisk{})
		snapshotDiskKeys = map[int32]struct{}{}
	)

	// Find all of the disks that are participating in snapshots.
	if moVM.LayoutEx != nil {
		for _, snap := range moVM.LayoutEx.Snapshot {
			for _, disk := range snap.Disk {
				snapshotDiskKeys[disk.Key] = struct{}{}
			}
		}
	}

	// Find any classic, file-based disks that have parent backings that are
	// not participating in snapshots.
	for i := range allDisks {
		d := allDisks[i].(*vimtypes.VirtualDisk)

		if d.VDiskId == nil { // Skip FCDs.

			if _, ok := snapshotDiskKeys[d.Key]; !ok { // Skip snapshots.

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
	}

	logger = logger.WithValues(
		"totalDisks", len(allDisks),
		"childDisks", len(childDisks))

	if len(childDisks) == 0 {
		logger.V(4).Info(
			"Skipping disk promotion for VM with no disks to promote")
		return nil
	}

	logger.V(4).Info("Checking if disks can be promoted")

	if len(runningTaskInfo) > 0 {
		for _, t := range runningTaskInfo {
			logger.V(4).Info("Skipping disk promotion for VM with running task",
				"taskRef", t.Task.Value,
				"taskDescriptionID", t.DescriptionId)
		}
		pkgcond.MarkFalse(
			vm,
			vmopv1.VirtualMachineDiskPromotionSynced,
			ReasonPending,
			"Cannot promote disks when VM has running task")
		return nil
	}

	switch promoMode {
	case vmopv1.VirtualMachinePromoteDisksModeOnline:
		if moVM.Snapshot != nil && moVM.Snapshot.CurrentSnapshot != nil {
			// Skip VMs that have snapshots.
			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				ReasonPending,
				"Cannot online promote disks when VM has snapshot")
			logger.V(4).Info(
				"Skipping online disk promotion for VM with snapshot(s)")
			return nil
		}

		if moVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOn {
			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				ReasonPending,
				"Pending VM powered on")
			logger.V(4).Info(
				"Skipping online disk promotion until VM powered on")
			return nil
		}

		if moVM.Guest != nil &&
			moVM.Guest.CustomizationInfo != nil {

			custStatus := vimtypes.GuestInfoCustomizationStatus(
				moVM.Guest.CustomizationInfo.CustomizationStatus)

			switch custStatus {
			case vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING,
				vimtypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING:

				pkgcond.MarkFalse(
					vm,
					vmopv1.VirtualMachineDiskPromotionSynced,
					ReasonPending,
					"Pending guest customization")
				logger.V(4).Info(
					"Skipping online disk promotion for guest customization")
				return nil
			}
		}

	case vmopv1.VirtualMachinePromoteDisksModeOffline:
		if moVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOff {
			// Skip VMs that are not powered off.
			pkgcond.MarkFalse(
				vm,
				vmopv1.VirtualMachineDiskPromotionSynced,
				ReasonPending,
				"Pending VM powered off")
			logger.V(4).Info(
				"Skipping offline disk promotion until VM is powered off")
			return nil
		}
	}

	logger.Info("Promoting disks")

	obj := object.NewVirtualMachine(vimClient, moVM.Self)
	task, err := obj.PromoteDisks(ctx, true, childDisks)
	if err != nil {
		return fmt.Errorf("failed to call promote disks task: %w", err)
	}

	logger.Info("Disk promotion task created",
		"taskRef", task.Reference().Value)

	return ErrPromoteDisks
}
