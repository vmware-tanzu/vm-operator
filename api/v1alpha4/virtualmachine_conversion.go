// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	//
	// Leaving this here for when v1a5 diverges from v1a4.
	//

	//nolint:godot
	// "github.com/vmware-tanzu/vm-operator/api/utilconversion"

	vmopv1a4common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha5/common"
)

func Convert_v1alpha5_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(
	in *vmopv1.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(in, out, s)
}

func Convert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha4_VirtualMachineCryptoStatus(
	in *vmopv1.VirtualMachineCryptoStatus, out *VirtualMachineCryptoStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha4_VirtualMachineCryptoStatus(in, out, s)
}

func Convert_common_LocalObjectRef_To_v1alpha5_VirtualMachineSnapshotReference(
	in *vmopv1a4common.LocalObjectRef, out *vmopv1.VirtualMachineSnapshotReference, s apiconversion.Scope) error {
	if in == nil {
		return nil
	}

	// Convert LocalObjectRef to the nested SnapshotReference field
	out.SnapshotReference = &vmopv1common.LocalObjectRef{
		APIVersion: in.APIVersion,
		Kind:       in.Kind,
		Name:       in.Name,
	}

	return nil
}

func Convert_v1alpha5_VirtualMachineSnapshotReference_To_common_LocalObjectRef(
	in *vmopv1.VirtualMachineSnapshotReference, out *vmopv1a4common.LocalObjectRef, s apiconversion.Scope) error {
	if in == nil || in.SnapshotReference == nil {
		return nil
	}

	// Convert the nested SnapshotReference back to LocalObjectRef
	out.APIVersion = in.SnapshotReference.APIVersion
	out.Kind = in.SnapshotReference.Kind
	out.Name = in.SnapshotReference.Name

	return nil
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha4_VirtualMachine_To_v1alpha5_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	//
	// Leaving this here for when v1a5 diverges from v1a4.
	//

	// Manually restore data.
	// restored := &vmopv1.VirtualMachine{}
	// if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
	// 	return err
	// }

	// // BEGIN RESTORE

	// // END RESTORE

	// dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha5_VirtualMachine_To_v1alpha4_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	//
	// Leaving this here for when v1a5 diverges from v1a4.
	//

	// Preserve Hub data on down-conversion except for metadata
	// return utilconversion.MarshalData(src, dst)

	return nil
}
