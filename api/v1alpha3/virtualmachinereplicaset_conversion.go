// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// ConvertTo converts this VirtualMachineReplicaSet to the Hub version.
func (src *VirtualMachineReplicaSet) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineReplicaSet)
	if err := Convert_v1alpha3_VirtualMachineReplicaSet_To_v1alpha5_VirtualMachineReplicaSet(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachineReplicaSet{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineReplicaSet.
func (dst *VirtualMachineReplicaSet) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineReplicaSet)
	if err := Convert_v1alpha5_VirtualMachineReplicaSet_To_v1alpha3_VirtualMachineReplicaSet(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineReplicaSetList to the Hub version.
func (src *VirtualMachineReplicaSetList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineReplicaSetList)
	return Convert_v1alpha3_VirtualMachineReplicaSetList_To_v1alpha5_VirtualMachineReplicaSetList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineReplicaSetList.
func (dst *VirtualMachineReplicaSetList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineReplicaSetList)
	return Convert_v1alpha5_VirtualMachineReplicaSetList_To_v1alpha3_VirtualMachineReplicaSetList(src, dst, nil)
}
