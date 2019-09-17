/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"context"

	"github.com/vmware-tanzu/vm-operator/pkg/apis/vmoperator/v1alpha1"
)

type VirtualMachineMetadata map[string]string

// VirtualMachineProviderInterface is a plugable interface for VM Providers
type VirtualMachineProviderInterface interface {
	Name() string

	// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
	// to perform housekeeping or run custom controllers specific to the cloud provider.
	// Any tasks started here should be cleaned up when the stop channel closes.
	Initialize(stop <-chan struct{})

	ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error)
	GetVirtualMachineImage(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachineImage, error)

	ListVirtualMachines(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachine, error)
	DoesVirtualMachineExist(ctx context.Context, namespace, name string) (bool, error)

	CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
		vmClass v1alpha1.VirtualMachineClass, vmMetadata VirtualMachineMetadata, profileID string) error
	UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine,
		vmClass v1alpha1.VirtualMachineClass, vmMetadata VirtualMachineMetadata) error
	DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error
}
