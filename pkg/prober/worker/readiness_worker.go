// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/patch"
	proberctx "github.com/vmware-tanzu/vm-operator/pkg/prober/context"
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
	queue    DelayingInterface
	prober   *probe.Prober
	client   client.Client
	recorder vmoprecord.Recorder
}

// NewReadinessWorker creates a new readiness worker to run readiness probes.
func NewReadinessWorker(
	queue DelayingInterface,
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

func (w *readinessWorker) GetQueue() DelayingInterface {
	return w.queue
}

// CreateProbeContext creates a probe context for readiness probe.
func (w *readinessWorker) CreateProbeContext(vm *vmopv1.VirtualMachine) (*proberctx.ProbeContext, error) {
	p := vm.Spec.ReadinessProbe

	if p.TCPSocket == nil && p.GuestHeartbeat == nil && len(p.GuestInfo) == 0 {
		return nil, nil
	}

	patchHelper, err := patch.NewHelper(vm, w.client)
	if err != nil {
		return nil, err
	}

	return &proberctx.ProbeContext{
		Context:       context.Background(),
		Logger:        ctrl.Log.WithName("readiness-probe").WithValues("vmName", vm.NamespacedName()),
		PatchHelper:   patchHelper,
		VM:            vm,
		ProbeType:     "readiness",
		PeriodSeconds: p.PeriodSeconds,
	}, nil
}

// ProcessProbeResult processes probe results to get ReadyCondition and
// sets the ReadyCondition in vm status if the new condition status is a transition.
func (w *readinessWorker) ProcessProbeResult(ctx *proberctx.ProbeContext, res probe.Result, resErr error) error {
	vm := ctx.VM
	condition := w.getCondition(res, resErr)

	// We only send event when either the condition type is added or its status changes, not
	// if either its reason, severity, or message changes.
	if c := conditions.Get(vm, condition.Type); c == nil || c.Status != condition.Status {
		if condition.Status == metav1.ConditionTrue {
			w.recorder.Eventf(vm, readyReason, "")
		} else {
			w.recorder.Eventf(vm, condition.Reason, condition.Message)
		}
		// Log the time when the VM changes its readiness condition.
		ctx.Logger.Info("VM resource READINESS probe condition updated",
			"condition.status", condition.Status, "time", condition.LastTransitionTime,
			"reason", condition.Reason)
	}

	conditions.Set(vm, condition)

	err := ctx.PatchHelper.Patch(ctx, vm, patch.WithOwnedConditions{
		Conditions: []string{vmopv1.ReadyConditionType},
	})
	if err != nil {
		return fmt.Errorf("patched failed: %w", err)
	}

	return nil
}

func (w *readinessWorker) DoProbe(ctx *proberctx.ProbeContext) error {
	res, err := w.runProbe(ctx)
	if err != nil {
		ctx.Logger.Error(err, "readiness probe fails", "result", res)
	}
	return w.ProcessProbeResult(ctx, res, err)
}

// getProbe returns a specific type of probe method.
func (w *readinessWorker) getProbe(probeSpec *vmopv1.VirtualMachineReadinessProbeSpec) probe.Probe {
	if probeSpec == nil {
		return nil
	}

	if probeSpec.TCPSocket != nil {
		return w.prober.TCPProbe
	}
	if probeSpec.GuestHeartbeat != nil {
		return w.prober.GuestHeartbeat
	}
	if len(probeSpec.GuestInfo) != 0 {
		return w.prober.GuestInfo
	}

	return nil
}

// runProbe runs a specific type of probe based on the VM probe spec.
func (w *readinessWorker) runProbe(ctx *proberctx.ProbeContext) (probe.Result, error) {
	if p := w.getProbe(ctx.VM.Spec.ReadinessProbe); p != nil {
		return p.Probe(ctx)
	}

	return probe.Unknown, fmt.Errorf("unknown action specified for VM %s readiness probe", ctx.VM.NamespacedName())
}

// getCondition returns condition based on VM probe results.
func (w *readinessWorker) getCondition(res probe.Result, err error) *metav1.Condition {
	msg := ""
	if err != nil {
		msg = err.Error()
	}

	switch res {
	case probe.Success:
		return conditions.TrueCondition(vmopv1.ReadyConditionType)
	case probe.Failure:
		return conditions.FalseCondition(vmopv1.ReadyConditionType, notReadyReason, "%s", msg)
	default: // probe.Unknown
		return conditions.UnknownCondition(vmopv1.ReadyConditionType, unknownReason, "%s", msg)
	}
}
