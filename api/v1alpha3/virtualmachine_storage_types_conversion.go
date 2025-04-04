// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	apiconversion "k8s.io/apimachinery/pkg/conversion"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

func Convert_v1alpha3_VirtualMachineStorageStatusUsage_To_v1alpha4_VirtualMachineStorageStatusUsage(
	in *VirtualMachineStorageStatusUsage, out *vmopv1.VirtualMachineStorageStatusUsage, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha3_VirtualMachineStorageStatusUsage_To_v1alpha4_VirtualMachineStorageStatusUsage(in, out, s); err != nil {
		return err
	}

	out.Total = in.Unshared

	return nil
}

func Convert_v1alpha4_VirtualMachineStorageStatusUsage_To_v1alpha3_VirtualMachineStorageStatusUsage(
	in *vmopv1.VirtualMachineStorageStatusUsage, out *VirtualMachineStorageStatusUsage, s apiconversion.Scope) error {

	if err := autoConvert_v1alpha4_VirtualMachineStorageStatusUsage_To_v1alpha3_VirtualMachineStorageStatusUsage(in, out, s); err != nil {
		return err
	}

	out.Unshared = in.Total

	return nil
}
