// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// RemoveStaleGroupOwnerRef removes an object's owner reference to the previous
// group if the object's group name is deleted or changed to a different group.
// Returns true if any owner references were modified, false otherwise.
func RemoveStaleGroupOwnerRef(newObj, oldObj vmopv1.VirtualMachineOrGroup) bool {

	var (
		oldGroupName = oldObj.GetGroupName()
		newGroupName = newObj.GetGroupName()
	)

	if oldGroupName == "" || oldGroupName == newGroupName {
		return false
	}

	// Object's group name is deleted or changed to a different group name.
	// Remove the owner reference to the old group if it exists in new object.

	var (
		filteredRefs      []metav1.OwnerReference
		oldGroupRefExists bool
	)

	for _, ref := range newObj.GetOwnerReferences() {
		if ref.Kind == "VirtualMachineGroup" && ref.Name == oldGroupName {
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
