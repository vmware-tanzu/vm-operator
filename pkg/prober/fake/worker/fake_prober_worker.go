// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"fmt"
	"sync"

	ctrl "sigs.k8s.io/controller-runtime"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"

	proberctx "github.com/vmware-tanzu/vm-operator/pkg/prober/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/worker"
)

type funcs struct {
	DoProbeFn            func(ctx *proberctx.ProbeContext) error
	ProcessProbeResultFn func(ctx *proberctx.ProbeContext, res probe.Result, err error) error
}

type FakeWorker struct {
	sync.Mutex
	funcs
	queue worker.DelayingInterface
}

func NewFakeWorker(queue worker.DelayingInterface) worker.Worker {
	return &FakeWorker{
		queue: queue,
	}
}

func (w *FakeWorker) Reset() {
	w.Lock()
	defer w.Unlock()

	w.funcs = funcs{}
}

func (w *FakeWorker) GetQueue() worker.DelayingInterface {
	return w.queue
}

func (w *FakeWorker) CreateProbeContext(vm *vmopv1.VirtualMachine) (*proberctx.ProbeContext, error) {
	if vm.Spec.ReadinessProbe.TCPSocket == nil {
		return nil, nil
	}

	return &proberctx.ProbeContext{
		Context:       context.Background(),
		Logger:        ctrl.Log.WithName("fake-probe").WithValues("vmName", vm.NamespacedName()),
		VM:            vm,
		ProbeType:     "fake-probe",
		PeriodSeconds: vm.Spec.ReadinessProbe.PeriodSeconds,
	}, nil
}

func (w *FakeWorker) DoProbe(ctx *proberctx.ProbeContext) error {
	w.Lock()
	defer w.Unlock()

	if w.funcs.DoProbeFn != nil {
		return w.funcs.DoProbeFn(ctx)
	}
	return fmt.Errorf("unexpected method call: DoProbe")
}

func (w *FakeWorker) ProcessProbeResult(ctx *proberctx.ProbeContext, res probe.Result, err error) error {
	w.Lock()
	defer w.Unlock()

	if w.funcs.ProcessProbeResultFn != nil {
		return w.funcs.ProcessProbeResultFn(ctx, res, err)
	}
	return fmt.Errorf("unexpected method call: ProcessProbeResult")
}
