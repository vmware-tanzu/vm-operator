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

// VirtualMachineReplicaSet is the converter for VirtualMachineReplicaSet across all spoke versions.
var VirtualMachineReplicaSet = whconversion.NewHubSpokeConverter(
	&vmopv1.VirtualMachineReplicaSet{},
	whconversion.NewSpokeConverter(&vmopv1a3.VirtualMachineReplicaSet{}, convertVMReplicaSetHubToV1Alpha3, convertVMReplicaSetV1Alpha3ToHub),
	whconversion.NewSpokeConverter(&vmopv1a4.VirtualMachineReplicaSet{}, convertVMReplicaSetHubToV1Alpha4, convertVMReplicaSetV1Alpha4ToHub),
	whconversion.NewSpokeConverter(&vmopv1a5.VirtualMachineReplicaSet{}, convertVMReplicaSetHubToV1Alpha5, convertVMReplicaSetV1Alpha5ToHub),
)

func convertVMReplicaSetHubToV1Alpha3(_ context.Context, hub *vmopv1.VirtualMachineReplicaSet, spoke *vmopv1a3.VirtualMachineReplicaSet) error {
	if err := vmopv1a3.Convert_v1alpha6_VirtualMachineReplicaSet_To_v1alpha3_VirtualMachineReplicaSet(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMReplicaSetV1Alpha3ToHub(_ context.Context, spoke *vmopv1a3.VirtualMachineReplicaSet, hub *vmopv1.VirtualMachineReplicaSet) error {
	if err := vmopv1a3.Convert_v1alpha3_VirtualMachineReplicaSet_To_v1alpha6_VirtualMachineReplicaSet(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineReplicaSet{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Status = restored.Status
	return nil
}

func convertVMReplicaSetHubToV1Alpha4(_ context.Context, hub *vmopv1.VirtualMachineReplicaSet, spoke *vmopv1a4.VirtualMachineReplicaSet) error {
	if err := vmopv1a4.Convert_v1alpha6_VirtualMachineReplicaSet_To_v1alpha4_VirtualMachineReplicaSet(hub, spoke, nil); err != nil {
		return err
	}
	return utilconversion.MarshalData(hub, spoke)
}

func convertVMReplicaSetV1Alpha4ToHub(_ context.Context, spoke *vmopv1a4.VirtualMachineReplicaSet, hub *vmopv1.VirtualMachineReplicaSet) error {
	if err := vmopv1a4.Convert_v1alpha4_VirtualMachineReplicaSet_To_v1alpha6_VirtualMachineReplicaSet(spoke, hub, nil); err != nil {
		return err
	}
	restored := &vmopv1.VirtualMachineReplicaSet{}
	ok, err := utilconversion.UnmarshalData(spoke, restored)
	if err != nil || !ok {
		return err
	}
	hub.Status = restored.Status
	return nil
}

func convertVMReplicaSetHubToV1Alpha5(_ context.Context, hub *vmopv1.VirtualMachineReplicaSet, spoke *vmopv1a5.VirtualMachineReplicaSet) error {
	return vmopv1a5.Convert_v1alpha6_VirtualMachineReplicaSet_To_v1alpha5_VirtualMachineReplicaSet(hub, spoke, nil)
}

func convertVMReplicaSetV1Alpha5ToHub(_ context.Context, spoke *vmopv1a5.VirtualMachineReplicaSet, hub *vmopv1.VirtualMachineReplicaSet) error {
	return vmopv1a5.Convert_v1alpha5_VirtualMachineReplicaSet_To_v1alpha6_VirtualMachineReplicaSet(spoke, hub, nil)
}
