// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"

	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachineGroup is the converter for VirtualMachineGroup across all spoke versions.
var VirtualMachineGroup = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineGroup{},
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineGroup{}, convertVMGroupHubToV1Alpha2, convertVMGroupV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineGroup{}, convertVMGroupHubToV1Alpha3, convertVMGroupV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineGroup{}, convertVMGroupHubToV1Alpha4, convertVMGroupV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineGroup{}, convertVMGroupHubToV1Alpha5, convertVMGroupV1Alpha5ToHub),
)

func convertVMGroupHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineGroup, spoke *vmopv1a2.VirtualMachineGroup) error {
	return vmopv1a2.Convert_v1alpha6_VirtualMachineGroup_To_v1alpha2_VirtualMachineGroup(hub, spoke, nil)
}

func convertVMGroupV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineGroup, hub *vmopv1.VirtualMachineGroup) error {
	return vmopv1a2.Convert_v1alpha2_VirtualMachineGroup_To_v1alpha6_VirtualMachineGroup(spoke, hub, nil)
}

func convertVMGroupHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineGroup, spoke *vmopv1a3.VirtualMachineGroup) error {
	return vmopv1a3.Convert_v1alpha6_VirtualMachineGroup_To_v1alpha3_VirtualMachineGroup(hub, spoke, nil)
}

func convertVMGroupV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineGroup, hub *vmopv1.VirtualMachineGroup) error {
	return vmopv1a3.Convert_v1alpha3_VirtualMachineGroup_To_v1alpha6_VirtualMachineGroup(spoke, hub, nil)
}

func convertVMGroupHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineGroup, spoke *vmopv1a4.VirtualMachineGroup) error {
	return vmopv1a4.Convert_v1alpha6_VirtualMachineGroup_To_v1alpha4_VirtualMachineGroup(hub, spoke, nil)
}

func convertVMGroupV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineGroup, hub *vmopv1.VirtualMachineGroup) error {
	return vmopv1a4.Convert_v1alpha4_VirtualMachineGroup_To_v1alpha6_VirtualMachineGroup(spoke, hub, nil)
}

func convertVMGroupHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineGroup, spoke *vmopv1a5.VirtualMachineGroup) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineGroup_To_v1alpha5_VirtualMachineGroup(hub, spoke, nil)
}

func convertVMGroupV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineGroup, hub *vmopv1.VirtualMachineGroup) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineGroup_To_v1alpha6_VirtualMachineGroup(spoke, hub, nil)
}
