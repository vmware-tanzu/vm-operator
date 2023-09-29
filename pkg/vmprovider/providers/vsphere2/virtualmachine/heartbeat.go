// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

func GetGuestHeartBeatStatus(
	ctx context.Context,
	vm *object.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {

	var o mo.VirtualMachine
	if err := vm.Properties(ctx, vm.Reference(), []string{"guestHeartbeatStatus"}, &o); err != nil {
		return "", err
	}

	return vmopv1.GuestHeartbeatStatus(o.GuestHeartbeatStatus), nil
}
