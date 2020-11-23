// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"

	"github.com/go-logr/logr"
	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

// ProbeContext is the context used for VM Probes.
type ProbeContext struct {
	context.Context
	Logger    logr.Logger
	VM        *vmopv1alpha1.VirtualMachine
	ProbeType string
	ProbeSpec *vmopv1alpha1.Probe
}

// String returns probe type.
func (p *ProbeContext) String() string {
	return p.ProbeType
}
