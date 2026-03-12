// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"
	"path"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/fault"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

// VMDeletePropertiesSelector is the set of VM properties fetched at the start
// of provider DeleteVirtualMachine.
var VMDeletePropertiesSelector = []string{
	"recentTask",
	"config.extraConfig",
	"config.files",
	"summary.runtime.connectionState",
}

// CleanupVMDir removes the VM directory on the datastore and, when non-TLD,
// the namespace mapping.
func CleanupVMDir(
	ctx context.Context,
	vimClient *vim25.Client,
	dc *object.Datacenter,
	vmDirPath, namespacePath string) error {

	if vimClient == nil || dc == nil || vmDirPath == "" {
		return fmt.Errorf(
			"invalid CleanupVMDir call: vimClient=%v, dc=%v, vmDirPath=%q",
			vimClient != nil, dc != nil, vmDirPath)
	}

	ctx = context.WithoutCancel(ctx)

	fm := object.NewFileManager(vimClient)
	task, err := fm.DeleteDatastoreFile(ctx, vmDirPath, dc)
	if err != nil {
		return err
	}
	if err := task.Wait(ctx); err != nil &&
		!fault.Is(err, &vimtypes.FileNotFound{}) {
		return err
	}

	if namespacePath != "" {
		nm := object.NewDatastoreNamespaceManager(vimClient)
		if err := nm.DeleteDirectory(ctx, dc, namespacePath); err != nil &&
			!fault.Is(err, &vimtypes.FileNotFound{}) {
			return err
		}
	}

	return nil
}

// DeleteVirtualMachine powers off the VM, destroys it from vCenter, and
// performs best-effort cleanup of VM files. Cleanup errors are non-fatal
// since the primary objective (VM deletion) already succeeded. Orphaned
// directories don't impact functionality and can be cleaned asynchronously.
// Propagating cleanup errors would block deletion, leaving the VirtualMachine
// resource in Terminating state indefinitely.
func DeleteVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	dc *object.Datacenter) error {

	if _, err := vmutil.SetAndWaitOnPowerState(
		logr.NewContext(vmCtx, vmCtx.Logger),
		vcVM.Client(),
		vmutil.ManagedObjectFromObject(vcVM),
		false,
		vimtypes.VirtualMachinePowerStatePoweredOff,
		vmutil.ParsePowerOpMode(string(vmCtx.VM.Spec.PowerOffMode))); err != nil {

		return err
	}

	t, err := vcVM.Destroy(vmCtx)
	if err != nil {
		return err
	}

	if taskInfo, err := t.WaitForResult(vmCtx); err != nil {
		if taskInfo != nil {
			vmCtx.Logger.V(5).Error(err, "destroy VM task failed",
				"taskInfo", taskInfo)
		}
		return fmt.Errorf("destroy VM task failed: %w", err)
	}

	namespacePath, _ := object.OptionValueList(
		vmCtx.MoVM.Config.ExtraConfig).GetString(
		pkgconst.ExtraConfigVMDirNamespacePath)

	_ = CleanupVMDir(vmCtx.Context,
		vcVM.Client(),
		dc,
		path.Dir(vmCtx.MoVM.Config.Files.VmPathName),
		namespacePath)
	return nil
}
