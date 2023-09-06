// Copyright (c) 2020-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"fmt"
	"sync"

	"github.com/vmware/govmomi/vapi/library"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

// This Fake Provider is supposed to simulate an actual VM provider.
// That includes maintaining states for objects created through this
// provider, and query/get APIs reflecting those states. As a convenience
// to simulate scenarios, this provider also exposes a few function variables
// to override certain behaviors. The functionality of this provider is
// expected to evolve as more tests get added in the future.

type funcsA2 struct {
	CreateOrUpdateVirtualMachineFn func(ctx context.Context, vm *vmopv1.VirtualMachine) error
	DeleteVirtualMachineFn         func(ctx context.Context, vm *vmopv1.VirtualMachine) error
	PublishVirtualMachineFn        func(ctx context.Context, vm *vmopv1.VirtualMachine,
		vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error)
	BackupVirtualMachineFn             func(ctx context.Context, vm *vmopv1.VirtualMachine) error
	GetVirtualMachineGuestHeartbeatFn  func(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
	GetVirtualMachineWebMKSTicketFn    func(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error)
	GetVirtualMachineHardwareVersionFn func(ctx context.Context, vm *vmopv1.VirtualMachine) (int32, error)

	// ListItemsFromContentLibraryFn              func(ctx context.Context, contentLibrary *vmopv1.ContentLibraryProvider) ([]string, error)
	// GetVirtualMachineImageFromContentLibraryFn func(ctx context.Context, contentLibrary *vmopv1.ContentLibraryProvider, itemID string,
	//	currentCLImages map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error)

	GetItemFromLibraryByNameFn func(ctx context.Context, contentLibrary, itemName string) (*library.Item, error)
	UpdateContentLibraryItemFn func(ctx context.Context, itemID, newName string, newDescription *string) error
	SyncVirtualMachineImageFn  func(ctx context.Context, cli, vmi client.Object) error

	UpdateVcPNIDFn  func(ctx context.Context, vcPNID, vcPort string) error
	ResetVcClientFn func(ctx context.Context)

	CreateOrUpdateVirtualMachineSetResourcePolicyFn func(ctx context.Context, rp *vmopv1.VirtualMachineSetResourcePolicy) error
	IsVirtualMachineSetResourcePolicyReadyFn        func(ctx context.Context, azName string, rp *vmopv1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicyFn         func(ctx context.Context, rp *vmopv1.VirtualMachineSetResourcePolicy) error
	ComputeCPUMinFrequencyFn                        func(ctx context.Context) error

	GetTasksByActIDFn func(ctx context.Context, actID string) (tasksInfo []vimTypes.TaskInfo, retErr error)
}

type VMProviderA2 struct {
	sync.Mutex
	funcsA2
	vmMap             map[client.ObjectKey]*vmopv1.VirtualMachine
	resourcePolicyMap map[client.ObjectKey]*vmopv1.VirtualMachineSetResourcePolicy
	vmPubMap          map[string]vimTypes.TaskInfoState

	isPublishVMCalled bool
}

var _ vmprovider.VirtualMachineProviderInterfaceA2 = &VMProviderA2{}

func (s *VMProviderA2) Reset() {
	s.Lock()
	defer s.Unlock()

	s.funcsA2 = funcsA2{}
	s.vmMap = make(map[client.ObjectKey]*vmopv1.VirtualMachine)
	s.resourcePolicyMap = make(map[client.ObjectKey]*vmopv1.VirtualMachineSetResourcePolicy)
	s.vmPubMap = make(map[string]vimTypes.TaskInfoState)
	s.isPublishVMCalled = false
}

func (s *VMProviderA2) CreateOrUpdateVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	s.Lock()
	defer s.Unlock()
	if s.CreateOrUpdateVirtualMachineFn != nil {
		return s.CreateOrUpdateVirtualMachineFn(ctx, vm)
	}
	s.addToVMMap(vm)
	return nil
}

func (s *VMProviderA2) DeleteVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	s.Lock()
	defer s.Unlock()
	if s.DeleteVirtualMachineFn != nil {
		return s.DeleteVirtualMachineFn(ctx, vm)
	}
	s.deleteFromVMMap(vm)
	return nil
}

func (s *VMProviderA2) PublishVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine,
	vmPub *vmopv1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
	s.Lock()
	defer s.Unlock()

	s.isPublishVMCalled = true

	if s.PublishVirtualMachineFn != nil {
		return s.PublishVirtualMachineFn(ctx, vm, vmPub, cl, actID)
	}

	s.AddToVMPublishMap(actID, vimTypes.TaskInfoStateSuccess)
	return "dummy-id", nil
}

func (s *VMProviderA2) BackupVirtualMachine(ctx context.Context, vm *vmopv1.VirtualMachine) error {
	s.Lock()
	defer s.Unlock()

	if s.BackupVirtualMachineFn != nil {
		return s.BackupVirtualMachineFn(ctx, vm)
	}

	return nil
}

func (s *VMProviderA2) GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error) {
	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineGuestHeartbeatFn != nil {
		return s.GetVirtualMachineGuestHeartbeatFn(ctx, vm)
	}
	return "", nil
}

func (s *VMProviderA2) GetVirtualMachineWebMKSTicket(ctx context.Context, vm *vmopv1.VirtualMachine, pubKey string) (string, error) {
	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineWebMKSTicketFn != nil {
		return s.GetVirtualMachineWebMKSTicketFn(ctx, vm, pubKey)
	}
	return "", nil
}

func (s *VMProviderA2) GetVirtualMachineHardwareVersion(ctx context.Context, vm *vmopv1.VirtualMachine) (int32, error) {
	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineHardwareVersionFn != nil {
		return s.GetVirtualMachineHardwareVersionFn(ctx, vm)
	}
	return 15, nil
}

func (s *VMProviderA2) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {
	s.Lock()
	defer s.Unlock()

	if s.CreateOrUpdateVirtualMachineSetResourcePolicyFn != nil {
		return s.CreateOrUpdateVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	s.addToResourcePolicyMap(resourcePolicy)

	return nil
}

func (s *VMProviderA2) IsVirtualMachineSetResourcePolicyReady(ctx context.Context, azName string, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) (bool, error) {
	s.Lock()
	defer s.Unlock()

	if s.IsVirtualMachineSetResourcePolicyReadyFn != nil {
		return s.IsVirtualMachineSetResourcePolicyReadyFn(ctx, azName, resourcePolicy)
	}
	objectKey := client.ObjectKey{
		Namespace: resourcePolicy.Namespace,
		Name:      resourcePolicy.Name,
	}
	_, found := s.resourcePolicyMap[objectKey]

	return found, nil
}

func (s *VMProviderA2) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {
	s.Lock()
	defer s.Unlock()

	if s.DeleteVirtualMachineSetResourcePolicyFn != nil {
		return s.DeleteVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	s.deleteFromResourcePolicyMap(resourcePolicy)

	return nil
}

func (s *VMProviderA2) ComputeCPUMinFrequency(ctx context.Context) error {
	s.Lock()
	defer s.Unlock()
	if s.ComputeCPUMinFrequencyFn != nil {
		return s.ComputeCPUMinFrequencyFn(ctx)
	}

	return nil
}

func (s *VMProviderA2) UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error {
	s.Lock()
	defer s.Unlock()
	if s.UpdateVcPNIDFn != nil {
		return s.UpdateVcPNIDFn(ctx, vcPNID, vcPort)
	}
	return nil
}

func (s *VMProviderA2) ResetVcClient(ctx context.Context) {
	s.Lock()
	defer s.Unlock()

	if s.ResetVcClientFn != nil {
		s.ResetVcClientFn(ctx)
	}
}

/*
func (s *VMProviderA2) ListItemsFromContentLibrary(ctx context.Context, contentLibrary *vmopv1.ContentLibraryProvider) ([]string, error) {
	s.Lock()
	defer s.Unlock()

	if s.ListItemsFromContentLibraryFn != nil {
		return s.ListItemsFromContentLibraryFn(ctx, contentLibrary)
	}

	// No-op for now.
	return []string{}, nil
}

func (s *VMProviderA2) GetVirtualMachineImageFromContentLibrary(ctx context.Context, contentLibrary *vmopv1.ContentLibraryProvider, itemID string,
	currentCLImages map[string]vmopv1.VirtualMachineImage) (*vmopv1.VirtualMachineImage, error) {
	s.Lock()
	defer s.Unlock()

	if s.GetVirtualMachineImageFromContentLibraryFn != nil {
		return s.GetVirtualMachineImageFromContentLibraryFn(ctx, contentLibrary, itemID, currentCLImages)
	}

	// No-op for now.
	return nil, nil
}
*/

func (s *VMProviderA2) SyncVirtualMachineImage(ctx context.Context, cli, vmi client.Object) error {
	s.Lock()
	defer s.Unlock()

	if s.SyncVirtualMachineImageFn != nil {
		return s.SyncVirtualMachineImageFn(ctx, cli, vmi)
	}

	return nil
}

func (s *VMProviderA2) GetItemFromLibraryByName(ctx context.Context,
	contentLibrary, itemName string) (*library.Item, error) {
	s.Lock()
	defer s.Unlock()

	if s.GetItemFromLibraryByNameFn != nil {
		return s.GetItemFromLibraryByNameFn(ctx, contentLibrary, itemName)
	}

	return nil, nil
}

func (s *VMProviderA2) UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error {
	s.Lock()
	defer s.Unlock()

	if s.UpdateContentLibraryItemFn != nil {
		return s.UpdateContentLibraryItemFn(ctx, itemID, newName, newDescription)
	}
	return nil
}

func (s *VMProviderA2) GetTasksByActID(ctx context.Context, actID string) (tasksInfo []vimTypes.TaskInfo, retErr error) {
	s.Lock()
	defer s.Unlock()

	if s.GetTasksByActIDFn != nil {
		return s.GetTasksByActIDFn(ctx, actID)
	}

	status := s.vmPubMap[actID]
	if status == "" {
		return nil, nil
	}

	task1 := vimTypes.TaskInfo{
		DescriptionId: "com.vmware.ovfs.LibraryItem.capture",
		ActivationId:  actID,
		State:         status,
	}

	return []vimTypes.TaskInfo{task1}, nil
}

func (s *VMProviderA2) addToVMMap(vm *vmopv1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	s.vmMap[objectKey] = vm
}

func (s *VMProviderA2) deleteFromVMMap(vm *vmopv1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	delete(s.vmMap, objectKey)
}

func (s *VMProviderA2) addToResourcePolicyMap(rp *vmopv1.VirtualMachineSetResourcePolicy) {
	objectKey := client.ObjectKey{
		Namespace: rp.Namespace,
		Name:      rp.Name,
	}

	s.resourcePolicyMap[objectKey] = rp
}

func (s *VMProviderA2) deleteFromResourcePolicyMap(rp *vmopv1.VirtualMachineSetResourcePolicy) {
	objectKey := client.ObjectKey{
		Namespace: rp.Namespace,
		Name:      rp.Name,
	}
	delete(s.resourcePolicyMap, objectKey)
}

func (s *VMProviderA2) AddToVMPublishMap(actID string, result vimTypes.TaskInfoState) {
	s.vmPubMap[actID] = result
}

func (s *VMProviderA2) GetVMPublishRequestResult(vmPub *vmopv1.VirtualMachinePublishRequest) vimTypes.TaskInfoState {
	s.Lock()
	defer s.Unlock()

	actID := fmt.Sprintf("%s-%d", vmPub.UID, vmPub.Status.Attempts)
	return s.vmPubMap[actID]
}

func (s *VMProviderA2) GetVMPublishRequestResultWithActIDLocked(actID string) vimTypes.TaskInfoState {
	return s.vmPubMap[actID]
}

func (s *VMProviderA2) IsPublishVMCalled() bool {
	s.Lock()
	defer s.Unlock()

	return s.isPublishVMCalled
}

func NewVMProviderA2() *VMProviderA2 {
	provider := VMProviderA2{
		vmMap:             map[client.ObjectKey]*vmopv1.VirtualMachine{},
		resourcePolicyMap: map[client.ObjectKey]*vmopv1.VirtualMachineSetResourcePolicy{},
		vmPubMap:          map[string]vimTypes.TaskInfoState{},
	}
	return &provider
}
