// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"
	"slices"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

type vmContextKey uint8

const (
	// vmRecentTasksContextKey is the context key for
	// setting/getting a []vimtypes.RecentTask slice from a context.
	vmRecentTasksContextKey vmContextKey = iota
	vmTagsContextKey
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

var ignoredTaskDescriptionIDs = []string{
	"com.vmware.wcp.mobility.virtualmachinesimport.create",
}

// Snapshot-related task description IDs that should block VM reconciliation.
var snapshotTaskDescriptionIDs = []string{
	"VirtualMachine.createSnapshot",
	"VirtualMachine.removeSnapshot",
	"VirtualMachine.revertToSnapshot",
}

// HasVMRunningTask returns true if the VM has a running task that should block
// reconciling the VM's desired state.
func HasVMRunningTask(ctx context.Context, checkSharedLock bool) bool {
	for _, t := range GetVMRecentTasks(ctx) {
		if t.State == vimtypes.TaskInfoStateRunning &&
			!slices.Contains(ignoredTaskDescriptionIDs, t.DescriptionId) {

			if t.Entity != nil && checkSharedLock {
				return slices.Contains(t.Locked, *t.Entity)
			}

			return true
		}
	}
	return false
}

// HasVMRunningSnapshotTask returns true if the VM has a running snapshot-related task.
func HasVMRunningSnapshotTask(ctx context.Context) bool {
	for _, t := range GetVMRecentTasks(ctx) {
		if t.State == vimtypes.TaskInfoStateRunning &&
			slices.Contains(snapshotTaskDescriptionIDs, t.DescriptionId) {
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

	val, ok := obj.([]vimtypes.TaskInfo)
	if !ok {
		return nil
	}

	return val
}

func WithVMRecentTasks(
	parent context.Context,
	taskInfo []vimtypes.TaskInfo) context.Context {

	return context.WithValue(parent, vmRecentTasksContextKey, taskInfo)
}

func WithVMTags(
	parent context.Context,
	tags []string) context.Context {

	return context.WithValue(parent, vmTagsContextKey, tags)
}

func GetVMTags(ctx context.Context) []string {
	obj := ctx.Value(vmTagsContextKey)
	if obj == nil {
		return nil
	}

	val, ok := obj.([]string)
	if !ok {
		return nil
	}

	return val
}
