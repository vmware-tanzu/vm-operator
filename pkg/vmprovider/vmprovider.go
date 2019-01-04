/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"context"
	"fmt"
	"io"
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"
	"log"
	"os"
	"sync"
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

// DWB: Need to get rid of these intermediate types and resolve the import cycle for the k8s types
type VirtualMachineImage struct {
	Name string
	PowerState string
	Uuid string
	InternalId string
}

type VirtualMachine struct {
	Name string
	PowerState string
	Uuid string
	InternalId string
}

type VirtualMachineImages interface {
	ListVirtualMachineImages(ctx context.Context, namespace string) ([]VirtualMachineImage, error)
	GetVirtualMachineImage(ctx context.Context, name string) (VirtualMachineImage, error)
}

type VirtualMachines interface {
	ListVirtualMachines(ctx context.Context, namespace string) ([]VirtualMachine, error)
	GetVirtualMachine(ctx context.Context, name string) (VirtualMachine, error)
	CreateVirtualMachine(ctx context.Context, virtualMachine VirtualMachine) (error)
	DeleteVirtualMachine(ctx context.Context, name string) (error)
}

// Factory is a function that returns a vmprovider.Interface.
// The config parameter provides an io.Reader handler to the factory in
// order to load specific configurations. If no configuration is provided
// the parameter is nil.
type Factory func(config io.Reader) (VirtualMachineProviderInterface, error)

// All registered VM providers.
var (
	providersMutex           sync.Mutex
	providers                = make(map[string]Factory)
)

// RegisterVmProvider registers a vmprovider.Factory by name.  This
// is expected to happen during app startup.
func RegisterVmProvider(name string, vmProvider Factory) {
	providersMutex.Lock()
	defer providersMutex.Unlock()
	if _, found := providers[name]; found {
		klog.Fatalf("VM provider %q was registered twice", name)
	}
	klog.V(1).Infof("Registered VM provider %q", name)
	providers[name] = vmProvider
}

// GetVmProvider creates an instance of the named VM provider, or nil if
// the name is unknown.  The error return is only used if the named provider
// was known but failed to initialize. The config parameter specifies the
// io.Reader handler of the configuration file for the vm provider, or nil
// for no configuration.
func GetVmProvider(name string, config io.Reader) (VirtualMachineProviderInterface, error) {
	providersMutex.Lock()
	defer providersMutex.Unlock()
	f, found := providers[name]
	if !found {
		return nil, nil
	}
	return f(config)
}

// InitVmProvider creates an instance of the named VM provider.
func InitVmProvider(name string, configFilePath string) (VirtualMachineProviderInterface, error) {
	var vmProvider VirtualMachineProviderInterface
	var err error

	if name == "" {
		klog.Info("No VM provider specified.")
		return nil, nil
	}

	if configFilePath != "" {
		var config *os.File
		config, err = os.Open(configFilePath)
		if err != nil {
			klog.Fatalf("Couldn't open VM provider configuration %s: %#v",
				configFilePath, err)
		}

		defer config.Close()
		vmProvider, err = GetVmProvider(name, config)
	} else {
		// Pass explicit nil so plugins can actually check for nil. See
		// "Why is my nil error value not equal to nil?" in golang.org/doc/faq.
		vmProvider, err = GetVmProvider(name, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("could not init VM provider %q: %v", name, err)
	}
	if vmProvider == nil {
		return nil, fmt.Errorf("unknown VM provider %q", name)
	}

	return vmProvider, nil
}

func NewVmProvider(namespace string) (VirtualMachineProviderInterface, error) {
	log.Print("Lookup VM provider")
	return InitVmProvider("vsphere", "")
}
