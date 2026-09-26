// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"

	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachineSnapshot is the converter for VirtualMachineSnapshot across all spoke versions.
var VirtualMachineSnapshot = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineSnapshot{},
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineSnapshot{}, convertVMSnapshotHubToV1Alpha5, convertVMSnapshotV1Alpha5ToHub),
)

func convertVMSnapshotHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineSnapshot, spoke *vmopv1a5.VirtualMachineSnapshot) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineSnapshot_To_v1alpha5_VirtualMachineSnapshot(hub, spoke, nil)
}

func convertVMSnapshotV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineSnapshot, hub *vmopv1.VirtualMachineSnapshot) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineSnapshot_To_v1alpha6_VirtualMachineSnapshot(spoke, hub, nil)
}
