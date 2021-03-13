// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	goctx "context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/pkg/errors"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober/probe"
	vmoprecord "github.com/vmware-tanzu/vm-operator/pkg/record"
)

const (
	// readyReason, notReadyReason and unknownReason represent reasons for probe events and Condition.
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

// CreateProbeContext creates a probe context for readiness probe.
func (w *readinessWorker) CreateProbeContext(vm *vmopv1alpha1.VirtualMachine) (*context.ProbeContext, error) {
	patchHelper, err := patch.NewHelper(vm, w.client)
	if err != nil {
		return nil, err
	}

	return &context.ProbeContext{
		Context:     goctx.Background(),
		Logger:      ctrl.Log.WithName("readiness-probe").WithValues("vmName", vm.NamespacedName()),
		PatchHelper: patchHelper,
		VM:          vm,
		ProbeSpec:   vm.Spec.ReadinessProbe,
		ProbeType:   "readiness",
	}, nil
}

// ProcessProbeResult processes probe results to get ReadyCondition and
// sets the ReadyCondition in vm status if the new condition status is a transition.
func (w *readinessWorker) ProcessProbeResult(ctx *context.ProbeContext, res probe.Result, resErr error) error {
	vm := ctx.VM
	condition := w.getCondition(res, resErr)

	// We only send event when either the condition type is added or its status changes, not
	// if either its reason, severity, or message changes.
	if c := conditions.Get(vm, condition.Type); c == nil || c.Status != condition.Status {
		if condition.Status == corev1.ConditionTrue {
			w.recorder.Eventf(vm, readyReason, "")
		} else {
			w.recorder.Eventf(vm, condition.Reason, condition.Message)
		}
	}

	conditions.Set(vm, condition)

	err := ctx.PatchHelper.Patch(ctx, vm, patch.WithOwnedConditions{
		Conditions: []vmopv1alpha1.ConditionType{vmopv1alpha1.ReadyCondition},
	})
	if err != nil {
		return errors.Wrapf(err, "patched failed")
	}

	return nil
}

func (w *readinessWorker) DoProbe(ctx *context.ProbeContext) error {
	res, err := w.runProbe(ctx)
	if err != nil {
		ctx.Logger.Error(err, "readiness probe fails")
	}
	return w.ProcessProbeResult(ctx, res, err)
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

// getCondition returns condition based on VM probe results.
func (w *readinessWorker) getCondition(res probe.Result, err error) *vmopv1alpha1.Condition {
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	switch res {
	case probe.Success:
		return conditions.TrueCondition(vmopv1alpha1.ReadyCondition)
	case probe.Failure:
		return conditions.FalseCondition(vmopv1alpha1.ReadyCondition, notReadyReason, vmopv1alpha1.ConditionSeverityInfo, msg)
	default: // probe.Unknown
		return conditions.UnknownCondition(vmopv1alpha1.ReadyCondition, unknownReason, msg)
	}
}
