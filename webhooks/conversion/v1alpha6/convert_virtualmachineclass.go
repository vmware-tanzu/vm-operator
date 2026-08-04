// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"

	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachineClass is the converter for VirtualMachineClass across all spoke versions.
var VirtualMachineClass = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineClass{},
	whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachineClass{}, convertVMClassHubToV1Alpha1, convertVMClassV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineClass{}, convertVMClassHubToV1Alpha2, convertVMClassV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineClass{}, convertVMClassHubToV1Alpha3, convertVMClassV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineClass{}, convertVMClassHubToV1Alpha4, convertVMClassV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineClass{}, convertVMClassHubToV1Alpha5, convertVMClassV1Alpha5ToHub),
)

func convertVMClassHubToV1Alpha1(_ context.Context, hub *vmopv1.VirtualMachineClass, spoke *vmopv1a1.VirtualMachineClass) error {
	return vmopv1a1.Convert_v1alpha6_VirtualMachineClass_To_v1alpha1_VirtualMachineClass(hub, spoke, nil)
}

func convertVMClassV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.VirtualMachineClass, hub *vmopv1.VirtualMachineClass) error {
	return vmopv1a1.Convert_v1alpha1_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(spoke, hub, nil)
}

func convertVMClassHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineClass, spoke *vmopv1a2.VirtualMachineClass) error {
	return vmopv1a2.Convert_v1alpha6_VirtualMachineClass_To_v1alpha2_VirtualMachineClass(hub, spoke, nil)
}

func convertVMClassV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineClass, hub *vmopv1.VirtualMachineClass) error {
	return vmopv1a2.Convert_v1alpha2_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(spoke, hub, nil)
}

func convertVMClassHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineClass, spoke *vmopv1a3.VirtualMachineClass) error {
	return vmopv1a3.Convert_v1alpha6_VirtualMachineClass_To_v1alpha3_VirtualMachineClass(hub, spoke, nil)
}

func convertVMClassV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineClass, hub *vmopv1.VirtualMachineClass) error {
	return vmopv1a3.Convert_v1alpha3_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(spoke, hub, nil)
}

func convertVMClassHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineClass, spoke *vmopv1a4.VirtualMachineClass) error {
	return vmopv1a4.Convert_v1alpha6_VirtualMachineClass_To_v1alpha4_VirtualMachineClass(hub, spoke, nil)
}

func convertVMClassV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineClass, hub *vmopv1.VirtualMachineClass) error {
	return vmopv1a4.Convert_v1alpha4_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(spoke, hub, nil)
}

func convertVMClassHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineClass, spoke *vmopv1a5.VirtualMachineClass) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineClass_To_v1alpha5_VirtualMachineClass(hub, spoke, nil)
}

func convertVMClassV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineClass, hub *vmopv1.VirtualMachineClass) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineClass_To_v1alpha6_VirtualMachineClass(spoke, hub, nil)
}
