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

// VirtualMachineService is the converter for VirtualMachineService across all spoke versions.
var VirtualMachineService = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineService{},
	whconversion.NewSpokeConverter(&vmopv1a1.VirtualMachineService{}, convertVMServiceHubToV1Alpha1, convertVMServiceV1Alpha1ToHub),
	whconversion.NewSpokeConverter(&vmopv1a2.VirtualMachineService{}, convertVMServiceHubToV1Alpha2, convertVMServiceV1Alpha2ToHub),
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineService{}, convertVMServiceHubToV1Alpha3, convertVMServiceV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineService{}, convertVMServiceHubToV1Alpha4, convertVMServiceV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineService{}, convertVMServiceHubToV1Alpha5, convertVMServiceV1Alpha5ToHub),
)

func restoreVirtualMachineServiceIPFamilies(dst, restored *vmopv1.VirtualMachineService) {
	dst.Spec.IPFamilies = restored.Spec.IPFamilies
	dst.Spec.IPFamilyPolicy = restored.Spec.IPFamilyPolicy
}

func convertVMServiceHubToV1Alpha1(_ context.Context, hub *vmopv1.VirtualMachineService, spoke *vmopv1a1.VirtualMachineService) error {
	if err := vmopv1a1.Convert_v1alpha6_VirtualMachineService_To_v1alpha1_VirtualMachineService(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMServiceV1Alpha1ToHub(_ context.Context, spoke *vmopv1a1.VirtualMachineService, hub *vmopv1.VirtualMachineService) error {
	if err := vmopv1a1.Convert_v1alpha1_VirtualMachineService_To_v1alpha6_VirtualMachineService(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineService{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	restoreVirtualMachineServiceIPFamilies(hub, restored)
	hub.Status = restored.Status
	return nil
}

func convertVMServiceHubToV1Alpha2(_ context.Context, hub *vmopv1.VirtualMachineService, spoke *vmopv1a2.VirtualMachineService) error {
	if err := vmopv1a2.Convert_v1alpha6_VirtualMachineService_To_v1alpha2_VirtualMachineService(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMServiceV1Alpha2ToHub(_ context.Context, spoke *vmopv1a2.VirtualMachineService, hub *vmopv1.VirtualMachineService) error {
	if err := vmopv1a2.Convert_v1alpha2_VirtualMachineService_To_v1alpha6_VirtualMachineService(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineService{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	restoreVirtualMachineServiceIPFamilies(hub, restored)
	hub.Status = restored.Status
	return nil
}

func convertVMServiceHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineService, spoke *vmopv1a3.VirtualMachineService) error {
	if err := vmopv1a3.Convert_v1alpha6_VirtualMachineService_To_v1alpha3_VirtualMachineService(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMServiceV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineService, hub *vmopv1.VirtualMachineService) error {
	if err := vmopv1a3.Convert_v1alpha3_VirtualMachineService_To_v1alpha6_VirtualMachineService(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineService{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	restoreVirtualMachineServiceIPFamilies(hub, restored)
	hub.Status = restored.Status
	return nil
}

func convertVMServiceHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineService, spoke *vmopv1a4.VirtualMachineService) error {
	if err := vmopv1a4.Convert_v1alpha6_VirtualMachineService_To_v1alpha4_VirtualMachineService(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMServiceV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineService, hub *vmopv1.VirtualMachineService) error {
	if err := vmopv1a4.Convert_v1alpha4_VirtualMachineService_To_v1alpha6_VirtualMachineService(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineService{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	restoreVirtualMachineServiceIPFamilies(hub, restored)
	hub.Status = restored.Status
	return nil
}

func convertVMServiceHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineService, spoke *vmopv1a5.VirtualMachineService) error {
	if err := vmopv1a5.Convert_v1alpha6_VirtualMachineService_To_v1alpha5_VirtualMachineService(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMServiceV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineService, hub *vmopv1.VirtualMachineService) error {
	if err := vmopv1a5.Convert_v1alpha5_VirtualMachineService_To_v1alpha6_VirtualMachineService(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineService{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	restoreVirtualMachineServiceIPFamilies(hub, restored)
	hub.Status = restored.Status
	return nil
}
