// Copyright (c) 2022-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

func DeleteVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	if _, err := vmutil.SetAndWaitOnPowerState(
		vmCtx,
		vcVM.Client(),
		mo.VirtualMachine{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: vcVM.Reference(),
				},
			},
		},
		false,
		types.VirtualMachinePowerStatePoweredOff,
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
		return errors.Wrapf(err, "destroy VM task failed")
	}

	return nil
}
