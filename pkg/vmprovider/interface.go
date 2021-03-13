// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmprovider

import (
	"context"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

type VmMetadata struct {
	Data      map[string]string
	Transport v1alpha1.VirtualMachineMetadataTransport
}

type VmConfigArgs struct {
	VmClass            v1alpha1.VirtualMachineClass
	VmImage            *v1alpha1.VirtualMachineImage
	ResourcePolicy     *v1alpha1.VirtualMachineSetResourcePolicy
	VmMetadata         *VmMetadata
	StorageProfileID   string
	ContentLibraryUUID string
}

// VirtualMachineProviderInterface is a plugable interface for VM Providers
type VirtualMachineProviderInterface interface {
	Name() string

	// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
	// to perform housekeeping or run custom controllers specific to the cloud provider.
	// Any tasks started here should be cleaned up when the stop channel closes.
	Initialize(stop <-chan struct{})

	DoesVirtualMachineExist(ctx context.Context, vm *v1alpha1.VirtualMachine) (bool, error)
	CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VmConfigArgs) error
	UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmConfigArgs VmConfigArgs) error
	DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error

	CreateOrUpdateVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error
	// Used by VirtualMachine controller to determine if entities of ResourcePolicy exist on the infrastructure provider
	DoesVirtualMachineSetResourcePolicyExist(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) (bool, error)
	DeleteVirtualMachineSetResourcePolicy(ctx context.Context, resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error

	// "Infra" related
	UpdateVcPNID(ctx context.Context, vcPNID, vcPort string) error
	ClearSessionsAndClient(ctx context.Context)
	DeleteNamespaceSessionInCache(ctx context.Context, namespace string)
	ComputeClusterCpuMinFrequency(ctx context.Context) error

	ListVirtualMachineImagesFromContentLibrary(ctx context.Context, cl v1alpha1.ContentLibraryProvider) ([]*v1alpha1.VirtualMachineImage, error)
}
