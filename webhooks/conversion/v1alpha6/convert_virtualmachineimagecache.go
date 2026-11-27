// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import (
	"context"

	whconversion "sigs.k8s.io/controller-runtime/pkg/webhook/conversion"

	"github.com/vmware-tanzu/vm-operator/api/utilconversion"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// VirtualMachineImageCache is the converter for VirtualMachineImageCache across all spoke versions.
var VirtualMachineImageCache = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineImageCache{},
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineImageCache{}, convertVMImageCacheHubToV1Alpha3, convertVMImageCacheV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineImageCache{}, convertVMImageCacheHubToV1Alpha4, convertVMImageCacheV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineImageCache{}, convertVMImageCacheHubToV1Alpha5, convertVMImageCacheV1Alpha5ToHub),
)

func convertVMImageCacheHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineImageCache, spoke *vmopv1a3.VirtualMachineImageCache) error {
	if err := vmopv1a3.Convert_v1alpha6_VirtualMachineImageCache_To_v1alpha3_VirtualMachineImageCache(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMImageCacheV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineImageCache, hub *vmopv1.VirtualMachineImageCache) error {
	if err := vmopv1a3.Convert_v1alpha3_VirtualMachineImageCache_To_v1alpha6_VirtualMachineImageCache(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineImageCache{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Status = restored.Status
	return nil
}

func convertVMImageCacheHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineImageCache, spoke *vmopv1a4.VirtualMachineImageCache) error {
	if err := vmopv1a4.Convert_v1alpha6_VirtualMachineImageCache_To_v1alpha4_VirtualMachineImageCache(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMImageCacheV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineImageCache, hub *vmopv1.VirtualMachineImageCache) error {
	if err := vmopv1a4.Convert_v1alpha4_VirtualMachineImageCache_To_v1alpha6_VirtualMachineImageCache(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineImageCache{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Status = restored.Status
	return nil
}

func convertVMImageCacheHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineImageCache, spoke *vmopv1a5.VirtualMachineImageCache) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineImageCache_To_v1alpha5_VirtualMachineImageCache(hub, spoke, nil)
}

func convertVMImageCacheV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineImageCache, hub *vmopv1.VirtualMachineImageCache) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineImageCache_To_v1alpha6_VirtualMachineImageCache(spoke, hub, nil)
}
