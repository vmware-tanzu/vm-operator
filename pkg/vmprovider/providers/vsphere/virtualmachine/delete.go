// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func DeleteVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	state, err := vcVM.PowerState(vmCtx)
	if err != nil {
		return err
	}

	// Only a powered off VM can be destroyed.
	if state != types.VirtualMachinePowerStatePoweredOff {
		vmCtx.Logger.Info("Powering off VM prior to destroy", "currentState", state)
		if err := ChangePowerState(vmCtx, vcVM, types.VirtualMachinePowerStatePoweredOff); err != nil {
			return err
		}
	}

	t, err := vcVM.Destroy(vmCtx)
	if err != nil {
		return err
	}

	if taskInfo, err := t.WaitForResult(vmCtx); err != nil {
		if taskInfo != nil {
			vmCtx.Logger.V(5).Error(err, "destroy VM task failed", "taskInfo", taskInfo)
		}
		return errors.Wrapf(err, "destroy VM task failed")
	}

	return nil
}
