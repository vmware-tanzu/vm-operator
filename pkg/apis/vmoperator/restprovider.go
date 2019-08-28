/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vmoperator

import (
	"errors"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/klogr"
)

// This is in this package to avoid an import cycle.
// Registered rest provider.  Only one is allowed.
var (
	providerMutex sync.Mutex
	provider      *RestProvider
)

type RestProviderInterface interface {
	New() runtime.Object
}

// +k8s:deepcopy-gen=false
type RestProvider struct {
	ImagesProvider RestProviderInterface
}

func RegisterRestProvider(restProvider RestProvider) error {
	providerMutex.Lock()
	defer providerMutex.Unlock()

	if provider != nil {
		return errors.New("REST provider already registered")
	}

	log := klogr.New()
	log.V(1).Info("REST provider registered")
	provider = &restProvider
	return nil
}

func GetRestProvider() *RestProvider {
	providerMutex.Lock()
	defer providerMutex.Unlock()
	return provider
}
