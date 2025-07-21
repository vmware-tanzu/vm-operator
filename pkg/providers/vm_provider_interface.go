// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package providers

import (
	"context"
	"errors"

	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

var (
	// ErrTooManyCreates is returned from the CreateOrUpdateVirtualMachine and
	// CreateOrUpdateVirtualMachineAsync functions when the number of create
	// threads/goroutines have reached the allowed limit.
	ErrTooManyCreates = errors.New("too many creates")

	// ErrReconcileInProgress is returned from the
	// CreateOrUpdateVirtualMachine and DeleteVirtualMachine functions when
	// the VM is still being reconciled in a background thread.
	ErrReconcileInProgress = errors.New("reconcile already in progress")
)

type VMGroupPlacement struct {
	VMGroup   *vmopv1.VirtualMachineGroup
	VMMembers []*vmopv1.VirtualMachine
}

// VirtualMachineProviderInterface is a pluggable interface for VM Providers.
type VirtualMachineProviderInterface interface {
	CreateOrUpdateVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	CreateOrUpdateVirtualMachineAsync(ctx context.Context, vm *vmopv1.VirtualMachine) (<-chan error, error)
	DeleteVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error
	PublishVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine,
		vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error)
	GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
	GetVirtualMachineProperties(ctx context.Context, vm *vmopv1.VirtualMachine, propertyPaths []string) (map[string]any, error)
	GetVirtualMachineWebMKSTicket(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error)
	GetVirtualMachineHardwareVersion(ctx context.Context, vm *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error)
	PlaceVirtualMachineGroup(ctx context.Context, group *vmopv1.VirtualMachineGroup, groupPlacements []VMGroupPlacement) error

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error

	// "Infra" related
	UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error
	UpdateVcCreds(ctx context.Context, data map[string][]byte) error
	ComputeCPUMinFrequency(ctx context.Context) error

	GetItemFromLibraryByName(ctx context.Context, contentLibrary, itemName string) (*library.Item, error)
	UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error
	SyncVirtualMachineImage(ctx context.Context, cli, vmi ctrlclient.Object) error

	GetTasksByActID(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error)

	// DoesProfileSupportEncryption returns true if the specified profile
	// supports encryption by checking whether or not the underlying policy
	// contains any IOFILTERs.
	DoesProfileSupportEncryption(ctx context.Context, profileID string) (bool, error)

	// VSphereClient returns the provider's vSphere client.
	VSphereClient(context.Context) (*client.Client, error)

	// DeleteSnapshot deletes a snapshot from a virtual machine.
	DeleteSnapshot(ctx context.Context, vmSnapshot *vmopv1.VirtualMachineSnapshot,
		vm *vmopv1.VirtualMachine, removeChildren bool, consolidate *bool) (bool, error)
}
