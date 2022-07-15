package virtualmachine

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/task"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func ChangePowerState(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	ps types.VirtualMachinePowerState) error {

	var t *object.Task
	var err error

	switch ps {
	case types.VirtualMachinePowerStatePoweredOn:
		t, err = vcVM.PowerOn(vmCtx)
	case types.VirtualMachinePowerStatePoweredOff:
		t, err = vcVM.PowerOff(vmCtx)
	default:
		return fmt.Errorf("invalid power state %s", ps)
	}

	if err != nil {
		return errors.Wrapf(err, "failed task creation to change power state to %s", ps)
	}

	if taskInfo, err := t.WaitForResult(vmCtx); err != nil {
		if te, ok := err.(task.Error); ok {
			// Ignore error if VM was already in desired state.
			if ips, ok := te.Fault().(*types.InvalidPowerState); ok && ips.ExistingState == ips.RequestedState {
				return nil
			}
		}

		if taskInfo != nil {
			vmCtx.Logger.V(5).Error(err, "Change power state task failed", "taskInfo", taskInfo)
		}

		return errors.Wrapf(err, "change power state to %s task failed", ps)
	}

	return nil
}
