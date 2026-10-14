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

// VirtualMachineGroupPublishRequest is the converter for VirtualMachineGroupPublishRequest across all spoke versions.
var VirtualMachineGroupPublishRequest = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineGroupPublishRequest{},
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineGroupPublishRequest{}, convertVMGroupPublishRequestHubToV1Alpha5, convertVMGroupPublishRequestV1Alpha5ToHub),
)

func convertVMGroupPublishRequestHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineGroupPublishRequest, spoke *vmopv1a5.VirtualMachineGroupPublishRequest) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineGroupPublishRequest_To_v1alpha5_VirtualMachineGroupPublishRequest(hub, spoke, nil)
}

func convertVMGroupPublishRequestV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineGroupPublishRequest, hub *vmopv1.VirtualMachineGroupPublishRequest) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineGroupPublishRequest_To_v1alpha6_VirtualMachineGroupPublishRequest(spoke, hub, nil)
}
