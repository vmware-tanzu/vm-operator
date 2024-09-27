// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"

	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// VirtualMachineProviderInterface is a pluggable interface for VM Providers.
type VirtualMachineProviderInterface interface {
	CreateOrUpdateVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	DeleteVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	PublishVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine,
		vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error)
	GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
	GetVirtualMachineProperties(ctx context.Context, vm *vmopv1.VirtualMachine, propertyPaths []string) (map[string]any, error)
	GetVirtualMachineWebMKSTicket(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error)
	GetVirtualMachineHardwareVersion(ctx context.Context, vm *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error)

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error
	IsVirtualMachineSetResourcePolicyReady(ctx context.Context, availabilityZoneName string, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error

	// "Infra" related
	UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error
	ResetVcClient(ctx context.Context)
	ComputeCPUMinFrequency(ctx context.Context) error

	GetItemFromLibraryByName(ctx context.Context, contentLibrary, itemName string) (*library.Item, error)
	UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error
	SyncVirtualMachineImage(ctx context.Context, cli, vmi client.Object) error

	GetTasksByActID(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error)

	// DoesProfileSupportEncryption returns true if the specified profile
	// supports encryption by checking whether or not the underlying policy
	// contains any IOFILTERs.
	DoesProfileSupportEncryption(ctx context.Context, profileID string) (bool, error)
}
