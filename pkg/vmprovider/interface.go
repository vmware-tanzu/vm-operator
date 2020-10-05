// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmprovider

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

type VmMetadata struct {
	Data      map[string]string
	Transport v1alpha1.VirtualMachineMetadataTransport
}

type VmConfigArgs struct {
	VmClass          v1alpha1.VirtualMachineClass
	ResourcePolicy   *v1alpha1.VirtualMachineSetResourcePolicy
	VmMetadata       *VmMetadata
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
	InvokeFsrVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	// Used by VirtualMachine controller to determine if entities of ResourcePolicy exist on the infrastructure provider
	DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	ComputeClusterCpuMinFrequency(ctx context.Context) error
	UpdateVcPNID(ctx context.Context, clusterConfigMap *corev1.ConfigMap) error
	UpdateVmOpSACredSecret(ctx context.Context)
	UpdateVmOpConfigMap(ctx context.Context)
	DeleteNamespaceSessionInCache(ctx context.Context, namespace string)

	// AKP move to content provider interface
	DoesContentLibraryExist(ctx context.Context, contentLibrary *v1alpha1.ContentLibraryProvider) (bool, error)
	ListVirtualMachineImagesFromContentLibrary(ctx context.Context, cl v1alpha1.ContentLibraryProvider) ([]*v1alpha1.VirtualMachineImage, error)
}
