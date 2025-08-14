// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	ctrlconversion "sigs.k8s.io/controller-runtime/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// ConvertTo converts this VirtualMachineWebConsoleRequest to the Hub version.
func (src *VirtualMachineWebConsoleRequest) ConvertTo(dstRaw ctrlconversion.Hub) error {
	dst := dstRaw.(*vmopv1.VirtualMachineWebConsoleRequest)
	return Convert_v1alpha4_VirtualMachineWebConsoleRequest_To_v1alpha5_VirtualMachineWebConsoleRequest(src, dst, nil)
}

// ConvertFrom converts the hub version to this VirtualMachineWebConsoleRequest.
func (dst *VirtualMachineWebConsoleRequest) ConvertFrom(srcRaw ctrlconversion.Hub) error {
	src := srcRaw.(*vmopv1.VirtualMachineWebConsoleRequest)
	return Convert_v1alpha5_VirtualMachineWebConsoleRequest_To_v1alpha4_VirtualMachineWebConsoleRequest(src, dst, nil)
}
