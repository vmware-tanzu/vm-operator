// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// FirstIncompleteBootOrder returns the index in group.Spec.BootOrder of the
// first boot order group that has not reached the group's power state. Boot
// order groups are walked in power-on order (first to last) when the group's
// power state is PoweredOn, and in power-off order (last to first) when it is
// PoweredOff. It returns -1 if every boot order group has reached the group's
// power state.
//
// Completion is evaluated from the observed state of the members, see
// IsGroupMemberInPowerState.
func FirstIncompleteBootOrder(
	ctx context.Context,
	c ctrlclient.Reader,
	group *vmopv1.VirtualMachineGroup) (int, error) {

	n := len(group.Spec.BootOrder)
	for j := range n {
		i := j
		if group.Spec.PowerState == vmopv1.VirtualMachinePowerStateOff {
			i = n - 1 - j
		}
		for _, member := range group.Spec.BootOrder[i].Members {
			ok, _, err := IsGroupMemberInPowerState(
				ctx,
				c,
				group.Namespace,
				group.Name,
				member,
				group.Spec.PowerState,
				group.Spec.PowerOffMode)
			if err != nil {
				return -1, err
			}
			if !ok {
				return i, nil
			}
		}
	}

	return -1, nil
}

// IsGroupMemberInPowerState reports whether a member of the named group has
// reached the given power state, which is PoweredOn or PoweredOff. When the
// member has not reached the power state, the returned string describes why.
//
// A VirtualMachine member has reached PoweredOn when its observed power state
// is PoweredOn. It has reached PoweredOff when its observed power state is
// PoweredOff, or Suspended when powerOffMode is Soft, as a Soft power-off
// does not act on a suspended VM. A VirtualMachineGroup member has reached
// the power state when all of its members, at any depth, have reached it. A
// member that does not exist, or that does not belong to the group, has not
// reached the power state.
//
// An error is returned if a member cannot be retrieved for a reason other
// than not found, or if a loop is detected among nested groups.
func IsGroupMemberInPowerState(
	ctx context.Context,
	c ctrlclient.Reader,
	namespace, groupName string,
	member vmopv1.GroupMember,
	powerState vmopv1.VirtualMachinePowerState,
	powerOffMode vmopv1.VirtualMachinePowerOpMode) (bool, string, error) {

	return isGroupMemberInPowerState(
		ctx,
		c,
		namespace,
		groupName,
		member,
		powerState,
		powerOffMode,
		sets.New(groupName))
}

func isGroupMemberInPowerState(
	ctx context.Context,
	c ctrlclient.Reader,
	namespace, groupName string,
	member vmopv1.GroupMember,
	powerState vmopv1.VirtualMachinePowerState,
	powerOffMode vmopv1.VirtualMachinePowerOpMode,
	visitedGroups sets.Set[string]) (bool, string, error) {

	kind := member.Kind
	if kind == "" {
		kind = vmKind
	}
	memberKindAndName := kind + "/" + member.Name

	var obj VirtualMachineOrGroup
	switch kind {
	case vmKind:
		obj = &vmopv1.VirtualMachine{}
	case vmgKind:
		if visitedGroups.Has(member.Name) {
			return false, "", fmt.Errorf(
				"a loop is detected among groups: %q visited", member.Name)
		}
		obj = &vmopv1.VirtualMachineGroup{}
	default:
		return false, "", fmt.Errorf(
			"member %q has unknown kind", memberKindAndName)
	}

	if err := c.Get(ctx, ctrlclient.ObjectKey{
		Namespace: namespace,
		Name:      member.Name,
	}, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return false, fmt.Sprintf(
				"member %q not found", memberKindAndName), nil
		}
		return false, "", fmt.Errorf(
			"failed to get member %q: %w", memberKindAndName, err)
	}

	if obj.GetGroupName() != groupName {
		return false, fmt.Sprintf(
			"member %q does not belong to group %q",
			memberKindAndName, groupName), nil
	}

	switch o := obj.(type) {
	case *vmopv1.VirtualMachine:
		if isVMInPowerState(o.Status.PowerState, powerState, powerOffMode) {
			return true, "", nil
		}
		return false, fmt.Sprintf(
			"member %q has power state %q",
			memberKindAndName, o.Status.PowerState), nil

	case *vmopv1.VirtualMachineGroup:
		// Only the groups on the current path are tracked, so a loop is
		// detected without rejecting a group that is reached twice.
		visitedGroups.Insert(o.Name)
		defer visitedGroups.Delete(o.Name)
		for _, bootOrder := range o.Spec.BootOrder {
			for _, childMember := range bootOrder.Members {
				ok, reason, err := isGroupMemberInPowerState(
					ctx,
					c,
					namespace,
					o.Name,
					childMember,
					powerState,
					powerOffMode,
					visitedGroups)
				if err != nil || !ok {
					return ok, reason, err
				}
			}
		}
		return true, "", nil
	}

	return false, "", nil
}

// isVMInPowerState reports whether a VM with the given observed power state
// has reached the given desired power state.
func isVMInPowerState(
	observed, desired vmopv1.VirtualMachinePowerState,
	powerOffMode vmopv1.VirtualMachinePowerOpMode) bool {

	if observed == desired {
		return true
	}
	return desired == vmopv1.VirtualMachinePowerStateOff &&
		observed == vmopv1.VirtualMachinePowerStateSuspended &&
		powerOffMode == vmopv1.VirtualMachinePowerOpModeSoft
}
