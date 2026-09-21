// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package awsmachine

import (
	"context"

	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/ec2"
	"github.com/vmware-tanzu/vm-operator/external/kubevm-provider-aws/internal/intent"
)

// applyPower starts or stops the instance to match the power state the
// parent asks for, and reports what it did.
//
// Read from the PARENT every reconcile, not from this object's own spec.
// Reading the persisted copy would make the machine follow whatever was
// written last rather than what the user is asking for now, and an edit to the
// portable object would silently stop propagating.
func (r *Reconciler) applyPower(ctx context.Context, id string,
	state ec2types.InstanceStateName, want *intent.Intent) stepResult {

	// Nothing to do while the instance is moving. Issuing Start against a
	// stopping instance is rejected, and issuing it repeatedly is how a
	// controller ends up hammering an API for no reason.
	if ec2.IsTransitional(state) || ec2.IsTerminated(state) {
		return stepResult{}
	}

	switch want.PowerState {
	case kubevmv1a1.PowerStateOn:
		if state == ec2types.InstanceStateNameRunning {
			return stepResult{}
		}
		_, err := r.EC2.StartInstances(ctx,
			&awsec2.StartInstancesInput{InstanceIds: []string{id}})
		return stepResult{acted: true, operation: "StartInstances", err: err}

	case kubevmv1a1.PowerStateOff:
		if state == ec2types.InstanceStateNameStopped {
			return stepResult{}
		}
		_, err := r.EC2.StopInstances(ctx,
			&awsec2.StopInstancesInput{InstanceIds: []string{id}})
		return stepResult{acted: true, operation: "StopInstances", err: err}
	}

	// Suspended does nothing here. EC2 has no suspended state -- hibernation
	// lands in stopped and must be enabled at launch -- so the request is
	// reported as unsupported by intent, not approximated by a stop that
	// would look like success. It is reported through UpToDate instead, by
	// internal/intent, alongside every other request this platform cannot
	// meet.
	return stepResult{}
}
