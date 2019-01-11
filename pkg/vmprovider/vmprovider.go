/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"fmt"
	"github.com/golang/glog"
	"io"
	"os"
	"sync"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
)

// Factory is a function that returns a vmprovider.Interface.
// The config parameter provides an io.Reader handler to the factory in
// order to load specific configurations. If no configuration is provided
// the parameter is nil.
type Factory func(config io.Reader) (iface.VirtualMachineProviderInterface, error)

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
		glog.Fatalf("VM provider %q was registered twice", name)
	}
	glog.V(1).Infof("Registered VM provider %q", name)
	providers[name] = vmProvider
}

// GetVmProvider creates an instance of the named VM provider, or nil if
// the name is unknown.  The error return is only used if the named provider
// was known but failed to initialize. The config parameter specifies the
// io.Reader handler of the configuration file for the vm provider, or nil
// for no configuration.
func GetVmProvider(name string, config io.Reader) (iface.VirtualMachineProviderInterface, error) {
	providersMutex.Lock()
	defer providersMutex.Unlock()
	f, found := providers[name]
	if !found {
		return nil, nil
	}
	return f(config)
}

// InitVmProvider creates an instance of the named VM provider.
func InitVmProvider(name string, configFilePath string) (iface.VirtualMachineProviderInterface, error) {
	var vmProvider iface.VirtualMachineProviderInterface
	var err error

	if name == "" {
		glog.Info("No VM provider specified.")
		return nil, nil
	}

	if configFilePath != "" {
		var config *os.File
		config, err = os.Open(configFilePath)
		if err != nil {
			glog.Fatalf("Couldn't open VM provider configuration %s: %#v",
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

func NewVmProvider() (iface.VirtualMachineProviderInterface, error) {
	return InitVmProvider("vsphere", "")
}
