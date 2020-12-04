// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"sync"

	vmoperatorv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/prober"
)

type funcs struct {
	AddToProberManagerFn      func(vm *vmoperatorv1alpha1.VirtualMachine)
	RemoveFromProberManagerFn func(vm *vmoperatorv1alpha1.VirtualMachine)
}

type FakeProberManager struct {
	funcs
	sync.Mutex
	IsAddToProberManagerCalled      bool
	IsRemoveFromProberManagerCalled bool
}

func NewFakeProberManager() prober.Manager {
	return &FakeProberManager{}
}

func (m *FakeProberManager) Start(stopChan <-chan struct{}) error {
	<-stopChan
	return nil
}

func (m *FakeProberManager) AddToProberManager(vm *vmoperatorv1alpha1.VirtualMachine) {
	m.Lock()
	defer m.Unlock()

	if m.AddToProberManagerFn != nil {
		m.AddToProberManagerFn(vm)
		return
	}
}

func (m *FakeProberManager) RemoveFromProberManager(vm *vmoperatorv1alpha1.VirtualMachine) {
	m.Lock()
	defer m.Unlock()

	if m.RemoveFromProberManagerFn != nil {
		m.RemoveFromProberManagerFn(vm)
		return
	}
}
