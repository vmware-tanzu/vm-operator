// Copyright (c) 2018-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmprovider

import (
	"context"

	"github.com/vmware/govmomi/vapi/library"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
)

// VirtualMachineProviderInterface is a plugable interface for VM Providers.
type VirtualMachineProviderInterface interface {
	CreateOrUpdateVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	DeleteVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	PublishVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine,
		vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error)
	BackupVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
	GetVirtualMachineWebMKSTicket(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error)
	GetVirtualMachineHardwareVersion(ctx context.Context, vm *vmopv1.VirtualMachine) (int32, error)

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error
	IsVirtualMachineSetResourcePolicyReady(ctx context.Context, availabilityZoneName string, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error

	// "Infra" related
	UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error
	ResetVcClient(ctx context.Context)
	ComputeCPUMinFrequency(ctx context.Context) error

	ListItemsFromContentLibrary(ctx context.Context, contentLibrary *vmopv1.ContentLibraryProvider) ([]string, error)
	GetVirtualMachineImageFromContentLibrary(ctx context.Context, contentLibrary *vmopv1.ContentLibraryProvider, itemID string,
		currentCLImages map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error)
	GetItemFromLibraryByName(ctx context.Context, contentLibrary, itemName string) (*library.Item, error)
	UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error
	SyncVirtualMachineImage(ctx context.Context, cli, vmi client.Object) error

	GetTasksByActID(ctx context.Context, actID string) (tasksInfo []vimTypes.TaskInfo, retErr error)
}
