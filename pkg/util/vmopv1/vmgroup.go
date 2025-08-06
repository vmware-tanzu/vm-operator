// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
)

// RetrieveVMGroupMembers retrieves all the group linked VMs under a VM group recursively.
// An error is return if a loop is detected among nested groups or a group has duplicated members.
func RetrieveVMGroupMembers(ctx context.Context, c ctrlclient.Client,
	vmGroupKey ctrlclient.ObjectKey, visitedGroups *sets.Set[string]) (sets.Set[string], error) {
	if visitedGroups.Has(vmGroupKey.Name) {
		return nil, fmt.Errorf("a loop is detected among groups: %q visisted", vmGroupKey.Name)
	}
	visitedGroups.Insert(vmGroupKey.Name)

	vmSet := make(sets.Set[string])
	vmGroup := &vmopv1.VirtualMachineGroup{}
	if err := c.Get(ctx, vmGroupKey, vmGroup); err != nil {
		return nil, err
	}

	for _, member := range vmGroup.Status.Members {
		if !conditions.IsTrue(&member, vmopv1.VirtualMachineGroupMemberConditionGroupLinked) {
			continue
		}

		switch member.Kind {
		case "VirtualMachine":
			if vmSet.Has(member.Name) {
				return nil, fmt.Errorf("duplicate member %q found in the group", member.Name)
			}
			vmSet.Insert(member.Name)
		case "VirtualMachineGroup":
			childVMSet, err := RetrieveVMGroupMembers(ctx, c, ctrlclient.ObjectKey{Namespace: vmGroupKey.Namespace, Name: member.Name}, visitedGroups)
			if err != nil {
				return nil, err
			}

			dupVMs := childVMSet.Intersection(vmSet)
			if dupVMs.Len() != 0 {
				return nil, fmt.Errorf("duplicate member(s) found in the group: %q", dupVMs.UnsortedList())
			}
			vmSet = vmSet.Union(childVMSet)
		default:
			return nil, fmt.Errorf("VM group %q status has a member with unknown kind: %q", vmGroup.Name, member.Kind)
		}
	}
	return vmSet, nil
}
