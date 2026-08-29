// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"

	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachinePublishRequest is the converter for VirtualMachinePublishRequest across all spoke versions.
var VirtualMachinePublishRequest = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachinePublishRequest{},
	whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachinePublishRequest{}, convertVMPublishRequestHubToV1Alpha1, convertVMPublishRequestV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachinePublishRequest{}, convertVMPublishRequestHubToV1Alpha2, convertVMPublishRequestV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachinePublishRequest{}, convertVMPublishRequestHubToV1Alpha3, convertVMPublishRequestV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachinePublishRequest{}, convertVMPublishRequestHubToV1Alpha4, convertVMPublishRequestV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachinePublishRequest{}, convertVMPublishRequestHubToV1Alpha5, convertVMPublishRequestV1Alpha5ToHub),
)

func convertVMPublishRequestHubToV1Alpha1(_ context.Context, hub *vmopv1.VirtualMachinePublishRequest, spoke *vmopv1a1.VirtualMachinePublishRequest) error {
	if err := vmopv1a1.Convert_v1alpha6_VirtualMachinePublishRequest_To_v1alpha1_VirtualMachinePublishRequest(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMPublishRequestV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.VirtualMachinePublishRequest, hub *vmopv1.VirtualMachinePublishRequest) error {
	if err := vmopv1a1.Convert_v1alpha1_VirtualMachinePublishRequest_To_v1alpha6_VirtualMachinePublishRequest(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachinePublishRequest{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Spec.BackoffLimit = restored.Spec.BackoffLimit
	return nil
}

func convertVMPublishRequestHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachinePublishRequest, spoke *vmopv1a2.VirtualMachinePublishRequest) error {
	if err := vmopv1a2.Convert_v1alpha6_VirtualMachinePublishRequest_To_v1alpha2_VirtualMachinePublishRequest(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMPublishRequestV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachinePublishRequest, hub *vmopv1.VirtualMachinePublishRequest) error {
	if err := vmopv1a2.Convert_v1alpha2_VirtualMachinePublishRequest_To_v1alpha6_VirtualMachinePublishRequest(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachinePublishRequest{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Spec.BackoffLimit = restored.Spec.BackoffLimit
	return nil
}

func convertVMPublishRequestHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachinePublishRequest, spoke *vmopv1a3.VirtualMachinePublishRequest) error {
	if err := vmopv1a3.Convert_v1alpha6_VirtualMachinePublishRequest_To_v1alpha3_VirtualMachinePublishRequest(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMPublishRequestV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachinePublishRequest, hub *vmopv1.VirtualMachinePublishRequest) error {
	if err := vmopv1a3.Convert_v1alpha3_VirtualMachinePublishRequest_To_v1alpha6_VirtualMachinePublishRequest(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachinePublishRequest{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Spec.BackoffLimit = restored.Spec.BackoffLimit
	return nil
}

func convertVMPublishRequestHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachinePublishRequest, spoke *vmopv1a4.VirtualMachinePublishRequest) error {
	if err := vmopv1a4.Convert_v1alpha6_VirtualMachinePublishRequest_To_v1alpha4_VirtualMachinePublishRequest(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMPublishRequestV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachinePublishRequest, hub *vmopv1.VirtualMachinePublishRequest) error {
	if err := vmopv1a4.Convert_v1alpha4_VirtualMachinePublishRequest_To_v1alpha6_VirtualMachinePublishRequest(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachinePublishRequest{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Spec.BackoffLimit = restored.Spec.BackoffLimit
	return nil
}

func convertVMPublishRequestHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachinePublishRequest, spoke *vmopv1a5.VirtualMachinePublishRequest) error {
	if err := vmopv1a5.Convert_v1alpha6_VirtualMachinePublishRequest_To_v1alpha5_VirtualMachinePublishRequest(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMPublishRequestV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachinePublishRequest, hub *vmopv1.VirtualMachinePublishRequest) error {
	if err := vmopv1a5.Convert_v1alpha5_VirtualMachinePublishRequest_To_v1alpha6_VirtualMachinePublishRequest(spoke, hub, nil); err != nil {
		return err
	}
	return nil
}
