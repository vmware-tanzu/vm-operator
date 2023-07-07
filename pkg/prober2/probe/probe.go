// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	goctx "context"
	"time"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
)

type Result int

const (
	// Failure is encoded as -1 (type Result).
	Failure Result = iota - 1
	// Unknown is encoded as 0 (type Result).
	Unknown
	// Success is encoded as 1 (type Result).
	Success

	defaultConnectTimeout = 10 * time.Second
)

// Probe is the interface to execute VM probes.
type Probe interface {
	Probe(ctx *context.ProbeContext) (Result, error)
}

// Probing related provider methods.
type vmProviderProber interface {
	GetVirtualMachineGuestHeartbeat(ctx goctx.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
}

// Prober contains the different type of probes.
type Prober struct {
	TCPProbe       Probe
	GuestHeartbeat Probe
}

// NewProber creates a new Prober.
func NewProber(vmProviderProber vmProviderProber) *Prober {
	return &Prober{
		TCPProbe:       NewTCPProber(),
		GuestHeartbeat: NewGuestHeartbeatProber(vmProviderProber),
	}
}
