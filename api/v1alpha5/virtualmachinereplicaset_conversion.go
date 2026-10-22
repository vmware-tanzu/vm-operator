// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	"fmt"

	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ConvertTo converts this VirtualMachineReplicaSet to the Hub version.
func (src *VirtualMachineReplicaSet) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineReplicaSet)
	if err := Convert_v1alpha5_VirtualMachineReplicaSet_To_v1alpha6_VirtualMachineReplicaSet(src, dst, nil); err != nil {
		return fmt.Errorf("failed to convert ReplicaSet to v1alpha6: %w", err)
	}

	restored := &vmopv1.VirtualMachineReplicaSet{}
	ok, err := utilconversion.UnmarshalData(src, restored)
	if err != nil {
		return fmt.Errorf("failed to restore ReplicaSet conversion data: %w", err)
	}
	if !ok {
		return nil
	}

	restore_v1alpha6_VirtualMachineVolumes(
		dst.Spec.Template.Spec.Volumes, restored.Spec.Template.Spec.Volumes)
	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineReplicaSet.
func (dst *VirtualMachineReplicaSet) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineReplicaSet)
	if err := Convert_v1alpha6_VirtualMachineReplicaSet_To_v1alpha5_VirtualMachineReplicaSet(src, dst, nil); err != nil {
		return fmt.Errorf("failed to convert ReplicaSet to v1alpha5: %w", err)
	}

	// Preserve Hub data on down-conversion except for metadata.
	if err := utilconversion.MarshalData(src, dst); err != nil {
		return fmt.Errorf("failed to preserve ReplicaSet conversion data: %w", err)
	}
	return nil
}

// ConvertTo converts this VirtualMachineReplicaSetList to the Hub version.
func (src *VirtualMachineReplicaSetList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineReplicaSetList)
	if err := Convert_v1alpha5_VirtualMachineReplicaSetList_To_v1alpha6_VirtualMachineReplicaSetList(src, dst, nil); err != nil {
		return fmt.Errorf("failed to convert ReplicaSet list to v1alpha6: %w", err)
	}
	for i := range src.Items {
		if err := src.Items[i].ConvertTo(&dst.Items[i]); err != nil {
			return fmt.Errorf("failed to convert ReplicaSet list item %d to v1alpha6: %w", i, err)
		}
	}
	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineReplicaSetList.
func (dst *VirtualMachineReplicaSetList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineReplicaSetList)
	if err := Convert_v1alpha6_VirtualMachineReplicaSetList_To_v1alpha5_VirtualMachineReplicaSetList(src, dst, nil); err != nil {
		return fmt.Errorf("failed to convert ReplicaSet list to v1alpha5: %w", err)
	}
	for i := range src.Items {
		if err := dst.Items[i].ConvertFrom(&src.Items[i]); err != nil {
			return fmt.Errorf("failed to convert ReplicaSet list item %d to v1alpha5: %w", i, err)
		}
	}
	return nil
}
