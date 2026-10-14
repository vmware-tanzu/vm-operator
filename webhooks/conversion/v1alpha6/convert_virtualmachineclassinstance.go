// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"

	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachineClassInstance is the converter for VirtualMachineClassInstance across all spoke versions.
var VirtualMachineClassInstance = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineClassInstance{},
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineClassInstance{}, convertVMClassInstanceHubToV1Alpha4, convertVMClassInstanceV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineClassInstance{}, convertVMClassInstanceHubToV1Alpha5, convertVMClassInstanceV1Alpha5ToHub),
)

func convertVMClassInstanceHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineClassInstance, spoke *vmopv1a4.VirtualMachineClassInstance) error {
	return vmopv1a4.Convert_v1alpha6_VirtualMachineClassInstance_To_v1alpha4_VirtualMachineClassInstance(hub, spoke, nil)
}

func convertVMClassInstanceV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineClassInstance, hub *vmopv1.VirtualMachineClassInstance) error {
	return vmopv1a4.Convert_v1alpha4_VirtualMachineClassInstance_To_v1alpha6_VirtualMachineClassInstance(spoke, hub, nil)
}

func convertVMClassInstanceHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineClassInstance, spoke *vmopv1a5.VirtualMachineClassInstance) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineClassInstance_To_v1alpha5_VirtualMachineClassInstance(hub, spoke, nil)
}

func convertVMClassInstanceV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineClassInstance, hub *vmopv1.VirtualMachineClassInstance) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineClassInstance_To_v1alpha6_VirtualMachineClassInstance(spoke, hub, nil)
}
