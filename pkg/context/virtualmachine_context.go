// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
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

type vmContextKey uint8

const (
	// vmRecentTasksContextKey is the context key for
	// setting/getting a []vimtypes.RecentTask slice from a context.
	vmRecentTasksContextKey vmContextKey = iota
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

func HasVMRunningTask(ctx context.Context) bool {
	for _, t := range GetVMRecentTasks(ctx) {
		if t.State == vimtypes.TaskInfoStateRunning {
			return true
		}
	}
	return false
}

func GetVMRecentTasks(ctx context.Context) []vimtypes.TaskInfo {
	obj := ctx.Value(vmRecentTasksContextKey)
	if obj == nil {
		return nil
	}

	rt, ok := obj.([]vimtypes.TaskInfo)
	if !ok {
		return nil
	}

	return rt
}

func WithVMRecentTasks(
	parent context.Context,
	taskInfo []vimtypes.TaskInfo) context.Context {

	return context.WithValue(parent, vmRecentTasksContextKey, taskInfo)
}
