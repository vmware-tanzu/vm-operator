// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package awsmachine

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/redact"
)

// reconcileDelete terminates the instance, then releases the object.
//
// The finalizer is what makes this the only path that can remove the object,
// and it is load-bearing: the core deletes the provider object and waits, so
// without it the EC2 instance simply outlives everything that knew about it
// and goes on billing. That is the most expensive failure this provider can
// have, and it is invisible from inside the cluster.
func (r *Reconciler) reconcileDelete(ctx context.Context,
	machine *awsv1a1.AWSMachine) (ctrl.Result, error) {

	if !controllerutil.ContainsFinalizer(machine, awsv1a1.Finalizer) {
		return ctrl.Result{}, nil
	}

	id := instanceIDFrom(machine.Status.ProviderID)
	if id == "" {
		if machine.Status.LaunchRequested == nil {
			// No launch was ever asked for, so nothing exists.
			return ctrl.Result{}, r.releaseFinalizer(ctx, machine)
		}

		// A launch was asked for and never recorded: its reply may have been
		// lost after EC2 acted. The ClientToken stops a second launch; it
		// does not prove there was no first, so look for it.
		found, err := r.findByToken(ctx, machine)
		if err != nil {
			return r.holdDeleting(ctx, machine, redact.Redact(
				ec2.Describe("DescribeInstances", err)))
		}
		if found == nil {
			if r.withinGrace(machine) {
				return r.holdDeleting(ctx, machine, "checking whether the "+
					"launch created an instance before releasing the object")
			}
			return ctrl.Result{}, r.releaseFinalizer(ctx, machine)
		}
		id = aws.ToString(found.InstanceId)
	}

	gone, notFound, err := r.terminate(ctx, id)
	if err != nil {
		// Redacted before it reaches the condition AND before it reaches the
		// log: returning the raw error here would put whatever the platform
		// said into the controller's own output, which is the thing
		// internal/redact exists to prevent.
		message := redact.Redact(ec2.Describe("TerminateInstances", err))
		log.FromContext(ctx).Error(nil, "terminating the instance failed",
			"instance", id, "detail", message)
		if cerr := r.markNotReady(ctx, machine, awsv1a1.ReasonDeleting,
			message); cerr != nil {
			return ctrl.Result{}, cerr
		}
		// Either way the workqueue decides when to look again: a retryable
		// failure backs off, a terminal one is a reported error.
		return ctrl.Result{}, fmt.Errorf("terminating %s: %s", id, message)
	}
	// Not found moments after a launch may only mean EC2 does not show the
	// new instance yet, as it does for DescribeInstances. Wait out that
	// window before believing it.
	if notFound && r.withinGrace(machine) {
		return r.holdDeleting(ctx, machine, fmt.Sprintf("waiting to see "+
			"instance %s before releasing the object: EC2 may not show a "+
			"new instance yet", id))
	}
	if gone {
		return ctrl.Result{}, r.releaseFinalizer(ctx, machine)
	}

	// Still shutting down. Hold the finalizer: releasing it now would let
	// the object disappear while the machine is still alive, which is
	// exactly the leak the finalizer exists to prevent.
	if err := r.markNotReady(ctx, machine, awsv1a1.ReasonDeleting,
		fmt.Sprintf("waiting for instance %s to terminate", id)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: bootRequeue}, nil
}

// terminate asks EC2 to destroy the instance and reports whether it is gone.
//
// "Gone" covers two cases that cannot be told apart and do not need to be:
// terminated, and forgotten. EC2 drops terminated instances from its API after
// roughly an hour, so a delete arriving late finds nothing — and treating that
// as an error would block the object forever over a machine that is provably
// not running.
func (r *Reconciler) terminate(ctx context.Context, id string) (
	gone, notFound bool, err error) {

	out, err := r.EC2.TerminateInstances(ctx,
		&awsec2.TerminateInstancesInput{InstanceIds: []string{id}})
	if err != nil {
		if ec2.IsGone(err) {
			return true, true, nil
		}
		return false, false, err
	}

	for _, ch := range out.TerminatingInstances {
		if ch.CurrentState == nil {
			continue
		}
		if ec2.IsTerminated(ch.CurrentState.Name) {
			return true, false, nil
		}
	}
	return false, false, nil
}

// holdDeleting keeps the finalizer, says why, and looks again soon.
func (r *Reconciler) holdDeleting(ctx context.Context,
	machine *awsv1a1.AWSMachine, message string) (ctrl.Result, error) {

	if err := r.markNotReady(ctx, machine, awsv1a1.ReasonDeleting,
		message); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: bootRequeue}, nil
}

// releaseFinalizer removes the finalizer so the object can go.
func (r *Reconciler) releaseFinalizer(ctx context.Context,
	machine *awsv1a1.AWSMachine) error {

	patch := client.MergeFromWithOptions(machine.DeepCopy(),
		client.MergeFromWithOptimisticLock{})
	controllerutil.RemoveFinalizer(machine, awsv1a1.Finalizer)
	if err := r.Patch(ctx, machine, patch); err != nil {
		return fmt.Errorf("removing the finalizer: %w", err)
	}
	return nil
}
