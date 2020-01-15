// Copyright (c) 2019-2020  VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmoperator

import (
	"errors"
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
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

	provider = &restProvider
	return nil
}

func GetRestProvider() *RestProvider {
	providerMutex.Lock()
	defer providerMutex.Unlock()
	return provider
}
