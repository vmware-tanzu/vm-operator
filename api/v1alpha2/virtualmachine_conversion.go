// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// ConvertTo converts this VirtualMachine to the Hub version.
func (src *VirtualMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachine)
	if err := Convert_v1alpha2_VirtualMachine_To_v1alpha3_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Manually restore data.
	restored := &v1alpha3.VirtualMachine{}
	if ok, err := utilconversion.UnmarshalData(src, restored); err != nil || !ok {
		return err
	}

	// BEGIN RESTORE

	// END RESTORE

	dst.Status = restored.Status

	return nil
}

// ConvertFrom converts the hub version to this VirtualMachine.
func (dst *VirtualMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachine)
	if err := Convert_v1alpha3_VirtualMachine_To_v1alpha2_VirtualMachine(src, dst, nil); err != nil {
		return err
	}

	// Preserve Hub data on down-conversion except for metadata
	return utilconversion.MarshalData(src, dst)
}

// ConvertTo converts this VirtualMachineList to the Hub version.
func (src *VirtualMachineList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineList)
	return Convert_v1alpha2_VirtualMachineList_To_v1alpha3_VirtualMachineList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineList.
func (dst *VirtualMachineList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineList)
	return Convert_v1alpha3_VirtualMachineList_To_v1alpha2_VirtualMachineList(src, dst, nil)
}
