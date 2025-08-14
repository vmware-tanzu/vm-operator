// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/paused"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

func DeleteVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	if err := vcVM.Properties(
		vmCtx,
		vcVM.Reference(),
		[]string{
			"config.extraConfig",
			"summary.runtime.connectionState",
		}, &vmCtx.MoVM); err != nil {

		return fmt.Errorf("failed to fetch props when deleting VM: %w", err)
	}

	// Only process connected VMs or if the connection state is empty.
	if cs := vmCtx.MoVM.Summary.Runtime.ConnectionState; cs != "" && cs !=
		vimtypes.VirtualMachineConnectionStateConnected {

		// Return a NoRequeueError so the VM is not requeued for
		// reconciliation.
		//
		// The watcher service ensures that VMs will be reconciled
		// immediately upon their summary.runtime.connectionState value
		// changing.
		//
		// TODO(akutz) Determine if we should surface some type of condition
		//             that indicates this state.

		return fmt.Errorf("failed to delete vm: %w", pkgerr.NoRequeueError{
			Message: fmt.Sprintf("unsupported connection state: %s", cs),
		})
	}

	// Throw an error to distinguish from successful deletion.
	if paused := paused.ByAdmin(vmCtx.MoVM); paused {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = make(map[string]string)
		}
		vmCtx.VM.Labels[vmopv1.PausedVMLabelKey] = "admin"

		return fmt.Errorf("failed to delete vm: %w", pkgerr.NoRequeueError{
			Message: constants.VMPausedByAdminError,
		})
	}

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
			vmCtx.Logger.V(5).Error(err, "destroy VM task failed", "taskInfo", taskInfo)
		}
		return fmt.Errorf("destroy VM task failed: %w", err)
	}

	return nil
}
