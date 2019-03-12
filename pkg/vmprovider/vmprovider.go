/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmprovider

import (
	"sync"

	"github.com/golang/glog"
	"vmware.com/kubevsphere/pkg/vmprovider/iface"
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
		glog.Fatal("VM provider is already registered")
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
