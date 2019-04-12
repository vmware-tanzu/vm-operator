/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"sync"

	"github.com/golang/glog"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/iface"
)

var (
	mutex                sync.Mutex
	registeredVmProvider iface.VirtualMachineProviderInterface
)

// RegisterVmProvider registers the provider.
func RegisterVmProvider(vmProvider iface.VirtualMachineProviderInterface) {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider != nil {
		glog.Fatalf("VM provider %q is already registered", registeredVmProvider.Name())
	}

	registeredVmProvider = vmProvider
}

// GetVmProvider returns the registered provider or nil.
func GetVmProvider() iface.VirtualMachineProviderInterface {
	mutex.Lock()
	defer mutex.Unlock()

	return registeredVmProvider
}

// GetVmProviderOrDie returns the registered provider or dies if not registered.
func GetVmProviderOrDie() iface.VirtualMachineProviderInterface {
	mutex.Lock()
	defer mutex.Unlock()

	if registeredVmProvider == nil {
		glog.Fatal("No VM provider registered")
	}

	return registeredVmProvider
}
