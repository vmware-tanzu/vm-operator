// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// VirtualMachineContext is the context used for VirtualMachineControllers.
type VirtualMachineContext struct {
	context.Context
	Logger logr.Logger
	VM     *vmopv1.VirtualMachine
	MoVM   mo.VirtualMachine
}

func (v VirtualMachineContext) String() string {
	if v.VM == nil {
		return ""
	}
	return fmt.Sprintf("%s %s/%s", v.VM.GroupVersionKind(), v.VM.Namespace, v.VM.Name)
}

func (v VirtualMachineContext) IsOffToOn() bool {
	if v.VM == nil {
		return false
	}
	return v.MoVM.Runtime.PowerState != vimtypes.VirtualMachinePowerStatePoweredOn &&
		v.VM.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn
}

func (v VirtualMachineContext) IsOnToOff() bool {
	if v.VM == nil {
		return false
	}
	return v.MoVM.Runtime.PowerState == vimtypes.VirtualMachinePowerStatePoweredOn &&
		v.VM.Spec.PowerState == vmopv1.VirtualMachinePowerStateOff
}
