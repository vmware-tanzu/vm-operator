/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package iface

import (
	"context"

	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
)

// VirtualMachineProviderInterface is a plugable interface for VM Providers
// Error types should use the k8s builtins
type VirtualMachineProviderInterface interface {
	// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
	// to perform housekeeping or run custom controllers specific to the cloud provider.
	// Any tasks started here should be cleaned up when the stop channel closes.
	Initialize(stop <-chan struct{})

	// VirtualMachines Provides a provider-specific VirtualMachines interface
	VirtualMachines() (VirtualMachines, bool)

	// VirtualMachinesImages Provides a provider-specific VirtualMachinesImages interface
	VirtualMachineImages() (VirtualMachineImages, bool)
}

type VirtualMachineImages interface {
	ListVirtualMachineImages(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachineImage, error)
	GetVirtualMachineImage(ctx context.Context, name string) (*v1alpha1.VirtualMachineImage, error)
}

type VirtualMachines interface {
	ListVirtualMachines(ctx context.Context, namespace string) ([]*v1alpha1.VirtualMachine, error)
	GetVirtualMachine(ctx context.Context, name string) (*v1alpha1.VirtualMachine, error)
	CreateVirtualMachine(ctx context.Context, virtualMachine *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error)
	UpdateVirtualMachine(ctx context.Context, virtualMachine *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error)
	DeleteVirtualMachine(ctx context.Context, virtualMachine *v1alpha1.VirtualMachine) error
}
