/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package iface

import (
	"context"

	"gitlab.eng.vmware.com/iaas-platform/vm-operator/pkg/apis/vmoperator/v1alpha1"
)

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
	GetVirtualMachine(ctx context.Context, namespace, name string) (*v1alpha1.VirtualMachine, error)

	CreateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine, vmClass *v1alpha1.VirtualMachineClass) (*v1alpha1.VirtualMachine, error)
	UpdateVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) (*v1alpha1.VirtualMachine, error)
	DeleteVirtualMachine(ctx context.Context, vm *v1alpha1.VirtualMachine) error
}
