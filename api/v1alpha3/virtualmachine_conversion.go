// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

func Convert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha3_VirtualMachineCryptoStatus(
	in *vmopv1.VirtualMachineCryptoStatus, out *VirtualMachineCryptoStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha3_VirtualMachineCryptoStatus(in, out, s)
}

func Convert_v1alpha3_VirtualMachineStorageStatus_To_v1alpha5_VirtualMachineStorageStatus(
	in *VirtualMachineStorageStatus, out *vmopv1.VirtualMachineStorageStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStorageStatus_To_v1alpha5_VirtualMachineStorageStatus(in, out, s); err != nil {
		return err
	}

	if in.Usage != nil {
		out.Total = in.Usage.Total
		if in.Usage.Other != nil {
			if out.Used == nil {
				out.Used = &vmopv1.VirtualMachineStorageStatusUsed{}
			}
			out.Used.Other = in.Usage.Other
		}
		if in.Usage.Disks != nil {
			if out.Requested == nil {
				out.Requested = &vmopv1.VirtualMachineStorageStatusRequested{}
			}
			out.Requested.Disks = in.Usage.Disks
		}
	}

	return nil
}

func Convert_v1alpha5_VirtualMachineStorageStatus_To_v1alpha3_VirtualMachineStorageStatus(
	in *vmopv1.VirtualMachineStorageStatus, out *VirtualMachineStorageStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha5_VirtualMachineStorageStatus_To_v1alpha3_VirtualMachineStorageStatus(in, out, s); err != nil {
		return err
	}

	if in.Total != nil {
		if out.Usage == nil {
			out.Usage = &VirtualMachineStorageStatusUsage{}
		}
		out.Usage.Total = in.Total
	}
	if in.Requested != nil {
		if in.Requested.Disks != nil {
			if out.Usage == nil {
				out.Usage = &VirtualMachineStorageStatusUsage{}
			}
			out.Usage.Disks = in.Requested.Disks
		}
	}
	if in.Used != nil {
		if in.Used.Other != nil {
			if out.Usage == nil {
				out.Usage = &VirtualMachineStorageStatusUsage{}
			}
			out.Usage.Other = in.Used.Other
		}
	}

	return nil
}

func Convert_v1alpha5_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(
	in *vmopv1.VirtualMachineVolumeStatus, out *VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha5_VirtualMachineBootstrapCloudInitSpec_To_v1alpha3_VirtualMachineBootstrapCloudInitSpec(
	in *vmopv1.VirtualMachineBootstrapCloudInitSpec, out *VirtualMachineBootstrapCloudInitSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineBootstrapCloudInitSpec_To_v1alpha3_VirtualMachineBootstrapCloudInitSpec(in, out, s)
}

func Convert_v1alpha5_VirtualMachineSpec_To_v1alpha3_VirtualMachineSpec(
	in *vmopv1.VirtualMachineSpec, out *VirtualMachineSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineSpec_To_v1alpha3_VirtualMachineSpec(in, out, s)
}

func Convert_v1alpha5_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(
	in *vmopv1.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha5_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.Host = in.NodeName

	return nil
}

func Convert_v1alpha3_VirtualMachineStatus_To_v1alpha5_VirtualMachineStatus(
	in *VirtualMachineStatus, out *vmopv1.VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStatus_To_v1alpha5_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.NodeName = in.Host

	return nil
}

func restore_v1alpha5_VirtualMachineGroupName(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.GroupName = src.Spec.GroupName
}

func restore_v1alpha5_VirtualMachinePromoteDisksMode(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.PromoteDisksMode = src.Spec.PromoteDisksMode
}

func restore_v1alpha5_VirtualMachineBootOptions(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.BootOptions = src.Spec.BootOptions
}

func restore_v1alpha5_VirtualMachineAffinitySpec(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Affinity = src.Spec.Affinity
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha3_VirtualMachine_To_v1alpha5_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha5_VirtualMachineBootstrapCloudInitWaitOnNetwork(dst, restored)
	restore_v1alpha5_VirtualMachinePromoteDisksMode(dst, restored)
	restore_v1alpha5_VirtualMachineBootOptions(dst, restored)
	restore_v1alpha5_VirtualMachineAffinitySpec(dst, restored)
	restore_v1alpha5_VirtualMachineGroupName(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha5_VirtualMachine_To_v1alpha3_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineList to the Hub version.
func (src *VirtualMachineList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineList)
	return Convert_v1alpha3_VirtualMachineList_To_v1alpha5_VirtualMachineList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineList.
func (dst *VirtualMachineList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineList)
	return Convert_v1alpha5_VirtualMachineList_To_v1alpha3_VirtualMachineList(src, dst, nil)
}

func restore_v1alpha5_VirtualMachineBootstrapCloudInitWaitOnNetwork(dst, src *vmopv1.VirtualMachine) {
	if bs := src.Spec.Bootstrap; bs != nil {
		if ci := bs.CloudInit; ci != nil {
			if ci.WaitOnNetwork4 != nil || ci.WaitOnNetwork6 != nil {
				// Only restore these values if dst still has a CloudInit spec.
				if dst.Spec.Bootstrap != nil && dst.Spec.Bootstrap.CloudInit != nil {
					dst.Spec.Bootstrap.CloudInit.WaitOnNetwork4 = ci.WaitOnNetwork4
					dst.Spec.Bootstrap.CloudInit.WaitOnNetwork6 = ci.WaitOnNetwork6
				}
			}
		}
	}
}
