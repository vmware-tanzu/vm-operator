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

// UpdateVMGuestIDReconfiguredCondition deletes the VM's GuestIDReconfigured
// condition if the configSpec doesn't contain a guestID, or if the taskInfo
// does not contain an invalid guestID property error. Otherwise, it sets the
// condition to false with the invalid guest ID property value in the reason.
func UpdateVMGuestIDReconfiguredCondition(
	vmctx pkgctx.VirtualMachineContext,
	configSpec vimtypes.VirtualMachineConfigSpec,
	taskInfo *vimtypes.TaskInfo) {
	if configSpec.GuestId == "" {
		conditions.Delete(vmctx.VM, vmopv1.GuestIDReconfiguredCondition)
		return
	}

	var invalidGuestID bool

	defer func() {
		if invalidGuestID {
			conditions.MarkFalse(
				vmctx.VM,
				vmopv1.GuestIDReconfiguredCondition,
				"Invalid",
				fmt.Sprintf("The specified guest ID value is not supported: %s",
					configSpec.GuestId))
		} else {
			conditions.Delete(vmctx.VM, vmopv1.GuestIDReconfiguredCondition)
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
