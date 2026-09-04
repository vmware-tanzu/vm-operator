// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	"unsafe"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// ConvertTo converts this VirtualMachineSnapshot to the Hub version.
func (src *VirtualMachineSnapshot) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSnapshot)
	if err := Convert_v1alpha5_VirtualMachineSnapshot_To_v1alpha6_VirtualMachineSnapshot(src, dst, nil); err != nil {
		return err
	}

	restored := &vmopv1.VirtualMachineSnapshot{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil {
		return err
	} else if ok {
		dst.Status.Disks = restored.Status.Disks
	}

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachineSnapshot.
func (dst *VirtualMachineSnapshot) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSnapshot)
	if err := Convert_v1alpha6_VirtualMachineSnapshot_To_v1alpha5_VirtualMachineSnapshot(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineSnapshotList to the Hub version.
func (src *VirtualMachineSnapshotList) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineSnapshotList)
	return Convert_v1alpha5_VirtualMachineSnapshotList_To_v1alpha6_VirtualMachineSnapshotList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineSnapshotList.
func (dst *VirtualMachineSnapshotList) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineSnapshotList)
	return Convert_v1alpha6_VirtualMachineSnapshotList_To_v1alpha5_VirtualMachineSnapshotList(src, dst, nil)
}

// Convert_v1alpha6_VirtualMachineSnapshotStatus_To_v1alpha5_VirtualMachineSnapshotStatus drops
// fields that do not exist in v1alpha5 (Disks).
func Convert_v1alpha6_VirtualMachineSnapshotStatus_To_v1alpha5_VirtualMachineSnapshotStatus(
	in *vmopv1.VirtualMachineSnapshotStatus, out *VirtualMachineSnapshotStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha6_VirtualMachineSnapshotStatus_To_v1alpha5_VirtualMachineSnapshotStatus(in, out, s)
}

// Convert_v1alpha5_VirtualMachineSnapshotStatus_To_v1alpha6_VirtualMachineSnapshotStatus converts
// from v1alpha5 to v1alpha6.
func Convert_v1alpha5_VirtualMachineSnapshotStatus_To_v1alpha6_VirtualMachineSnapshotStatus(
	in *VirtualMachineSnapshotStatus, out *vmopv1.VirtualMachineSnapshotStatus, s apiconversion.Scope) error {

	out.PowerState = vmopv1.VirtualMachinePowerState(in.PowerState)
	out.Quiesced = in.Quiesced
	out.UniqueID = in.UniqueID
	out.Children = *(*[]vmopv1.VirtualMachineSnapshotReference)(unsafe.Pointer(&in.Children))
	out.Conditions = *(*[]metav1.Condition)(unsafe.Pointer(&in.Conditions))
	out.Storage = (*vmopv1.VirtualMachineSnapshotStorageStatus)(unsafe.Pointer(in.Storage))
	return nil
}
