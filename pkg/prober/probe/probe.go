// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"context"
	"time"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	proberctx "github.com/vmware-tanzu/vm-operator/pkg/prober/context"
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
	Probe(ctx *proberctx.ProbeContext) (Result, error)
}

// Probing related provider methods.
type vmProviderGuestHeartbeatProber interface {
	GetVirtualMachineGuestHeartbeat(ctx context.Context, vm *vmopv1.VirtualMachine) (vmopv1.GuestHeartbeatStatus, error)
}
type vmProviderGuestInfoProber interface {
	GetVirtualMachineProperties(ctx context.Context, vm *vmopv1.VirtualMachine, propertyPaths []string) (map[string]any, error)
}
type vmProviderProber interface {
	vmProviderGuestHeartbeatProber
	vmProviderGuestInfoProber
}

// Prober contains the different type of probes.
type Prober struct {
	TCPProbe       Probe
	GuestHeartbeat Probe
	GuestInfo      Probe
}

// NewProber creates a new Prober.
func NewProber(vmProvider vmProviderProber) *Prober {
	return &Prober{
		TCPProbe:       NewTCPProber(),
		GuestHeartbeat: NewGuestHeartbeatProber(vmProvider),
		GuestInfo:      NewGuestInfoProber(vmProvider),
	}
}
