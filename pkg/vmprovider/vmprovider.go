/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"sync"

	"k8s.io/klog"
)

var (
	mutex                sync.Mutex
	registeredVmProvider VirtualMachineProviderInterface
)

// RegisterVmProvider registers the provider.
func RegisterVmProvider(vmProvider VirtualMachineProviderInterface) {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider != nil {
		klog.Fatalf("VM provider %q is already registered", registeredVmProvider.Name())
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
		klog.Fatal("No VM provider registered")
	}

	return registeredVmProvider
}
