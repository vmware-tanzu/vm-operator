// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func Convert_v1alpha4_VirtualMachineBootstrapSpec_To_v1alpha3_VirtualMachineBootstrapSpec(
	in *vmopv1.VirtualMachineBootstrapSpec, out *VirtualMachineBootstrapSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha4_VirtualMachineBootstrapSpec_To_v1alpha3_VirtualMachineBootstrapSpec(in, out, s)
}

func restore_v1alpha4_VirtualMachineBootstrapJoinDomainMode(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Bootstrap != nil {
		if dst.Spec.Bootstrap == nil {
			dst.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{}
		}
		dst.Spec.Bootstrap.JoinDomainMode = src.Spec.Bootstrap.JoinDomainMode
	}
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha3_VirtualMachine_To_v1alpha4_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha4_VirtualMachineBootstrapJoinDomainMode(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha4_VirtualMachine_To_v1alpha3_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineList to the Hub version.
func (src *VirtualMachineList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineList)
	return Convert_v1alpha3_VirtualMachineList_To_v1alpha4_VirtualMachineList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineList.
func (dst *VirtualMachineList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineList)
	return Convert_v1alpha4_VirtualMachineList_To_v1alpha3_VirtualMachineList(src, dst, nil)
}
