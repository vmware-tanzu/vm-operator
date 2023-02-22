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

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"

	imgregv1a1 "github.com/vmware-tanzu/vm-operator/external/image-registry/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

// This Fake Provider is supposed to simulate an actual VM provider.
// That includes maintaining states for objects created through this
// provider, and query/get APIs reflecting those states. As a convenience
// to simulate scenarios, this provider also exposes a few function variables
// to override certain behaviors. The functionality of this provider is
// expected to evolve as more tests get added in the future.

type funcs struct {
	CreateOrUpdateVirtualMachineFn func(ctx context.Context, vm *v1alpha1.VirtualMachine) error
	DeleteVirtualMachineFn         func(ctx context.Context, vm *v1alpha1.VirtualMachine) error
	PublishVirtualMachineFn        func(ctx context.Context, vm *v1alpha1.VirtualMachine,
		vmPub *v1alpha1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error)
	GetVirtualMachineGuestHeartbeatFn func(ctx context.Context, vm *v1alpha1.VirtualMachine) (v1alpha1.GuestHeartbeatStatus, error)
	GetVirtualMachineWebMKSTicketFn   func(ctx context.Context, vm *v1alpha1.VirtualMachine, pubKey string) (string, error)

	ListItemsFromContentLibraryFn              func(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider) ([]string, error)
	GetVirtualMachineImageFromContentLibraryFn func(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider, itemID string,
		currentCLImages map[string]v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error)
	SyncVirtualMachineImageFn  func(ctx context.Context, itemID string, vmi client.Object) error
	GetItemFromLibraryByNameFn func(ctx context.Context, contentLibrary, itemName string) (*library.Item, error)
	UpdateContentLibraryItemFn func(ctx context.Context, itemID, newName string, newDescription *string) error

	UpdateVcPNIDFn  func(ctx context.Context, vcPNID, vcPort string) error
	ResetVcClientFn func(ctx context.Context)

	CreateOrUpdateVirtualMachineSetResourcePolicyFn func(ctx context.Context, rp *v1alpha1.VirtualMachineSetResourcePolicy) error
	IsVirtualMachineSetResourcePolicyReadyFn        func(ctx context.Context, azName string, rp *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicyFn         func(ctx context.Context, rp *v1alpha1.VirtualMachineSetResourcePolicy) error
	ComputeCPUMinFrequencyFn                        func(ctx context.Context) error

	GetTasksByActIDFn func(ctx context.Context, actID string) (tasksInfo []vimTypes.TaskInfo, retErr error)
}

type VMProvider struct {
	sync.Mutex
	funcs
	vmMap             map[client.ObjectKey]*v1alpha1.VirtualMachine
	resourcePolicyMap map[client.ObjectKey]*v1alpha1.VirtualMachineSetResourcePolicy
	vmPubMap          map[string]vimTypes.TaskInfoState

	isPublishVMCalled bool
}

var _ vmprovider.VirtualMachineProviderInterface = &VMProvider{}

func (s *VMProvider) Reset() {
	s.Lock()
	defer s.Unlock()

	s.funcs = funcs{}
	s.vmMap = make(map[client.ObjectKey]*v1alpha1.VirtualMachine)
	s.resourcePolicyMap = make(map[client.ObjectKey]*v1alpha1.VirtualMachineSetResourcePolicy)
	s.vmPubMap = make(map[string]vimTypes.TaskInfoState)
	s.isPublishVMCalled = false
}

func (s *VMProvider) CreateOrUpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	s.Lock()
	defer s.Unlock()
	if s.CreateOrUpdateVirtualMachineFn != nil {
		return s.CreateOrUpdateVirtualMachineFn(ctx, vm)
	}
	s.addToVMMap(vm)
	vm.Status.Phase = v1alpha1.Created
	return nil
}

func (s *VMProvider) DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	s.Lock()
	defer s.Unlock()
	if s.DeleteVirtualMachineFn != nil {
		return s.DeleteVirtualMachineFn(ctx, vm)
	}
	s.deleteFromVMMap(vm)
	return nil
}

func (s *VMProvider) PublishVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
	vmPub *v1alpha1.VirtualMachinePublishRequest, cl *imgregv1a1.ContentLibrary, actID string) (string, error) {
	s.Lock()
	defer s.Unlock()

	s.isPublishVMCalled = true

	if s.PublishVirtualMachineFn != nil {
		return s.PublishVirtualMachineFn(ctx, vm, vmPub, cl, actID)
	}

	s.AddToVMPublishMap(actID, vimTypes.TaskInfoStateSuccess)
	return "dummy-id", nil
}

func (s *VMProvider) GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *v1alpha1.VirtualMachine) (v1alpha1.GuestHeartbeatStatus, error) {
	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineGuestHeartbeatFn != nil {
		return s.GetVirtualMachineGuestHeartbeatFn(ctx, vm)
	}
	return "", nil
}

func (s *VMProvider) GetVirtualMachineWebMKSTicket(ctx context.Context, vm *v1alpha1.VirtualMachine, pubKey string) (string, error) {
	s.Lock()
	defer s.Unlock()
	if s.GetVirtualMachineWebMKSTicketFn != nil {
		return s.GetVirtualMachineWebMKSTicketFn(ctx, vm, pubKey)
	}
	return "", nil
}

func (s *VMProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	s.Lock()
	defer s.Unlock()

	if s.CreateOrUpdateVirtualMachineSetResourcePolicyFn != nil {
		return s.CreateOrUpdateVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	s.addToResourcePolicyMap(resourcePolicy)

	return nil
}

func (s *VMProvider) IsVirtualMachineSetResourcePolicyReady(ctx context.Context, azName string, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
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

func (s *VMProvider) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	s.Lock()
	defer s.Unlock()

	if s.DeleteVirtualMachineSetResourcePolicyFn != nil {
		return s.DeleteVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	s.deleteFromResourcePolicyMap(resourcePolicy)

	return nil
}

func (s *VMProvider) ComputeCPUMinFrequency(ctx context.Context) error {
	s.Lock()
	defer s.Unlock()
	if s.ComputeCPUMinFrequencyFn != nil {
		return s.ComputeCPUMinFrequencyFn(ctx)
	}

	return nil
}

func (s *VMProvider) UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error {
	s.Lock()
	defer s.Unlock()
	if s.UpdateVcPNIDFn != nil {
		return s.UpdateVcPNIDFn(ctx, vcPNID, vcPort)
	}
	return nil
}

func (s *VMProvider) ResetVcClient(ctx context.Context) {
	s.Lock()
	defer s.Unlock()

	if s.ResetVcClientFn != nil {
		s.ResetVcClientFn(ctx)
	}
}

func (s *VMProvider) ListItemsFromContentLibrary(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider) ([]string, error) {
	s.Lock()
	defer s.Unlock()

	if s.ListItemsFromContentLibraryFn != nil {
		return s.ListItemsFromContentLibraryFn(ctx, contentLibrary)
	}

	// No-op for now.
	return []string{}, nil
}

func (s *VMProvider) GetVirtualMachineImageFromContentLibrary(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider, itemID string,
	currentCLImages map[string]v1alpha1.VirtualMachineImage) (*v1alpha1.VirtualMachineImage, error) {
	s.Lock()
	defer s.Unlock()

	if s.GetVirtualMachineImageFromContentLibraryFn != nil {
		return s.GetVirtualMachineImageFromContentLibraryFn(ctx, contentLibrary, itemID, currentCLImages)
	}

	// No-op for now.
	return nil, nil
}

func (s *VMProvider) SyncVirtualMachineImage(ctx context.Context, itemID string, vmi client.Object) error {
	s.Lock()
	defer s.Unlock()

	if s.SyncVirtualMachineImageFn != nil {
		return s.SyncVirtualMachineImageFn(ctx, itemID, vmi)
	}

	return nil
}

func (s *VMProvider) GetItemFromLibraryByName(ctx context.Context,
	contentLibrary, itemName string) (*library.Item, error) {
	s.Lock()
	defer s.Unlock()

	if s.GetItemFromLibraryByNameFn != nil {
		return s.GetItemFromLibraryByNameFn(ctx, contentLibrary, itemName)
	}

	return nil, nil
}

func (s *VMProvider) UpdateContentLibraryItem(ctx context.Context, itemID, newName string, newDescription *string) error {
	s.Lock()
	defer s.Unlock()

	if s.UpdateContentLibraryItemFn != nil {
		return s.UpdateContentLibraryItemFn(ctx, itemID, newName, newDescription)
	}
	return nil
}

func (s *VMProvider) GetTasksByActID(ctx context.Context, actID string) (tasksInfo []vimTypes.TaskInfo, retErr error) {
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
		Result:        "dummy-item",
	}

	return []vimTypes.TaskInfo{task1}, nil
}

func (s *VMProvider) addToVMMap(vm *v1alpha1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	s.vmMap[objectKey] = vm
}

func (s *VMProvider) deleteFromVMMap(vm *v1alpha1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	delete(s.vmMap, objectKey)
}

func (s *VMProvider) addToResourcePolicyMap(rp *v1alpha1.VirtualMachineSetResourcePolicy) {
	objectKey := client.ObjectKey{
		Namespace: rp.Namespace,
		Name:      rp.Name,
	}

	s.resourcePolicyMap[objectKey] = rp
}

func (s *VMProvider) deleteFromResourcePolicyMap(rp *v1alpha1.VirtualMachineSetResourcePolicy) {
	objectKey := client.ObjectKey{
		Namespace: rp.Namespace,
		Name:      rp.Name,
	}
	delete(s.resourcePolicyMap, objectKey)
}

func (s *VMProvider) AddToVMPublishMap(actID string, result vimTypes.TaskInfoState) {
	s.vmPubMap[actID] = result
}

func (s *VMProvider) GetVMPublishRequestResult(vmPub *v1alpha1.VirtualMachinePublishRequest) vimTypes.TaskInfoState {
	s.Lock()
	defer s.Unlock()

	actID := fmt.Sprintf("%s-%d", vmPub.UID, vmPub.Status.Attempts)
	return s.vmPubMap[actID]
}

func (s *VMProvider) GetVMPublishRequestResultWithActIDLocked(actID string) vimTypes.TaskInfoState {
	return s.vmPubMap[actID]
}

func (s *VMProvider) IsPublishVMCalled() bool {
	s.Lock()
	defer s.Unlock()

	return s.isPublishVMCalled
}

func NewVMProvider() *VMProvider {
	provider := VMProvider{
		vmMap:             map[client.ObjectKey]*v1alpha1.VirtualMachine{},
		resourcePolicyMap: map[client.ObjectKey]*v1alpha1.VirtualMachineSetResourcePolicy{},
		vmPubMap:          map[string]vimTypes.TaskInfoState{},
	}
	return &provider
}
