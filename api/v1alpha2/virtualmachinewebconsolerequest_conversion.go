// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// ConvertTo converts this VirtualMachineWebConsoleRequest to the Hub version.
func (src *VirtualMachineWebConsoleRequest) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineWebConsoleRequest)
	return Convert_v1alpha2_VirtualMachineWebConsoleRequest_To_v1alpha3_VirtualMachineWebConsoleRequest(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineWebConsoleRequest.
func (dst *VirtualMachineWebConsoleRequest) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineWebConsoleRequest)
	return Convert_v1alpha3_VirtualMachineWebConsoleRequest_To_v1alpha2_VirtualMachineWebConsoleRequest(src, dst, nil)
}

// ConvertTo converts this VirtualMachineWebConsoleRequestList to the Hub version.
func (src *VirtualMachineWebConsoleRequestList) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1alpha3.VirtualMachineWebConsoleRequestList)
	return Convert_v1alpha2_VirtualMachineWebConsoleRequestList_To_v1alpha3_VirtualMachineWebConsoleRequestList(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineWebConsoleRequestList.
func (dst *VirtualMachineWebConsoleRequestList) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1alpha3.VirtualMachineWebConsoleRequestList)
	return Convert_v1alpha3_VirtualMachineWebConsoleRequestList_To_v1alpha2_VirtualMachineWebConsoleRequestList(src, dst, nil)
}
