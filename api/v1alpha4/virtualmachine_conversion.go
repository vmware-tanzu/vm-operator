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

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

func Convert_v1alpha5_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(
	in *vmopv1.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(in, out, s)
}

func Convert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha4_VirtualMachineCryptoStatus(
	in *vmopv1.VirtualMachineCryptoStatus, out *VirtualMachineCryptoStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha4_VirtualMachineCryptoStatus(in, out, s)
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
