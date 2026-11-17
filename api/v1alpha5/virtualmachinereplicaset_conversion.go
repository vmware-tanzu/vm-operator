// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

func restore_v1alpha6_VirtualMachineReplicaSetTemplateFlex(dst, src *vmopv1.VirtualMachineReplicaSet) {
	dstSpec := &dst.Spec.Template.Spec
	srcSpec := &src.Spec.Template.Spec

	dstSpec.ReservedProfileName = srcSpec.ReservedProfileName

	if srcSpec.Hardware == nil {
		return
	}
	if len(srcSpec.Hardware.VGPUs) == 0 &&
		len(srcSpec.Hardware.DynamicDirectPathIODevices) == 0 {

		return
	}
	hw := srcSpec.Hardware.DeepCopy()
	if dstSpec.Hardware == nil {
		dstSpec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
	}
	dstSpec.Hardware.VGPUs = hw.VGPUs
	dstSpec.Hardware.DynamicDirectPathIODevices = hw.DynamicDirectPathIODevices
}

// ConvertTo converts this VirtualMachineReplicaSet to the Hub version.
func (src *VirtualMachineReplicaSet) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineReplicaSet)
	if err := Convert_v1alpha5_VirtualMachineReplicaSet_To_v1alpha6_VirtualMachineReplicaSet(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachineReplicaSet{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha6_VirtualMachineReplicaSetTemplateFlex(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineReplicaSet.
func (dst *VirtualMachineReplicaSet) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineReplicaSet)
	if err := Convert_v1alpha6_VirtualMachineReplicaSet_To_v1alpha5_VirtualMachineReplicaSet(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineReplicaSetList to the Hub version.
func (src *VirtualMachineReplicaSetList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineReplicaSetList)
	return Convert_v1alpha5_VirtualMachineReplicaSetList_To_v1alpha6_VirtualMachineReplicaSetList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineReplicaSetList.
func (dst *VirtualMachineReplicaSetList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineReplicaSetList)
	return Convert_v1alpha6_VirtualMachineReplicaSetList_To_v1alpha5_VirtualMachineReplicaSetList(src, dst, nil)
}
