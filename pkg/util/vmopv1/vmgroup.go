// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
)

const (
	vmKind  = "VirtualMachine"
	vmgKind = "VirtualMachineGroup"
)

// VirtualMachineOrGroup is an internal interface that represents a
// VirtualMachine or VirtualMachineGroup object.
type VirtualMachineOrGroup interface {
	metav1.Object
	runtime.Object
	DeepCopyObject() runtime.Object
	GetMemberKind() string
	GetGroupName() string
	SetGroupName(value string)
	GetPowerState() vmopv1.VirtualMachinePowerState
	SetPowerState(value vmopv1.VirtualMachinePowerState)
	GetConditions() []metav1.Condition
	SetConditions([]metav1.Condition)
}

// GroupToMembersMapperFn returns a mapper function that can be used to queue
// reconcile requests for all the currently linked group members with given kind
// (VirtualMachine/VirtualMachineGroup) in response to VirtualMachineGroup watch.
func GroupToMembersMapperFn(
	ctx context.Context,
	client ctrlclient.Client,
	memberKind string) handler.MapFunc {

	return func(ctx context.Context, o ctrlclient.Object) []reconcile.Request {

		var (
			group     = o.(*vmopv1.VirtualMachineGroup)
			namespace = group.Namespace
			requests  = make([]reconcile.Request, 0, len(group.Status.Members))
		)

		for _, groupMember := range group.Status.Members {
			// Skip if the member is not of the desired kind.
			if groupMember.Kind != memberKind {
				continue
			}

			// Skip if the group status doesn't have this member linked.
			if !conditions.IsTrue(
				&groupMember,
				vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			) {
				continue
			}

			switch memberKind {
			case vmKind:
				vmName := groupMember.Name
				vm := &vmopv1.VirtualMachine{}
				key := ctrlclient.ObjectKey{Namespace: namespace, Name: vmName}
				if err := client.Get(ctx, key, vm); err != nil {
					continue
				}

				// Check VM.Spec.GroupName still points to this group in case it
				// changed while the group status hasn't been updated yet.
				if vm.Spec.GroupName != group.Name {
					continue
				}

				// Only trigger a reconcile if the VM's condition doesn't have
				// group linked condition set to true, or if VM isn't placed.
				if !conditions.IsTrue(vm, vmopv1.VirtualMachineGroupMemberConditionGroupLinked) ||
					!conditions.IsTrue(vm, vmopv1.VirtualMachineConditionPlacementReady) {
					requests = append(requests, reconcile.Request{
						NamespacedName: ctrlclient.ObjectKey{
							Namespace: namespace,
							Name:      vmName,
						},
					})
				}
			case vmgKind:
				vmgName := groupMember.Name
				vmg := &vmopv1.VirtualMachineGroup{}
				key := ctrlclient.ObjectKey{Namespace: namespace, Name: vmgName}
				if err := client.Get(ctx, key, vmg); err != nil {
					continue
				}

				// Check VMG.Spec.GroupName still points to this group in case
				// it changed while the group status hasn't been updated yet.
				if vmg.Spec.GroupName != group.Name {
					continue
				}

				// Only trigger a reconcile if the VMG's condition doesn't have
				// group linked condition set to true.
				if !conditions.IsTrue(
					vmg, vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				) {
					requests = append(requests, reconcile.Request{
						NamespacedName: ctrlclient.ObjectKey{
							Namespace: namespace,
							Name:      vmgName,
						},
					})
				}
			}
		}

		if len(requests) > 0 {
			pkglog.FromContextOrDefault(ctx).WithValues(
				"groupName", group.Name, "groupNamespace", namespace,
			).V(4).Info(
				"Reconciling members due to their VirtualMachineGroup watch",
				"requests", requests,
				"memberKind", memberKind,
			)
		}

		return requests
	}
}

// MemberToGroupMapperFn returns a MapFunc that reconciles a VirtualMachineGroup
// when a linked member (VM or VMGroup) changes. This ensures the group's status
// is updated in time to reflect the current member latest state (e.g. ready
// condition for VMGroup kind members or power state for VM kind members).
func MemberToGroupMapperFn(ctx context.Context) handler.MapFunc {

	return func(ctx context.Context, o ctrlclient.Object) []reconcile.Request {
		memberObj, ok := o.(VirtualMachineOrGroup)
		if !ok {
			panic(fmt.Sprintf("Expected VirtualMachineOrGroup, but got %T", o))
		}

		var requests []reconcile.Request

		if memberObj.GetGroupName() != "" && conditions.IsTrue(
			memberObj,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
		) {
			requests = append(requests, reconcile.Request{
				NamespacedName: ctrlclient.ObjectKey{
					Namespace: memberObj.GetNamespace(),
					Name:      memberObj.GetGroupName(),
				},
			})
		}

		if len(requests) > 0 {
			pkglog.FromContextOrDefault(ctx).WithValues(
				"memberName", memberObj.GetName(),
				"memberNamespace", memberObj.GetNamespace(),
			).V(4).Info(
				"Reconciling VirtualMachineGroup due to its member watch",
				"requests", requests,
			)
		}

		return requests
	}
}

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
		case vmKind:
			if vmSet.Has(member.Name) {
				return nil, fmt.Errorf("duplicate member %q found in the group", member.Name)
			}
			vmSet.Insert(member.Name)
		case vmgKind:
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

// UpdateGroupLinkedCondition updates the group linked condition for a member.
// If the member has no group name, the group linked condition is deleted.
func UpdateGroupLinkedCondition(
	ctx context.Context,
	member VirtualMachineOrGroup,
	c ctrlclient.Client) error {

	if member.GetGroupName() == "" {
		conditions.Delete(
			member,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
		)
		return nil
	}

	var (
		obj vmopv1.VirtualMachineGroup
		key = ctrlclient.ObjectKey{
			Name:      member.GetGroupName(),
			Namespace: member.GetNamespace(),
		}
	)

	if err := c.Get(ctx, key, &obj); err != nil {
		if !apierrors.IsNotFound(err) {
			conditions.MarkError(
				member,
				vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				"Error",
				err,
			)
			return err
		}

		conditions.MarkFalse(
			member,
			vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
			"NotFound",
			"",
		)
		return nil
	}

	for _, bo := range obj.Spec.BootOrder {
		for _, m := range bo.Members {
			if m.Kind == member.GetMemberKind() && m.Name == member.GetName() {
				conditions.MarkTrue(
					member,
					vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
				)
				return nil
			}
		}
	}

	conditions.MarkFalse(
		member,
		vmopv1.VirtualMachineGroupMemberConditionGroupLinked,
		"NotMember",
		"",
	)

	return nil
}

// RemoveStaleGroupOwnerRef removes an object's owner reference to the previous
// group if the object's group name is deleted or changed to a different group.
// Returns true if any owner references were modified, false otherwise.
func RemoveStaleGroupOwnerRef(newObj, oldObj VirtualMachineOrGroup) bool {

	var (
		oldGroupName = oldObj.GetGroupName()
		newGroupName = newObj.GetGroupName()
	)

	if oldGroupName == "" || oldGroupName == newGroupName {
		return false
	}

	// Object's group name is deleted or changed to a different group name.
	// Remove the owner reference to the old group if it exists in new object.

	filteredRefs := make(
		[]metav1.OwnerReference,
		0,
		len(newObj.GetOwnerReferences()),
	)
	oldGroupRefExists := false

	for _, ref := range newObj.GetOwnerReferences() {
		if ref.Kind == vmgKind && ref.Name == oldGroupName {
			oldGroupRefExists = true
			continue
		}
		filteredRefs = append(filteredRefs, ref)
	}

	if oldGroupRefExists {
		newObj.SetOwnerReferences(filteredRefs)
		return true
	}

	return false
}
