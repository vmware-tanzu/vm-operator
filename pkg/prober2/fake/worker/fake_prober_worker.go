// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	goctx "context"
	"fmt"
	"sync"

	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/worker"
)

type funcs struct {
	DoProbeFn            func(ctx *context.ProbeContext) error
	ProcessProbeResultFn func(ctx *context.ProbeContext, res probe.Result, err error) error
}

type FakeWorker struct {
	sync.Mutex
	funcs
	queue workqueue.DelayingInterface
}

func NewFakeWorker(queue workqueue.DelayingInterface) worker.Worker {
	return &FakeWorker{
		queue: queue,
	}
}

func (w *FakeWorker) Reset() {
	w.Lock()
	defer w.Unlock()

	w.funcs = funcs{}
}

func (w *FakeWorker) GetQueue() workqueue.DelayingInterface {
	return w.queue
}

func (w *FakeWorker) CreateProbeContext(vm *vmopv1.VirtualMachine) (*context.ProbeContext, error) {
	if vm.Spec.ReadinessProbe.TCPSocket == nil {
		return nil, nil
	}

	return &context.ProbeContext{
		Context:       goctx.Background(),
		Logger:        ctrl.Log.WithName("fake-probe").WithValues("vmName", vm.NamespacedName()),
		VM:            vm,
		ProbeType:     "fake-probe",
		PeriodSeconds: vm.Spec.ReadinessProbe.PeriodSeconds,
	}, nil
}

func (w *FakeWorker) DoProbe(ctx *context.ProbeContext) error {
	w.Lock()
	defer w.Unlock()

	if w.funcs.DoProbeFn != nil {
		return w.funcs.DoProbeFn(ctx)
	}
	return fmt.Errorf("unexpected method call: DoProbe")
}

func (w *FakeWorker) ProcessProbeResult(ctx *context.ProbeContext, res probe.Result, err error) error {
	w.Lock()
	defer w.Unlock()

	if w.funcs.ProcessProbeResultFn != nil {
		return w.funcs.ProcessProbeResultFn(ctx, res, err)
	}
	return fmt.Errorf("unexpected method call: ProcessProbeResult")
}
