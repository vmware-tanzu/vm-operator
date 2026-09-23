// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package containermachine

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	containerv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-container/internal/container"
)

// reconcileContainer creates the container if it does not exist yet, then
// observes it and applies the power state its parent asked for.
//
// One pass does both, unlike external/kubevm-provider-aws's separate create
// and observe steps: a docker/podman `run -d` blocks until the engine has
// either created the container or refused, so the engine call itself never
// leaves the container's existence ambiguous the way EC2's RunInstances
// does. It is the write of the result back to this object's own status that
// can still go missing after a successful Create — see the name-conflict
// recovery below, and docs/findings.md's "A Create that succeeds can still
// look like a failure to the next reconcile."
func (r *Reconciler) reconcileContainer(ctx context.Context,
	machine *containerv1a1.ContainerMachine) (ctrl.Result, error) {

	eng := r.engine(machine.Spec.Runtime)
	name := containerName(machine)

	if machine.Status.ContainerID == "" {
		id, err := eng.Create(ctx, name, machine.Spec.Image)
		if err != nil {
			// A name collision means a prior reconcile's Create succeeded but
			// this object never got to record the id -- e.g. the status
			// patch that would have saved it lost a race. The container
			// that call made is still by name, the identifying key this
			// provider picked, so an Inspect by that same name recovers it
			// instead of leaving the object stuck retrying a Create that
			// will never succeed. Any other failure is genuinely terminal.
			if !container.IsNameConflict(err) {
				return r.reportRuntimeFailure(ctx, machine, "create", err)
			}
			ins, found, ierr := eng.Inspect(ctx, name)
			if ierr != nil || !found {
				return r.reportRuntimeFailure(ctx, machine, "create", err)
			}
			id = ins.ID
		}
		return r.settle(ctx, machine, eng, name, id)
	}
	return r.settle(ctx, machine, eng, name, machine.Status.ContainerID)
}

// settle inspects the container, applies power if needed, and writes status
// once.
func (r *Reconciler) settle(ctx context.Context,
	machine *containerv1a1.ContainerMachine, eng container.Client,
	name, id string) (ctrl.Result, error) {

	base := machine.DeepCopy()

	ins, found, err := eng.Inspect(ctx, name)
	if err != nil {
		return r.reportRuntimeFailure(ctx, machine, "inspect", err)
	}
	if !found {
		return r.vanished(ctx, machine)
	}

	machine.Status.ContainerID = id
	machine.Status.ProviderID = fmt.Sprintf("container://%s/%s",
		machine.Spec.Runtime, ins.ID)
	machine.Status.ProviderMetadata = map[string]string{
		"runtime":       string(machine.Spec.Runtime),
		"containerName": name,
	}
	machine.Status.Addresses = nil
	if ins.NetworkSettings.IPAddress != "" {
		machine.Status.Addresses = []containerv1a1.ContainerMachineAddress{{
			Interface: "eth0",
			Type:      containerv1a1.AddressInternalIP,
			Address:   ins.NetworkSettings.IPAddress,
		}}
	}

	// ABSENT while transitional, not a guess: an absent path is explicitly
	// not an error under the contract, matching what
	// external/kubevm-provider-aws does for EC2's own transitional states.
	machine.Status.PowerState = ""
	switch {
	case ins.State.Running:
		machine.Status.PowerState = "PoweredOn"
	case ins.State.Status == "exited", ins.State.Status == "created":
		machine.Status.PowerState = "PoweredOff"
	}

	res, err := r.applyPower(ctx, eng, name, ins, machine)
	if err != nil {
		return r.reportRuntimeFailure(ctx, machine, "power", err)
	}

	setCondition(machine, readiness(ins, machine.Spec.PowerState))
	setCondition(machine, upToDate(machine.Spec))

	if err := r.patchStatusIfChanged(ctx, machine, base); err != nil {
		return ctrl.Result{}, err
	}
	return res, nil
}

// applyPower starts or stops the container to match what the parent asked
// for, reading the request from THIS OBJECT'S OWN SPEC — persistResolved has
// already copied the parent's PowerState here on this same reconcile, so the
// two never disagree, but only one of them is what `kubectl get -o yaml`
// shows.
func (r *Reconciler) applyPower(ctx context.Context, eng container.Client,
	name string, ins *container.Inspect,
	machine *containerv1a1.ContainerMachine) (ctrl.Result, error) {

	switch machine.Spec.PowerState {
	case "PoweredOn":
		if ins.State.Running {
			return ctrl.Result{}, nil
		}
		if err := eng.Start(ctx, name); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: pollRequeueDelay}, nil

	case "PoweredOff":
		if !ins.State.Running {
			return ctrl.Result{}, nil
		}
		if err := eng.Stop(ctx, name); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: pollRequeueDelay}, nil
	}
	// Anything else (Suspended, or unset) has no container-engine
	// equivalent; upToDate reports it instead of acting on it.
	return ctrl.Result{}, nil
}

// readiness reports InfrastructureReady from the container's observed state
// against the power state asked for.
func readiness(ins *container.Inspect, want string) metav1.Condition {
	switch {
	case ins.State.Running && want == "PoweredOff":
		return notReady(containerv1a1.ReasonPowerChanging,
			"the container is running; stopping it, as asked")
	case !ins.State.Running && want == "PoweredOn":
		return notReady(containerv1a1.ReasonPowerChanging,
			"the container is stopped; starting it, as asked")
	case ins.State.Running:
		return ready(containerv1a1.ReasonRunning, "the container is running")
	case ins.State.Status == "exited", ins.State.Status == "created":
		return ready(containerv1a1.ReasonStopped, "the container is stopped")
	}
	return notReady(containerv1a1.ReasonProvisioning,
		fmt.Sprintf("the container is %s", ins.State.Status))
}

// upToDate reports the one request this provider cannot honour: a Suspended
// power state, which neither docker nor podman implements.
func upToDate(spec containerv1a1.ContainerMachineSpec) metav1.Condition {
	if spec.PowerState != "" && spec.PowerState != "PoweredOn" &&
		spec.PowerState != "PoweredOff" {
		return metav1.Condition{
			Type:   containerv1a1.ConditionUpToDate,
			Status: metav1.ConditionFalse,
			Reason: containerv1a1.ReasonUnsupportedByProvider,
			Message: fmt.Sprintf("powerState %q has no docker/podman "+
				"equivalent and was not applied", spec.PowerState),
		}
	}
	return metav1.Condition{
		Type:   containerv1a1.ConditionUpToDate,
		Status: metav1.ConditionTrue,
		Reason: "Applied",
	}
}

func ready(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    containerv1a1.ConditionInfrastructureReady,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

func notReady(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    containerv1a1.ConditionInfrastructureReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

// vanished reports a container the engine no longer knows about.
//
// Terminal, not a wait: unlike EC2, there is no propagation delay for a
// docker/podman name lookup to catch up with — see internal/container's
// package comment — so an inspect miss right after create.go's `Create`
// call already succeeded (and returned an id) means something else removed
// the container in between, not that the engine has not caught up yet.
func (r *Reconciler) vanished(ctx context.Context,
	machine *containerv1a1.ContainerMachine) (ctrl.Result, error) {

	base := machine.DeepCopy()
	machine.Status.PowerState = ""
	machine.Status.Addresses = nil
	machine.Status.ProviderMetadata = nil
	setCondition(machine, notReady(containerv1a1.ReasonContainerGone,
		fmt.Sprintf("container %q no longer exists", containerName(machine))))
	return ctrl.Result{}, r.patchStatusIfChanged(ctx, machine, base)
}

// reportRuntimeFailure records an engine-call failure as a condition.
func (r *Reconciler) reportRuntimeFailure(ctx context.Context,
	machine *containerv1a1.ContainerMachine, op string, err error) (
	ctrl.Result, error) {

	base := machine.DeepCopy()
	setCondition(machine, notReady(containerv1a1.ReasonRuntimeError,
		fmt.Sprintf("%s: %v", op, err)))
	if perr := r.patchStatusIfChanged(ctx, machine, base); perr != nil {
		return ctrl.Result{}, perr
	}
	return ctrl.Result{}, fmt.Errorf("%s: %w", op, err)
}
