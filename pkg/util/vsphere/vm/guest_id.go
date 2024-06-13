// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// GuestIDProperty is the property name for the guest ID in the config spec.
const GuestIDProperty = "configSpec.guestId"

// UpdateVMGuestIDCondition updates the VM's GuestID condition to false if the
// taskInfo contains an InvalidArgument fault with an invalid guest ID property.
// Otherwise, it updates the condition to true.
func UpdateVMGuestIDCondition(
	vmctx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec,
	taskInfo *vimtypes.TaskInfo) {
	// Return early if the current configSpec does not have a guest ID.
	if configSpec.GuestId == "" {
		return
	}

	var invalidGuestID bool

	defer func() {
		if invalidGuestID {
			conditions.MarkFalse(vmctx.VM, vmopv1.GuestIDCondition, "Invalid",
				fmt.Sprintf("The specified guest ID value is not supported: %s",
					configSpec.GuestId))
		} else {
			conditions.MarkTrue(vmctx.VM, vmopv1.GuestIDCondition)
		}
	}()

	if taskInfo == nil || taskInfo.Error == nil {
		return
	}

	fault, ok := taskInfo.Error.Fault.(*vimtypes.InvalidArgument)
	if !ok {
		return
	}

	invalidGuestID = strings.Contains(fault.InvalidProperty, GuestIDProperty)
}
