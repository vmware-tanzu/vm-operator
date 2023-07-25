// Copyright (c) 2020-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"sync"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	prober "github.com/vmware-tanzu/vm-operator/pkg/prober2"
)

type funcs struct {
	AddToProberManagerFn      func(vm *vmopv1.VirtualMachine)
	RemoveFromProberManagerFn func(vm *vmopv1.VirtualMachine)
}

type ProberManager struct {
	funcs
	sync.Mutex
	IsAddToProberManagerCalled      bool
	IsRemoveFromProberManagerCalled bool
}

func NewFakeProberManager() prober.Manager {
	return &ProberManager{}
}

func (m *ProberManager) Start(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

func (m *ProberManager) AddToProberManager(vm *vmopv1.VirtualMachine) {
	m.Lock()
	defer m.Unlock()

	if m.AddToProberManagerFn != nil {
		m.AddToProberManagerFn(vm)
		return
	}
}

func (m *ProberManager) RemoveFromProberManager(vm *vmopv1.VirtualMachine) {
	m.Lock()
	defer m.Unlock()

	if m.RemoveFromProberManagerFn != nil {
		m.RemoveFromProberManagerFn(vm)
		return
	}
}
