// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Every condition this provider writes is built in this file, and every write
// of one goes through it.
//
// They were spread across the files that happened to need them, and two of
// them drifted: the same observable fact, an instance that is gone, was
// reported under two different reasons. A condition is the only account an
// object gives of itself, so keeping the vocabulary in one place is not
// tidiness.

package awsmachine

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/redact"
)

// markNotReady records why nothing is happening and persists it.
//
// Every refusal path goes through here, so an object is never merely idle: a
// user looking at one can always tell what it is waiting for. "Nothing
// happened" and "nothing happened for this reason" look identical from
// outside unless somebody writes the reason down.
func (r *Reconciler) markNotReady(ctx context.Context,
	machine *awsv1a1.AWSMachine, reason, message string) error {

	base := machine.DeepCopy()
	setCond(machine, metav1.Condition{
		Type:    awsv1a1.ConditionInfrastructureReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	})
	// UpToDate alongside it, always. This path runs before the parent's
	// intent has been evaluated, or after the instance has gone, so the
	// honest answer is "not observed" -- and leaving the previous value
	// standing would have the object claim a drift report about a machine
	// that no longer exists.
	setCond(machine, metav1.Condition{
		Type:   awsv1a1.ConditionUpToDate,
		Status: metav1.ConditionUnknown,
		Reason: awsv1a1.ReasonNotObserved,
		Message: "what the portable object asks for has not been compared " +
			"with the machine",
	})
	// An object deleted mid-reconcile cannot be told why it is not ready,
	// and does not need to be.
	return client.IgnoreNotFound(r.patchStatusIfChanged(ctx, machine, base))
}

// setCondition writes one condition and the observed generation.
func (r *Reconciler) setCondition(ctx context.Context,
	machine *awsv1a1.AWSMachine, c metav1.Condition) error {

	base := machine.DeepCopy()
	setCond(machine, c)
	return r.patchStatusIfChanged(ctx, machine, base)
}

// setCond sets one condition on the object in memory.
func setCond(machine *awsv1a1.AWSMachine, c metav1.Condition) {
	c.ObservedGeneration = machine.Generation
	meta.SetStatusCondition(&machine.Status.Conditions, c)
	machine.Status.ObservedGeneration = machine.Generation
}

// ready returns a true InfrastructureReady condition.
func ready(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    awsv1a1.ConditionInfrastructureReady,
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	}
}

// notReady returns a false InfrastructureReady condition.
func notReady(reason, message string) metav1.Condition {
	return metav1.Condition{
		Type:    awsv1a1.ConditionInfrastructureReady,
		Status:  metav1.ConditionFalse,
		Reason:  reason,
		Message: message,
	}
}

// failureCondition turns an EC2 error into an InfrastructureReady condition
// and says whether to retry soon.
//
// The message passes through redaction first, so a platform error cannot
// carry a credential into a condition, an event or a log -- while still
// naming the operation and the platform's own code, because an operator who
// cannot see "UnauthorizedOperation" cannot tell which permission to add.
func failureCondition(operation string, err error) (metav1.Condition, bool) {
	message := redact.Redact(ec2.Describe(operation, err))

	switch ec2.Classify(err) {
	case ec2.Denied:
		return notReady(awsv1a1.ReasonUnauthorized, message), false

	case ec2.Retryable:
		reason := awsv1a1.ReasonProvisioning
		if ec2.Code(err) == "InsufficientInstanceCapacity" {
			reason = awsv1a1.ReasonWaitingForCapacity
		}
		return notReady(reason, message), true

	case ec2.Gone:
		// Not a configuration error: the instance existed and does not any
		// more. An operator reading InvalidConfiguration here would go
		// looking for a typo that is not there.
		return notReady(awsv1a1.ReasonInstanceGone, message), false
	}

	// Terminal stops here. Retrying a typo forever burns quota and fills
	// logs.
	return notReady(awsv1a1.ReasonInvalidConfiguration, message), false
}

// upToDate returns the UpToDate condition for what could not be applied.
func upToDate(unsupported []string) metav1.Condition {
	if len(unsupported) == 0 {
		return metav1.Condition{
			Type:    awsv1a1.ConditionUpToDate,
			Status:  metav1.ConditionTrue,
			Reason:  awsv1a1.ReasonRunning,
			Message: "everything requested has been applied",
		}
	}

	message := unsupported[0]
	if len(unsupported) > 1 {
		message = fmt.Sprintf("%s (and %d more)", message,
			len(unsupported)-1)
	}
	return metav1.Condition{
		Type:    awsv1a1.ConditionUpToDate,
		Status:  metav1.ConditionFalse,
		Reason:  awsv1a1.ReasonUnsupportedByProvider,
		Message: message,
	}
}
