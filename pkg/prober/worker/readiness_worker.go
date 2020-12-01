// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	goctx "context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	vmoprecord "github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	// readyReason, notReadyReason and unknownReason represent reasons for probe events and VirtualMachineCondition.
	readyReason    string = "Ready"
	notReadyReason string = "NotReady"
	unknownReason  string = "Unknown"
)

// readinessWorker implements Worker interface.
type readinessWorker struct {
	queue    workqueue.DelayingInterface
	prober   *probe.Prober
	client   client.Client
	recorder vmoprecord.Recorder
}

// NewReadinessWorker creates a new readiness worker to run readiness probes.
func NewReadinessWorker(
	queue workqueue.DelayingInterface,
	prober *probe.Prober,
	client client.Client,
	recorder vmoprecord.Recorder,
) Worker {
	return &readinessWorker{
		queue:    queue,
		prober:   prober,
		client:   client,
		recorder: recorder,
	}
}

func (w *readinessWorker) GetQueue() workqueue.DelayingInterface {
	return w.queue
}

func (w *readinessWorker) AddAfterToQueue(item client.ObjectKey, periodSeconds int32) {
	if periodSeconds <= 0 {
		w.queue.Add(item)
		return
	}
	w.queue.AddAfter(item, time.Duration(periodSeconds)*time.Second)
}

// ProcessProbeResult processes probe results to get VirtualMachineReady condition value and
// sets the VirtualMachineReady condition in vm status if the new condition status is a transition.
func (w *readinessWorker) ProcessProbeResult(vm *vmopv1alpha1.VirtualMachine, res probe.Result, err error) error {
	status, reason, message := w.getConditionValue(res, err)
	oldVM := vm.DeepCopy()
	if statusChanged := SetVirtualMachineCondition(vm, vmopv1alpha1.VirtualMachineReady, status, reason, message); statusChanged {
		// if new condition is a transition, update VM status
		// note: right now, this patch will fail unless most of the VM status has been populated
		patchErr := w.client.Status().Patch(goctx.Background(), vm, client.MergeFrom(oldVM))
		w.recorder.Eventf(vm, reason, message)
		return patchErr
	}

	return nil
}

func (w *readinessWorker) DoProbe(vm *vmopv1alpha1.VirtualMachine) error {
	ctx := w.createProbeContext(vm)
	if ctx.ProbeSpec == nil {
		ctx.Logger.V(4).Info("readiness probe not specified")
		return nil
	}

	res, err := w.runProbe(ctx)
	if err != nil {
		ctx.Logger.Error(err, "readiness probe fails")
	}
	return w.ProcessProbeResult(vm, res, err)
}

// createProbeContext creates a probe context for readiness probe.
func (w *readinessWorker) createProbeContext(vm *vmopv1alpha1.VirtualMachine) *context.ProbeContext {
	return &context.ProbeContext{
		Context:   goctx.Background(),
		Logger:    ctrl.Log.WithName("readiness-probe").WithValues("vmName", vm.NamespacedName()),
		VM:        vm,
		ProbeSpec: vm.Spec.ReadinessProbe,
		ProbeType: "readiness",
	}
}

// getProbe returns a specific type of probe method.
func (w *readinessWorker) getProbe(probeSpec *vmopv1alpha1.Probe) probe.Probe {
	if probeSpec.TCPSocket != nil {
		return w.prober.TCPProbe
	}

	return nil
}

// runProbe runs a specific type of probe based on the VM probe spec.
func (w *readinessWorker) runProbe(ctx *context.ProbeContext) (probe.Result, error) {
	if p := w.getProbe(ctx.ProbeSpec); p != nil {
		return p.Probe(ctx)
	}

	return probe.Unknown, fmt.Errorf("unknown action specified for VM %s readiness probe", ctx.VM.NamespacedName())
}

// getConditionValue returns condition status, reason and messages based on VM probe results.
func (w *readinessWorker) getConditionValue(res probe.Result, err error) (status metav1.ConditionStatus, reason, message string) {
	if err != nil {
		message = err.Error()
	}

	switch res {
	case probe.Success:
		status = metav1.ConditionTrue
		reason = readyReason
	case probe.Unknown:
		status = metav1.ConditionUnknown
		reason = unknownReason
	case probe.Failure:
		status = metav1.ConditionFalse
		reason = notReadyReason
	}

	return status, reason, message
}
