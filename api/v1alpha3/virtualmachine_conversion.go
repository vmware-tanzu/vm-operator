// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func Convert_v1alpha3_VirtualMachineStorageStatus_To_v1alpha4_VirtualMachineStorageStatus(
	in *VirtualMachineStorageStatus, out *vmopv1.VirtualMachineStorageStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStorageStatus_To_v1alpha4_VirtualMachineStorageStatus(in, out, s); err != nil {
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

var cdromNameRegex = regexp.MustCompile(`^(ide|sata):([0-3]):([0-29])$`)

func Convert_v1alpha3_VirtualMachineCdromSpec_To_v1alpha4_VirtualMachineCdromSpec(
	in *VirtualMachineCdromSpec, out *vmopv1.VirtualMachineCdromSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineCdromSpec_To_v1alpha4_VirtualMachineCdromSpec(in, out, s); err != nil {
		return err
	}

	if m := cdromNameRegex.FindStringSubmatch(in.Name); len(m) != 0 {
		switch vmopv1.VirtualControllerType(strings.ToUpper(m[1])) {
		case vmopv1.VirtualControllerTypeIDE:
			out.ControllerType = vmopv1.VirtualControllerTypeIDE
		case vmopv1.VirtualControllerTypeSATA:
			out.ControllerType = vmopv1.VirtualControllerTypeSATA
		}

		busNumber, err := strconv.ParseInt(m[2], 10, 32)
		if err != nil {
			return err
		}
		unitNumber, err := strconv.ParseInt(m[3], 10, 32)
		if err != nil {
			return err
		}
		busNumber32, unitNumber32 := int32(busNumber), int32(unitNumber)
		out.ControllerBusNumber = &busNumber32
		out.UnitNumber = &unitNumber32
	}

	return nil
}

func Convert_v1alpha4_VirtualMachineStorageStatus_To_v1alpha3_VirtualMachineStorageStatus(
	in *vmopv1.VirtualMachineStorageStatus, out *VirtualMachineStorageStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineStorageStatus_To_v1alpha3_VirtualMachineStorageStatus(in, out, s); err != nil {
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

func Convert_v1alpha4_VirtualMachineCdromSpec_To_v1alpha3_VirtualMachineCdromSpec(
	in *vmopv1.VirtualMachineCdromSpec, out *VirtualMachineCdromSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineCdromSpec_To_v1alpha3_VirtualMachineCdromSpec(in, out, s); err != nil {
		return err
	}

	out.Name = fmt.Sprintf(
		"%s:%d:%d",
		strings.ToLower(string(in.ControllerType)),
		in.ControllerBusNumber,
		in.UnitNumber)

	return nil
}

func Convert_v1alpha4_VirtualMachineBootstrapCloudInitSpec_To_v1alpha3_VirtualMachineBootstrapCloudInitSpec(
	in *vmopv1.VirtualMachineBootstrapCloudInitSpec, out *VirtualMachineBootstrapCloudInitSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha4_VirtualMachineBootstrapCloudInitSpec_To_v1alpha3_VirtualMachineBootstrapCloudInitSpec(in, out, s)
}

func Convert_v1alpha4_PersistentVolumeClaimVolumeSource_To_v1alpha3_PersistentVolumeClaimVolumeSource(
	in *vmopv1.PersistentVolumeClaimVolumeSource, out *PersistentVolumeClaimVolumeSource, s apiconversion.Scope) error {

	return autoConvert_v1alpha4_PersistentVolumeClaimVolumeSource_To_v1alpha3_PersistentVolumeClaimVolumeSource(in, out, s)
}

func Convert_v1alpha4_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(
	in *vmopv1.VirtualMachineVolumeStatus, out *VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha4_VirtualMachineVolumeStatus_To_v1alpha3_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha4_VirtualMachineSpec_To_v1alpha3_VirtualMachineSpec(
	in *vmopv1.VirtualMachineSpec, out *VirtualMachineSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha4_VirtualMachineSpec_To_v1alpha3_VirtualMachineSpec(in, out, s)
}

func Convert_v1alpha4_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(
	in *vmopv1.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineStatus_To_v1alpha3_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.Host = in.NodeName

	return nil
}

func Convert_v1alpha3_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(
	in *VirtualMachineStatus, out *vmopv1.VirtualMachineStatus, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(in, out, s); err != nil {
		return err
	}

	out.NodeName = in.Host

	return nil
}

func restore_v1alpha4_VirtualMachineGroupName(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.GroupName = src.Spec.GroupName
}

func Convert_v1alpha3_VirtualMachineSpec_To_v1alpha4_VirtualMachineSpec(
	in *VirtualMachineSpec, out *vmopv1.VirtualMachineSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineSpec_To_v1alpha4_VirtualMachineSpec(in, out, s); err != nil {
		return err
	}

	if len(in.Cdrom) > 0 {
		if out.Hardware == nil {
			out.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
		}
		out.Hardware.Cdrom = make([]vmopv1.VirtualMachineCdromSpec, len(in.Cdrom))
		for i := range in.Cdrom {
			if err := autoConvert_v1alpha3_VirtualMachineCdromSpec_To_v1alpha4_VirtualMachineCdromSpec(&in.Cdrom[i], &out.Hardware.Cdrom[i], s); err != nil {
				return err
			}
		}
	}

	return nil
}

func restore_v1alpha4_VirtualMachinePromoteDisksMode(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.PromoteDisksMode = src.Spec.PromoteDisksMode
}

func restore_v1alpha4_VirtualMachineBootOptions(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.BootOptions = src.Spec.BootOptions
}

func restore_v1alpha4_VirtualMachineAffinitySpec(dst, src *vmopv1.VirtualMachine) {
	dst.Spec.Affinity = src.Spec.Affinity
}

func restore_v1alpha4_VirtualMachineVolumes(dst, src *vmopv1.VirtualMachine) {
	srcVolMap := map[string]*vmopv1.VirtualMachineVolume{}
	for i := range src.Spec.Volumes {
		vol := &src.Spec.Volumes[i]
		srcVolMap[vol.Name] = vol
	}
	for i := range dst.Spec.Volumes {
		dstVol := &dst.Spec.Volumes[i]
		if srcVol, ok := srcVolMap[dstVol.Name]; ok {
			if dstPvc := dstVol.PersistentVolumeClaim; dstPvc != nil {
				if srcPvc := srcVol.PersistentVolumeClaim; srcPvc != nil {
					dstPvc.ApplicationType = srcPvc.ApplicationType
					dstPvc.ControllerBusNumber = srcPvc.ControllerBusNumber
					dstPvc.ControllerType = srcPvc.ControllerType
					dstPvc.DiskMode = srcPvc.DiskMode
					dstPvc.SharingMode = srcPvc.SharingMode
					dstPvc.UnitNumber = srcPvc.UnitNumber
				}
			}
		}
	}
}

func restore_v1alpha4_VirtualMachineHardware(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Hardware != nil {
		dst.Spec.Hardware = src.Spec.Hardware.DeepCopy()
	} else {
		dst.Spec.Hardware = nil
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

	restore_v1alpha4_VirtualMachineBootstrapCloudInitWaitOnNetwork(dst, restored)
	restore_v1alpha4_VirtualMachinePromoteDisksMode(dst, restored)
	restore_v1alpha4_VirtualMachineBootOptions(dst, restored)
	restore_v1alpha4_VirtualMachineAffinitySpec(dst, restored)
	restore_v1alpha4_VirtualMachineGroupName(dst, restored)
	restore_v1alpha4_VirtualMachineVolumes(dst, restored)
	restore_v1alpha4_VirtualMachineHardware(dst, restored)

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

func restore_v1alpha4_VirtualMachineBootstrapCloudInitWaitOnNetwork(dst, src *vmopv1.VirtualMachine) {
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
