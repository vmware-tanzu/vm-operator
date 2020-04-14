/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

type VirtualMachineMetadata map[string]string

type VmConfigArgs struct {
	VmClass          v1alpha1.VirtualMachineClass
	ResourcePolicy   *v1alpha1.VirtualMachineSetResourcePolicy
	VmMetadata       VirtualMachineMetadata
	StorageProfileID string
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
	DeleteNamespaceSessionInCache(ctx context.Context, namespace string)
}
