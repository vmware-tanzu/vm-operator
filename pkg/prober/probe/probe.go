// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"time"

	"github.com/vmware-tanzu/vm-operator/pkg/prober/context"
)

type Result int

const (
	// Failure is encoded as -1 (type Result)
	Failure Result = iota - 1
	// Unknown is encoded as 0 (type Result)
	Unknown
	// Success is encoded as 1 (type Result)
	Success

	defaultConnectTimeout = 10 * time.Second
)

// Probe is the interface to execute VM probes.
type Probe interface {
	Probe(ctx *context.ProbeContext) (Result, error)
}

// Prober contains different type of probes, currently we only have tcp probe.
type Prober struct {
	TCPProbe Probe
}

// NewProber creates a new prober.
func NewProber() *Prober {
	return &Prober{
		TCPProbe: NewTcpProber(),
	}
}
