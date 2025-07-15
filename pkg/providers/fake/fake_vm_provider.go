// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/vmware/govmomi/vapi/library"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/providers"
	vsclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

// This Fake Provider is supposed to simulate an actual VM provider.
// That includes maintaining states for objects created through this
// provider, and query/get APIs reflecting those states. As a convenience
// to simulate scenarios, this provider also exposes a few function variables
// to override certain behaviors. The functionality of this provider is
// expected to evolve as more tests get added in the future.

type funcs struct {
	CreateOrUpdateVirtualMachineFn      func(ctx context.Context, vm *vmopv1.VirtualMachine) error
	CreateOrUpdateVirtualMachineAsyncFn func(ctx context.Context, vm *vmopv1.VirtualMachine) (<-chan error, error)
	DeleteVirtualMachineFn              func(ctx context.Context, vm *vmopv1.VirtualMachine) error
	PublishVirtualMachineFn             func(ctx context.Context, vm *vmopv1.VirtualMachine,
		vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error)
	GetVirtualMachineGuestHeartbeatFn  func(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
	GetVirtualMachinePropertiesFn      func(ctx context.Context, vm *vmopv1.VirtualMachine, propertyPaths []string) (map[string]any, error)
	GetVirtualMachineWebMKSTicketFn    func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error)
	GetVirtualMachineHardwareVersionFn func(ctx context.Context, vm *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error)
	PlaceVirtualMachineGroupFn         func(ctx context.Context, group *vmopv1.VirtualMachineGroup, groupPlacement []providers.VMGroupPlacement) error

	GetItemFromLibraryByNameFn func(ctx context.Context, contentLibrary, itemName string) (*library.Item, error)
	UpdateContentLibraryItemFn func(ctx context.Context, itemID, newName string, newDescription *string) error
	SyncVirtualMachineImageFn  func(ctx context.Context, cli, vmi client.Object) error

	UpdateVcPNIDFn           func(ctx context.Context, vcPNID, vcPort string) error
	UpdateVcCredsFn          func(ctx context.Context, data map[string][]byte) error
	ComputeCPUMinFrequencyFn func(ctx context.Context) error

	CreateOrUpdateVirtualMachineSetResourcePolicyFn func(ctx context.Context, rp *vmopv1.VirtualMachineSetResourcePolicy) error
	DeleteVirtualMachineSetResourcePolicyFn         func(ctx context.Context, rp *vmopv1.VirtualMachineSetResourcePolicy) error

	GetTasksByActIDFn func(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error)

	DoesProfileSupportEncryptionFn func(ctx context.Context, profileID string) (bool, error)
	VSphereClientFn                func(context.Context) (*vsclient.Client, error)
	DeleteSnapshotFn               func(ctx context.Context, vmSnapshot *vmopv1.VirtualMachineSnapshot, vm *vmopv1.VirtualMachine, removeChildren bool, consolidate *bool) (bool, error)
	GetParentSnapshotFn            func(ctx context.Context, vmSnapshotName string, vm *vmopv1.VirtualMachine) (*vimtypes.VirtualMachineSnapshotTree, error)
	GetSnapshotSizeFn              func(ctx context.Context, vmSnapshotName string, vm *vmopv1.VirtualMachine) (int64, error)
}

type VMProvider struct {
	sync.Mutex
	funcs
	vmMap    map[client.ObjectKey]*vmopv1.VirtualMachine
	vmPubMap map[string]vimtypes.TaskInfoState

	isPublishVMCalled bool
}

var _ providers.VirtualMachineProviderInterface = &VMProvider{}

func (s *VMProvider) Reset() {
	s.Lock()
	defer s.Unlock()

	s.funcs = funcs{}
	s.vmMap = make(map[client.ObjectKey]*vmopv1.VirtualMachine)
	s.vmPubMap = make(map[string]vimtypes.TaskInfoState)
	s.isPublishVMCalled = false
}

func (s *VMProvider) CreateOrUpdateVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.CreateOrUpdateVirtualMachineFn != nil {
		return s.CreateOrUpdateVirtualMachineFn(ctx, vm)
	}
	s.addToVMMap(vm)
	return nil
}

func (s *VMProvider) CreateOrUpdateVirtualMachineAsync(ctx context.Context, vm *vmopv1.VirtualMachine) (<-chan error, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.CreateOrUpdateVirtualMachineAsyncFn != nil {
		return s.CreateOrUpdateVirtualMachineAsyncFn(ctx, vm)
	}
	s.addToVMMap(vm)
	return nil, nil
}

func (s *VMProvider) DeleteVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.DeleteVirtualMachineFn != nil {
		return s.DeleteVirtualMachineFn(ctx, vm)
	}
	s.deleteFromVMMap(vm)
	return nil
}

func (s *VMProvider) PublishVirtualMachine(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	vmPub *vmopv1.VirtualMachinePublishRequest,
	cl *imgregv1a1.ContentLibrary, actID string) (string, error) {

	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	s.isPublishVMCalled = true
	if s.PublishVirtualMachineFn != nil {
		return s.PublishVirtualMachineFn(ctx, vm, vmPub, cl, actID)
	}
	s.AddToVMPublishMap(actID, vimtypes.TaskInfoStateSuccess)
	return "dummy-id", nil
}

func (s *VMProvider) GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineGuestHeartbeatFn != nil {
		return s.GetVirtualMachineGuestHeartbeatFn(ctx, vm)
	}
	return "", nil
}

func (s *VMProvider) GetVirtualMachineProperties(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	propertyPaths []string) (map[string]any, error) {

	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachinePropertiesFn != nil {
		return s.GetVirtualMachinePropertiesFn(ctx, vm, propertyPaths)
	}
	return nil, nil
}

func (s *VMProvider) GetVirtualMachineWebMKSTicket(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineWebMKSTicketFn != nil {
		return s.GetVirtualMachineWebMKSTicketFn(ctx, vm, pubKey)
	}
	return "", nil
}

func (s *VMProvider) GetVirtualMachineHardwareVersion(ctx context.Context, vm *vmopv1.VirtualMachine) (vimtypes.HardwareVersion, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineHardwareVersionFn != nil {
		return s.GetVirtualMachineHardwareVersionFn(ctx, vm)
	}
	return vimtypes.VMX15, nil
}

func (s *VMProvider) PlaceVirtualMachineGroup(ctx context.Context, group *vmopv1.VirtualMachineGroup, groupPlacements []providers.VMGroupPlacement) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.PlaceVirtualMachineGroupFn != nil {
		return s.PlaceVirtualMachineGroupFn(ctx, group, groupPlacements)
	}
	return nil
}

func (s *VMProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.CreateOrUpdateVirtualMachineSetResourcePolicyFn != nil {
		return s.CreateOrUpdateVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	return nil
}

func (s *VMProvider) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.DeleteVirtualMachineSetResourcePolicyFn != nil {
		return s.DeleteVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	return nil
}

func (s *VMProvider) ComputeCPUMinFrequency(ctx context.Context) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.ComputeCPUMinFrequencyFn != nil {
		return s.ComputeCPUMinFrequencyFn(ctx)
	}

	return nil
}

func (s *VMProvider) UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.UpdateVcPNIDFn != nil {
		return s.UpdateVcPNIDFn(ctx, vcPNID, vcPort)
	}
	return nil
}

func (s *VMProvider) UpdateVcCreds(ctx context.Context, data map[string][]byte) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.UpdateVcCredsFn != nil {
		return s.UpdateVcCredsFn(ctx, data)
	}
	return nil
}

func (s *VMProvider) SyncVirtualMachineImage(ctx context.Context, cli, vmi client.Object) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.SyncVirtualMachineImageFn != nil {
		return s.SyncVirtualMachineImageFn(ctx, cli, vmi)
	}

	return nil
}

func (s *VMProvider) GetItemFromLibraryByName(ctx context.Context,
	contentLibrary, itemName string) (*library.Item, error) {

	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetItemFromLibraryByNameFn != nil {
		return s.GetItemFromLibraryByNameFn(ctx, contentLibrary, itemName)
	}

	return nil, nil
}

func (s *VMProvider) UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.UpdateContentLibraryItemFn != nil {
		return s.UpdateContentLibraryItemFn(ctx, itemID, newName, newDescription)
	}
	return nil
}

func (s *VMProvider) GetTasksByActID(ctx context.Context, actID string) (tasksInfo []vimtypes.TaskInfo, retErr error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetTasksByActIDFn != nil {
		return s.GetTasksByActIDFn(ctx, actID)
	}

	status := s.vmPubMap[actID]
	if status == "" {
		return nil, nil
	}

	task1 := vimtypes.TaskInfo{
		DescriptionId: "com.vmware.ovfs.LibraryItem.capture",
		ActivationId:  actID,
		State:         status,
	}

	return []vimtypes.TaskInfo{task1}, nil
}

func (s *VMProvider) addToVMMap(vm *vmopv1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	s.vmMap[objectKey] = vm
}

func (s *VMProvider) deleteFromVMMap(vm *vmopv1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	delete(s.vmMap, objectKey)
}

func (s *VMProvider) AddToVMPublishMap(actID string, result vimtypes.TaskInfoState) {
	s.vmPubMap[actID] = result
}

func (s *VMProvider) GetVMPublishRequestResult(vmPub *vmopv1.VirtualMachinePublishRequest) vimtypes.TaskInfoState {
	s.Lock()
	defer s.Unlock()

	actID := fmt.Sprintf("%s-%d", vmPub.UID, vmPub.Status.Attempts)
	return s.vmPubMap[actID]
}

func (s *VMProvider) GetVMPublishRequestResultWithActIDLocked(actID string) vimtypes.TaskInfoState {
	return s.vmPubMap[actID]
}

func (s *VMProvider) IsPublishVMCalled() bool {
	s.Lock()
	defer s.Unlock()

	return s.isPublishVMCalled
}

func (s *VMProvider) DoesProfileSupportEncryption(
	ctx context.Context,
	profileID string) (bool, error) {

	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if fn := s.DoesProfileSupportEncryptionFn; fn != nil {
		return fn(ctx, profileID)
	}
	return false, nil
}

func (s *VMProvider) VSphereClient(ctx context.Context) (*vsclient.Client, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if fn := s.VSphereClientFn; fn != nil {
		return fn(ctx)
	}
	return nil, nil
}

func (s *VMProvider) DeleteSnapshot(
	ctx context.Context,
	snapshot *vmopv1.VirtualMachineSnapshot,
	vm *vmopv1.VirtualMachine,
	removeChildren bool,
	consolidate *bool) (bool, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.DeleteSnapshotFn != nil {
		return s.DeleteSnapshotFn(ctx, snapshot, vm, removeChildren, consolidate)
	}
	return false, nil
}

func (s *VMProvider) GetParentSnapshot(ctx context.Context, vmSnapshotName string, vm *vmopv1.VirtualMachine) (*vimtypes.VirtualMachineSnapshotTree, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetParentSnapshotFn != nil {
		return s.GetParentSnapshotFn(ctx, vmSnapshotName, vm)
	}
	return nil, nil
}

func (s *VMProvider) GetSnapshotSize(ctx context.Context, vmSnapshotName string, vm *vmopv1.VirtualMachine) (int64, error) {
	_ = pkgcfg.FromContext(ctx)

	s.Lock()
	defer s.Unlock()
	if s.GetSnapshotSizeFn != nil {
		return s.GetSnapshotSizeFn(ctx, vmSnapshotName, vm)
	}
	return 0, nil
}

func NewVMProvider() *VMProvider {
	provider := VMProvider{
		vmMap:    map[client.ObjectKey]*vmopv1.VirtualMachine{},
		vmPubMap: map[string]vimtypes.TaskInfoState{},
	}
	return &provider
}

func SetCreateOrUpdateFunction(
	ctx context.Context,
	provider *VMProvider,
	fn func(
		ctx context.Context,
		vm *vmopv1.VirtualMachine,
	) error,
) {
	provider.Lock()
	defer provider.Unlock()

	if pkgcfg.FromContext(ctx).AsyncSignalEnabled {

		provider.CreateOrUpdateVirtualMachineAsyncFn = func(
			ctx context.Context,
			vm *vmopv1.VirtualMachine) (<-chan error, error) {

			chanErr := make(chan error)
			close(chanErr)

			return chanErr, fn(ctx, vm)
		}

		return

	}

	provider.CreateOrUpdateVirtualMachineFn = fn
}
