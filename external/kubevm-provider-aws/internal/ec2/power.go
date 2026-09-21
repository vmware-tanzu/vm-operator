// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package ec2

import (
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	kubevmv1a1 "github.com/vmware-tanzu/vm-operator/external/kubevm/api/v1alpha1"
)

// PowerStateFor maps an EC2 instance state onto the portable vocabulary,
// reporting false when there is no portable equivalent.
//
// The boolean is the whole point of this function. EC2 has six states and the
// portable API has three, and the four that do not correspond must leave
// status.powerState ABSENT rather than set to a guess. An absent path is
// explicitly not an error under the contract; a guessed one produces a UI that
// flickers between values the machine was never in.
//
// Suspended is unreachable from any EC2 state -- see finding F12. Hibernation
// is not a distinct state: a hibernated instance reports stopped, and
// hibernation has to be enabled at launch.
func PowerStateFor(s ec2types.InstanceStateName) (kubevmv1a1.PowerState, bool) {
	switch s {
	case ec2types.InstanceStateNameRunning:
		return kubevmv1a1.PowerStateOn, true
	case ec2types.InstanceStateNameStopped:
		return kubevmv1a1.PowerStateOff, true
	}
	// pending, stopping, shutting-down, terminated.
	return "", false
}

// IsTransitional reports whether a state is one the instance is passing
// through rather than resting in.
//
// Distinct from PowerStateFor's boolean because terminated is also
// unreportable but is not transitional -- nothing further happens to a
// terminated instance, and a caller waiting for one to settle would wait
// forever.
func IsTransitional(s ec2types.InstanceStateName) bool {
	switch s {
	case ec2types.InstanceStateNamePending,
		ec2types.InstanceStateNameStopping,
		ec2types.InstanceStateNameShuttingDown:
		return true
	}
	return false
}

// IsTerminated reports whether an instance has reached its final state.
func IsTerminated(s ec2types.InstanceStateName) bool {
	return s == ec2types.InstanceStateNameTerminated
}
