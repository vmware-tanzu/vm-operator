// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"fmt"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
)

type guestHeartbeatProber struct {
	prober vmProviderProber
}

func NewGuestHeartbeatProber(vmProviderProber vmProviderProber) Probe {
	return &guestHeartbeatProber{
		prober: vmProviderProber,
	}
}

func heartbeatValue(value vmopv1.GuestHeartbeatStatus) int {
	switch value {
	case vmopv1.GreenHeartbeatStatus:
		return 1
	case vmopv1.YellowHeartbeatStatus:
		return 0
	default: // vmopv1.RedHeartbeatStatus, vmopv1.GrayHeartbeatStatus
		return -1
	}
}

func (hbp guestHeartbeatProber) Probe(ctx *context.ProbeContext) (Result, error) {
	heartbeat, err := hbp.prober.GetVirtualMachineGuestHeartbeat(ctx, ctx.VM)
	if err != nil {
		return Unknown, err
	}

	if heartbeat == "" {
		return Unknown, fmt.Errorf("no heartbeat value")
	}

	if heartbeatValue(heartbeat) < heartbeatValue(ctx.VM.Spec.ReadinessProbe.GuestHeartbeat.ThresholdStatus) {
		return Failure, fmt.Errorf("heartbeat status %q is below threshold", heartbeat)
	}

	return Success, nil
}
