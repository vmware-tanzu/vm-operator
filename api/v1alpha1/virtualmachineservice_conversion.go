// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/conversion"
)

// ConvertTo converts this VirtualMachineService to the Hub version.
func (src *VirtualMachineService) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineService)
	return Convert_v1alpha1_VirtualMachineService_To_v1alpha3_VirtualMachineService(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineService.
func (dst *VirtualMachineService) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineService)
	return Convert_v1alpha3_VirtualMachineService_To_v1alpha1_VirtualMachineService(src, dst, nil)
}

// ConvertTo converts this VirtualMachineServiceList to the Hub version.
func (src *VirtualMachineServiceList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineServiceList)
	return Convert_v1alpha1_VirtualMachineServiceList_To_v1alpha3_VirtualMachineServiceList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineServiceList.
func (dst *VirtualMachineServiceList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineServiceList)
	return Convert_v1alpha3_VirtualMachineServiceList_To_v1alpha1_VirtualMachineServiceList(src, dst, nil)
}
