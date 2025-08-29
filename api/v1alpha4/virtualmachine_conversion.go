// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	apiconversion "k8s.io/apimachinery/pkg/conversion"
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

var cdromNameRegex = regexp.MustCompile(`^(ide|sata):([0-3]):([0-29])$`)

func Convert_v1alpha4_VirtualMachineCdromSpec_To_v1alpha5_VirtualMachineCdromSpec(
	in *VirtualMachineCdromSpec, out *vmopv1.VirtualMachineCdromSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineCdromSpec_To_v1alpha5_VirtualMachineCdromSpec(in, out, s); err != nil {
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

func Convert_v1alpha5_VirtualMachineCdromSpec_To_v1alpha4_VirtualMachineCdromSpec(
	in *vmopv1.VirtualMachineCdromSpec, out *VirtualMachineCdromSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha5_VirtualMachineCdromSpec_To_v1alpha4_VirtualMachineCdromSpec(in, out, s); err != nil {
		return err
	}

	out.Name = fmt.Sprintf(
		"%s:%d:%d",
		strings.ToLower(string(in.ControllerType)),
		in.ControllerBusNumber,
		in.UnitNumber)

	return nil
}

func Convert_v1alpha5_PersistentVolumeClaimVolumeSource_To_v1alpha4_PersistentVolumeClaimVolumeSource(
	in *vmopv1.PersistentVolumeClaimVolumeSource, out *PersistentVolumeClaimVolumeSource, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_PersistentVolumeClaimVolumeSource_To_v1alpha4_PersistentVolumeClaimVolumeSource(in, out, s)
}

func Convert_v1alpha5_VirtualMachineVolumeStatus_To_v1alpha4_VirtualMachineVolumeStatus(
	in *vmopv1.VirtualMachineVolumeStatus, out *VirtualMachineVolumeStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineVolumeStatus_To_v1alpha4_VirtualMachineVolumeStatus(in, out, s)
}

func Convert_v1alpha5_VirtualMachineSpec_To_v1alpha4_VirtualMachineSpec(
	in *vmopv1.VirtualMachineSpec, out *VirtualMachineSpec, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineSpec_To_v1alpha4_VirtualMachineSpec(in, out, s)
}

func Convert_v1alpha4_VirtualMachineSpec_To_v1alpha5_VirtualMachineSpec(
	in *VirtualMachineSpec, out *vmopv1.VirtualMachineSpec, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineSpec_To_v1alpha5_VirtualMachineSpec(in, out, s); err != nil {
		return err
	}

	if len(in.Cdrom) > 0 {
		if out.Hardware == nil {
			out.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
		}
		out.Hardware.Cdrom = make([]vmopv1.VirtualMachineCdromSpec, len(in.Cdrom))
		for i := range in.Cdrom {
			if err := autoConvert_v1alpha4_VirtualMachineCdromSpec_To_v1alpha5_VirtualMachineCdromSpec(&in.Cdrom[i], &out.Hardware.Cdrom[i], s); err != nil {
				return err
			}
		}
	}

	return nil
}

func Convert_v1alpha5_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(
	in *vmopv1.VirtualMachineStatus, out *VirtualMachineStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineStatus_To_v1alpha4_VirtualMachineStatus(in, out, s)
}

func Convert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha4_VirtualMachineCryptoStatus(
	in *vmopv1.VirtualMachineCryptoStatus, out *VirtualMachineCryptoStatus, s apiconversion.Scope) error {

	return autoConvert_v1alpha5_VirtualMachineCryptoStatus_To_v1alpha4_VirtualMachineCryptoStatus(in, out, s)
}

func restore_v1alpha5_VirtualMachineHardware(dst, src *vmopv1.VirtualMachine) {
	if src.Spec.Hardware != nil {
		dst.Spec.Hardware = src.Spec.Hardware.DeepCopy()
	} else {
		dst.Spec.Hardware = nil
	}
}

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha4_VirtualMachine_To_v1alpha5_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &vmopv1.VirtualMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	restore_v1alpha5_VirtualMachineHardware(dst, restored)

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachine)
	if err := Convert_v1alpha5_VirtualMachine_To_v1alpha4_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}
