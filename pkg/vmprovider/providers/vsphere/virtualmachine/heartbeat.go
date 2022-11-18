// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

func GetGuestHeartBeatStatus(
	vmCtx context.VirtualMachineContext,
	vm *object.VirtualMachine) (vmopv1alpha1.GuestHeartbeatStatus, error) {

	var o mo.VirtualMachine
	if err := vm.Properties(vmCtx, vm.Reference(), []string{"guestHeartbeatStatus"}, &o); err != nil {
		return "", err
	}

	return vmopv1alpha1.GuestHeartbeatStatus(o.GuestHeartbeatStatus), nil
}
