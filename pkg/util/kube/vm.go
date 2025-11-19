// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package kube

import (
	"context"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

const (
	// PriorityVirtualMachineCreating is the reconcile priority for a VM that
	// does not have a Created condition set to True.
	PriorityVirtualMachineCreating int = 100 - iota
	// PriorityVirtualMachinePowerStateChange is the reconcile priority for a VM
	// that has a desired power state not equal to its observed power state.
	PriorityVirtualMachinePowerStateChange
	// PriorityVirtualMachineWaitingForIP is the reconcile priority for a VM
	// that is powered on, should have an IP, but does not.
	PriorityVirtualMachineWaitingForIP
	// PriorityVirtualMachineDeleting is the reconcile priority for a VM that
	// is being deleted.
	PriorityVirtualMachineDeleting
	// PriorityVirtualMachineWaitingForDiskPromo is the reconcile priority for a
	// VM that is waiting on disk promotion.
	PriorityVirtualMachineWaitingForDiskPromo
)

// GetVirtualMachineReconcilePriority returns the reconcile priority for a
// VirtualMachine.
func GetVirtualMachineReconcilePriority(
	ctx context.Context,
	_ EventType,
	newObj, _ client.Object,
	defaultPriority int) int {

	if v := newObj.GetAnnotations()[pkgconst.ReconcilePriorityAnnotationKey]; v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}

	vm, ok := newObj.(*vmopv1.VirtualMachine)
	switch {
	case !ok:
		return defaultPriority

	case !vm.DeletionTimestamp.IsZero():
		return PriorityVirtualMachineDeleting

	case !pkgcond.IsTrue(vm, vmopv1.VirtualMachineConditionCreated):
		return PriorityVirtualMachineCreating

	case vm.Status.PowerState != vm.Spec.PowerState:
		return PriorityVirtualMachinePowerStateChange

	case vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn:
		var (
			netSpec = vm.Spec.Network
			netStat = vm.Status.Network
		)
		if netSpec != nil && !netSpec.Disabled {
			if netStat == nil ||
				(netStat.PrimaryIP4 == "" &&
					netStat.PrimaryIP6 == "") {

				return PriorityVirtualMachineWaitingForIP
			}
		}

	case vm.Spec.PromoteDisksMode != vmopv1.VirtualMachinePromoteDisksModeDisabled:
		if !pkgcond.IsTrue(vm, vmopv1.VirtualMachineDiskPromotionSynced) {
			return PriorityVirtualMachineWaitingForDiskPromo
		}
	}

	return defaultPriority
}
