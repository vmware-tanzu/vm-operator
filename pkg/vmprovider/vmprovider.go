/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"os"
	"sync"

	"k8s.io/klog/klogr"
)

var (
	mutex                sync.Mutex
	registeredVmProvider VirtualMachineProviderInterface
	log                  = klogr.New()
)

// RegisterVmProvider registers the provider.
func RegisterVmProvider(vmProvider VirtualMachineProviderInterface) {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider != nil {
		log.Error(nil, "VM provider is already registered", "providerName", registeredVmProvider.Name())
		os.Exit(255)
	}

	registeredVmProvider = vmProvider
}

// GetVmProvider returns the registered provider or nil.
func GetVmProvider() VirtualMachineProviderInterface {
	mutex.Lock()
	defer mutex.Unlock()

	return registeredVmProvider
}

// GetVmProviderOrDie returns the registered provider or dies if not registered.
func GetVmProviderOrDie() VirtualMachineProviderInterface {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider == nil {
		log.Error(nil, "No VM provider registered")
		os.Exit(255)
	}

	return registeredVmProvider
}
