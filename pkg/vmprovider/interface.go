/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"context"
	"sync"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

type VirtualMachineMetadata map[string]string

type VmConfigArgs struct {
	VmClass          v1alpha1.VirtualMachineClass
	ResourcePolicy   *v1alpha1.VirtualMachineSetResourcePolicy
	VmMetadata       VirtualMachineMetadata
	StorageProfileID string
}

type VMProviderService interface {
	RegisterVmProvider(vmProvider VirtualMachineProviderInterface)
	GetRegisteredVmProviderOrDie() VirtualMachineProviderInterface
}

type vmProviderService struct {
	registeredVmProvider VirtualMachineProviderInterface
	mutex                sync.Mutex
}

var once sync.Once
var providerServiceVar *vmProviderService

// GetService creates a VMProviderService
func GetService() VMProviderService {
	once.Do(func() {
		providerServiceVar = &vmProviderService{}
	})
	return providerServiceVar
}

// RegisterVmProvider registers a VMProvider directly, replaces the existing
func (vs *vmProviderService) RegisterVmProvider(vmProvider VirtualMachineProviderInterface) {
	vs.mutex.Lock()
	defer vs.mutex.Unlock()

	vs.registeredVmProvider = vmProvider
}

// GetRegisteredVmProviderOrDie returns the registered VM Provider
func (vs *vmProviderService) GetRegisteredVmProviderOrDie() VirtualMachineProviderInterface {
	vs.mutex.Lock()
	defer vs.mutex.Unlock()

	if vs.registeredVmProvider == nil {
		panic("No VM provider registered")
	}

	return vs.registeredVmProvider
}

// VirtualMachineProviderInterface is a plugable interface for VM Providers
type VirtualMachineProviderInterface interface {
	Name() string

	// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
	// to perform housekeeping or run custom controllers specific to the cloud provider.
	// Any tasks started here should be cleaned up when the stop channel closes.
	Initialize(stop <-chan struct{})

	ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error)
	GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error)

	DoesVirtualMachineExist(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error)
	CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VmConfigArgs) error
	UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VmConfigArgs) error
	DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error

	GetClusterID(ctx context.Context, namespace string) (string, error)

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	// Used by VirtualMachine controller to determine if entities of ResourcePolicy exist on the infrastructure provider
	DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	ComputeClusterCpuMinFrequency(ctx context.Context) error
	UpdateVcPNID(ctx context.Context, clusterConfigMap *corev1.ConfigMap) error
	UpdateVmOpSACredSecret(ctx context.Context)
	UpdateVmOpConfigMap(ctx context.Context)
}
