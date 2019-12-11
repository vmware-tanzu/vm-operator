/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"fmt"
	"sync"
)

var (
	mutex                sync.Mutex
	registeredVmProvider VirtualMachineProviderInterface
)

// RegisterVmProviderOrDie registers the provider.
func RegisterVmProviderOrDie(vmProvider VirtualMachineProviderInterface) VirtualMachineProviderInterface {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider != nil {
		panic(fmt.Sprintf("VM provider is already registered: providerName %s", registeredVmProvider.Name()))
	}

	registeredVmProvider = vmProvider
	return vmProvider
}

func UnregisterVmProviderOrDie(vmProvider VirtualMachineProviderInterface) {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider.Name() != vmProvider.Name() {
		panic(fmt.Sprintf("VM provider does not match unregister request: currently registered %s, to unregister %s", registeredVmProvider.Name(), vmProvider.Name()))
	}

	registeredVmProvider = nil
}

// GetVmProviderOrDie returns the registered provider or dies if not registered.
func GetVmProviderOrDie() VirtualMachineProviderInterface {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider == nil {
		panic("No VM provider registered")
	}

	return registeredVmProvider
}
