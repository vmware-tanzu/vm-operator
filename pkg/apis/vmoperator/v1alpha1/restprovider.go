/* **********************************************************
 * Copyright 2018 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/
package v1alpha1

import (
	"sync"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/runtime"
)

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
		glog.Fatal("Rest provider is already registered")
	}
	glog.V(1).Infof("Registered Rest provider")
	provider = &restProvider
	return nil
}

func GetRestProvider() *RestProvider {
	providerMutex.Lock()
	defer providerMutex.Unlock()
	return provider
}
