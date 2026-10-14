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

// VirtualMachineWebConsoleRequest is the converter for VirtualMachineWebConsoleRequest across all spoke versions.
var VirtualMachineWebConsoleRequest = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineWebConsoleRequest{},
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineWebConsoleRequest{}, convertVMWebConsoleRequestHubToV1Alpha2, convertVMWebConsoleRequestV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineWebConsoleRequest{}, convertVMWebConsoleRequestHubToV1Alpha3, convertVMWebConsoleRequestV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineWebConsoleRequest{}, convertVMWebConsoleRequestHubToV1Alpha4, convertVMWebConsoleRequestV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineWebConsoleRequest{}, convertVMWebConsoleRequestHubToV1Alpha5, convertVMWebConsoleRequestV1Alpha5ToHub),
)

func convertVMWebConsoleRequestHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineWebConsoleRequest, spoke *vmopv1a2.VirtualMachineWebConsoleRequest) error {
	return vmopv1a2.Convert_v1alpha6_VirtualMachineWebConsoleRequest_To_v1alpha2_VirtualMachineWebConsoleRequest(hub, spoke, nil)
}

func convertVMWebConsoleRequestV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineWebConsoleRequest, hub *vmopv1.VirtualMachineWebConsoleRequest) error {
	return vmopv1a2.Convert_v1alpha2_VirtualMachineWebConsoleRequest_To_v1alpha6_VirtualMachineWebConsoleRequest(spoke, hub, nil)
}

func convertVMWebConsoleRequestHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineWebConsoleRequest, spoke *vmopv1a3.VirtualMachineWebConsoleRequest) error {
	return vmopv1a3.Convert_v1alpha6_VirtualMachineWebConsoleRequest_To_v1alpha3_VirtualMachineWebConsoleRequest(hub, spoke, nil)
}

func convertVMWebConsoleRequestV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineWebConsoleRequest, hub *vmopv1.VirtualMachineWebConsoleRequest) error {
	return vmopv1a3.Convert_v1alpha3_VirtualMachineWebConsoleRequest_To_v1alpha6_VirtualMachineWebConsoleRequest(spoke, hub, nil)
}

func convertVMWebConsoleRequestHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineWebConsoleRequest, spoke *vmopv1a4.VirtualMachineWebConsoleRequest) error {
	return vmopv1a4.Convert_v1alpha6_VirtualMachineWebConsoleRequest_To_v1alpha4_VirtualMachineWebConsoleRequest(hub, spoke, nil)
}

func convertVMWebConsoleRequestV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineWebConsoleRequest, hub *vmopv1.VirtualMachineWebConsoleRequest) error {
	return vmopv1a4.Convert_v1alpha4_VirtualMachineWebConsoleRequest_To_v1alpha6_VirtualMachineWebConsoleRequest(spoke, hub, nil)
}

func convertVMWebConsoleRequestHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineWebConsoleRequest, spoke *vmopv1a5.VirtualMachineWebConsoleRequest) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineWebConsoleRequest_To_v1alpha5_VirtualMachineWebConsoleRequest(hub, spoke, nil)
}

func convertVMWebConsoleRequestV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineWebConsoleRequest, hub *vmopv1.VirtualMachineWebConsoleRequest) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineWebConsoleRequest_To_v1alpha6_VirtualMachineWebConsoleRequest(spoke, hub, nil)
}
