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

// VirtualMachineSetResourcePolicy is the converter for VirtualMachineSetResourcePolicy across all spoke versions.
var VirtualMachineSetResourcePolicy = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineSetResourcePolicy{},
	whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachineSetResourcePolicy{}, convertVMSetResourcePolicyHubToV1Alpha1, convertVMSetResourcePolicyV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineSetResourcePolicy{}, convertVMSetResourcePolicyHubToV1Alpha2, convertVMSetResourcePolicyV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineSetResourcePolicy{}, convertVMSetResourcePolicyHubToV1Alpha3, convertVMSetResourcePolicyV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineSetResourcePolicy{}, convertVMSetResourcePolicyHubToV1Alpha4, convertVMSetResourcePolicyV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineSetResourcePolicy{}, convertVMSetResourcePolicyHubToV1Alpha5, convertVMSetResourcePolicyV1Alpha5ToHub),
)

func convertVMSetResourcePolicyHubToV1Alpha1(_ context.Context, hub *vmopv1.VirtualMachineSetResourcePolicy, spoke *vmopv1a1.VirtualMachineSetResourcePolicy) error {
	return vmopv1a1.Convert_v1alpha6_VirtualMachineSetResourcePolicy_To_v1alpha1_VirtualMachineSetResourcePolicy(hub, spoke, nil)
}

func convertVMSetResourcePolicyV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.VirtualMachineSetResourcePolicy, hub *vmopv1.VirtualMachineSetResourcePolicy) error {
	return vmopv1a1.Convert_v1alpha1_VirtualMachineSetResourcePolicy_To_v1alpha6_VirtualMachineSetResourcePolicy(spoke, hub, nil)
}

func convertVMSetResourcePolicyHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineSetResourcePolicy, spoke *vmopv1a2.VirtualMachineSetResourcePolicy) error {
	return vmopv1a2.Convert_v1alpha6_VirtualMachineSetResourcePolicy_To_v1alpha2_VirtualMachineSetResourcePolicy(hub, spoke, nil)
}

func convertVMSetResourcePolicyV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineSetResourcePolicy, hub *vmopv1.VirtualMachineSetResourcePolicy) error {
	return vmopv1a2.Convert_v1alpha2_VirtualMachineSetResourcePolicy_To_v1alpha6_VirtualMachineSetResourcePolicy(spoke, hub, nil)
}

func convertVMSetResourcePolicyHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineSetResourcePolicy, spoke *vmopv1a3.VirtualMachineSetResourcePolicy) error {
	return vmopv1a3.Convert_v1alpha6_VirtualMachineSetResourcePolicy_To_v1alpha3_VirtualMachineSetResourcePolicy(hub, spoke, nil)
}

func convertVMSetResourcePolicyV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineSetResourcePolicy, hub *vmopv1.VirtualMachineSetResourcePolicy) error {
	return vmopv1a3.Convert_v1alpha3_VirtualMachineSetResourcePolicy_To_v1alpha6_VirtualMachineSetResourcePolicy(spoke, hub, nil)
}

func convertVMSetResourcePolicyHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineSetResourcePolicy, spoke *vmopv1a4.VirtualMachineSetResourcePolicy) error {
	return vmopv1a4.Convert_v1alpha6_VirtualMachineSetResourcePolicy_To_v1alpha4_VirtualMachineSetResourcePolicy(hub, spoke, nil)
}

func convertVMSetResourcePolicyV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineSetResourcePolicy, hub *vmopv1.VirtualMachineSetResourcePolicy) error {
	return vmopv1a4.Convert_v1alpha4_VirtualMachineSetResourcePolicy_To_v1alpha6_VirtualMachineSetResourcePolicy(spoke, hub, nil)
}

func convertVMSetResourcePolicyHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineSetResourcePolicy, spoke *vmopv1a5.VirtualMachineSetResourcePolicy) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineSetResourcePolicy_To_v1alpha5_VirtualMachineSetResourcePolicy(hub, spoke, nil)
}

func convertVMSetResourcePolicyV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineSetResourcePolicy, hub *vmopv1.VirtualMachineSetResourcePolicy) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineSetResourcePolicy_To_v1alpha6_VirtualMachineSetResourcePolicy(spoke, hub, nil)
}
