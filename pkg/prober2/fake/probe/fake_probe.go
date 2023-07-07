// Copyright (c) 2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package probe

import (
	"sync"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/probe"
)

type funcs struct {
	ProbeFn func(ctx *context.ProbeContext) (probe.Result, error)
}

type FakeProbe struct {
	sync.Mutex
	funcs
}

func NewFakeProbe() probe.Probe {
	return &FakeProbe{}
}

func (p *FakeProbe) Probe(ctx *context.ProbeContext) (probe.Result, error) {
	p.Lock()
	defer p.Unlock()

	if p.ProbeFn != nil {
		return p.ProbeFn(ctx)
	}

	return probe.Unknown, nil
}
