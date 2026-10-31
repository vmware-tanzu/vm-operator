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

// FirstIncompletePowerOffBootOrder returns the index in group.Spec.BootOrder
// of the first boot order group, in power-off order (last to first), that has
// not completed power-off. It returns -1 if every boot order group has
// completed power-off.
//
// Completion is evaluated from the observed state of the members, see
// IsGroupMemberPoweredOff.
func FirstIncompletePowerOffBootOrder(
	ctx context.Context,
	c ctrlclient.Reader,
	group *vmopv1.VirtualMachineGroup) (int, error) {

	for i := len(group.Spec.BootOrder) - 1; i >= 0; i-- {
		for _, member := range group.Spec.BootOrder[i].Members {
			ok, _, err := IsGroupMemberPoweredOff(
				ctx,
				c,
				group.Namespace,
				group.Name,
				member,
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

// IsGroupMemberPoweredOff reports whether a member of the named group has
// completed power-off. When the member has not completed power-off, the
// returned string describes why.
//
// A VirtualMachine member has completed power-off when its observed power
// state is PoweredOff, or Suspended when powerOffMode is Soft, as a Soft
// power-off does not act on a suspended VM. A VirtualMachineGroup member has
// completed power-off when all of its members, at any depth, have completed
// power-off. A member that does not exist, or that does not belong to the
// group, has not completed power-off.
//
// An error is returned if a member cannot be retrieved for a reason other
// than not found, or if a loop is detected among nested groups.
func IsGroupMemberPoweredOff(
	ctx context.Context,
	c ctrlclient.Reader,
	namespace, groupName string,
	member vmopv1.GroupMember,
	powerOffMode vmopv1.VirtualMachinePowerOpMode) (bool, string, error) {

	return isGroupMemberPoweredOff(
		ctx,
		c,
		namespace,
		groupName,
		member,
		powerOffMode,
		sets.New(groupName))
}

func isGroupMemberPoweredOff(
	ctx context.Context,
	c ctrlclient.Reader,
	namespace, groupName string,
	member vmopv1.GroupMember,
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
		switch o.Status.PowerState {
		case vmopv1.VirtualMachinePowerStateOff:
			return true, "", nil
		case vmopv1.VirtualMachinePowerStateSuspended:
			if powerOffMode == vmopv1.VirtualMachinePowerOpModeSoft {
				return true, "", nil
			}
		case vmopv1.VirtualMachinePowerStateOn:
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
				ok, reason, err := isGroupMemberPoweredOff(
					ctx,
					c,
					namespace,
					o.Name,
					childMember,
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
