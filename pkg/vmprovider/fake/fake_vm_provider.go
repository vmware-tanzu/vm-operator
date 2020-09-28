// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
)

// This Fake Provider is supposed to simulate an actual VM provider.
// That includes maintaining states for objects created through this
// provider, and query/get APIs reflecting those states. As a convenience
// to simulate scenarios, this provider also exposes a few function variables
// to override certain behaviors. The functionality of this provider is
// expected to evolve as more tests get added in the future.
type FakeVmProvider struct {
	sync.Mutex
	vmMap                     map[client.ObjectKey]*v1alpha1.VirtualMachine
	DoesVirtualMachineExistFn func(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error)
	CreateVirtualMachineFn    func(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error
	UpdateVirtualMachineFn    func(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error
	DeleteVirtualMachineFn    func(ctx context.Context, vm *v1alpha1.VirtualMachine) error

	resourcePolicyMap                          map[client.ObjectKey]*v1alpha1.VirtualMachineSetResourcePolicy
	DoesVirtualMachineSetResourcePolicyExistFn func(ctx context.Context, rp *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicyFn    func(ctx context.Context, rp *v1alpha1.VirtualMachineSetResourcePolicy) error
}

func (s *FakeVmProvider) DoesVirtualMachineExist(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error) {
	s.Lock()
	defer s.Unlock()
	if s.DoesVirtualMachineExistFn != nil {
		return s.DoesVirtualMachineExistFn(ctx, vm)
	}
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	_, ok := s.vmMap[objectKey]
	return ok, nil
}

func (s *FakeVmProvider) CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	s.Lock()
	defer s.Unlock()
	if s.CreateVirtualMachineFn != nil {
		return s.CreateVirtualMachineFn(ctx, vm, vmConfigArgs)
	}
	s.addToMap(vm)
	vm.Status.Phase = v1alpha1.Created
	return nil
}

func (s *FakeVmProvider) UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	s.Lock()
	defer s.Unlock()
	if s.UpdateVirtualMachineFn != nil {
		return s.UpdateVirtualMachineFn(ctx, vm, vmConfigArgs)
	}
	s.addToMap(vm)
	return nil
}

func (s *FakeVmProvider) DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error {
	s.Lock()
	defer s.Unlock()
	if s.DeleteVirtualMachineFn != nil {
		return s.DeleteVirtualMachineFn(ctx, vm)
	}
	s.deleteFromVMMap(vm)
	return nil
}

func (s *FakeVmProvider) Initialize(stop <-chan struct{}) {}

func (s *FakeVmProvider) Name() string {
	return "fake"
}

func (s *FakeVmProvider) GetClusterID(ctx context.Context, namespace string) (string, error) {
	return "domain-c8", nil
}

func (s *FakeVmProvider) CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	return nil
}

func (s *FakeVmProvider) DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error) {
	s.Lock()
	defer s.Unlock()
	if s.DoesVirtualMachineSetResourcePolicyExistFn != nil {
		return s.DoesVirtualMachineSetResourcePolicyExistFn(ctx, resourcePolicy)
	}
	objectKey := client.ObjectKey{
		Namespace: resourcePolicy.Namespace,
		Name:      resourcePolicy.Name,
	}
	_, found := s.resourcePolicyMap[objectKey]

	return found, nil
}

func (s *FakeVmProvider) DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {
	s.Lock()
	defer s.Unlock()
	if s.DeleteVirtualMachineSetResourcePolicyFn != nil {
		return s.DeleteVirtualMachineSetResourcePolicyFn(ctx, resourcePolicy)
	}
	s.deleteFromResourcePolicyMap(resourcePolicy)

	return nil
}

func (s *FakeVmProvider) ComputeClusterCpuMinFrequency(ctx context.Context) error {
	return nil
}

func (s *FakeVmProvider) UpdateVcPNID(ctx context.Context, clusterConfigMap *corev1.ConfigMap) error {
	return nil
}

func (s *FakeVmProvider) UpdateVmOpSACredSecret(ctx context.Context) {}

func (s *FakeVmProvider) UpdateVmOpConfigMap(ctx context.Context) {}

func (s *FakeVmProvider) DeleteNamespaceSessionInCache(ctx context.Context, namespace string) {}

func (s *FakeVmProvider) DoesContentLibraryExist(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider) (bool, error) {
	return true, nil
}

func (s *FakeVmProvider) ListVirtualMachineImagesFromContentLibrary(ctx context.Context, cl v1alpha1.ContentLibraryProvider) ([]*v1alpha1.VirtualMachineImage, error) {
	return []*v1alpha1.VirtualMachineImage{}, nil
}

func (s *FakeVmProvider) ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error) {
	return []*v1alpha1.VirtualMachineImage{}, nil
}

func (s *FakeVmProvider) GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error) {
	return nil, nil
}

func (s *FakeVmProvider) addToMap(vm *v1alpha1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	s.vmMap[objectKey] = vm
}

func (s *FakeVmProvider) deleteFromVMMap(vm *v1alpha1.VirtualMachine) {
	objectKey := client.ObjectKey{
		Namespace: vm.Namespace,
		Name:      vm.Name,
	}
	delete(s.vmMap, objectKey)
}

func (s *FakeVmProvider) deleteFromResourcePolicyMap(rp *v1alpha1.VirtualMachineSetResourcePolicy) {
	objectKey := client.ObjectKey{
		Namespace: rp.Namespace,
		Name:      rp.Name,
	}
	delete(s.resourcePolicyMap, objectKey)
}

func NewFakeVmProvider() *FakeVmProvider {
	provider := FakeVmProvider{
		vmMap: map[client.ObjectKey]*v1alpha1.VirtualMachine{},
	}
	return &provider
}
