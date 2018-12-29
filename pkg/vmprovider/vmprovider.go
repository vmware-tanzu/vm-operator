/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"context"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

// ClientBuilder allows you to get clients and configs for controllers
type ClientBuilder interface {
	Config(name string) (*restclient.Config, error)
	ConfigOrDie(name string) *restclient.Config
	Client(name string) (clientset.Interface, error)
	ClientOrDie(name string) clientset.Interface
}


// Pluggable interface for VM Providers
type VirtualMachineProviderInterface interface {
	// Initialize provides the cloud with a kubernetes client builder and may spawn goroutines
	// to perform housekeeping or run custom controllers specific to the cloud provider.
	// Any tasks started here should be cleaned up when the stop channel closes.
	Initialize(clientBuilder ClientBuilder, stop <-chan struct{})

	// VirtualMachines Provides a provider-specific VirtualMachines interface
	VirtualMachines() (VirtualMachines, bool)

	// VirtualMachinesImages Provides a provider-specific VirtualMachinesImages interface
	VirtualMachineImages() (VirtualMachineImages, bool)
}

type VirtualMachineImage struct {
	Name string
}

type VirtualMachineImages interface {
	ListVirtualMachineImages(ctx context.Context, namespace string) ([]*VirtualMachineImage, error)
	GetVirtualMachineImage(ctx context.Context, name string) (*VirtualMachineImage, error)
}

type VirtualMachine struct {
	Name string
}

type VirtualMachines interface {
	ListVirtualMachines(ctx context.Context, namespace string) ([]*VirtualMachine, error)
	GetVirtualMachine(ctx context.Context, name string) (*VirtualMachine, error)
}
