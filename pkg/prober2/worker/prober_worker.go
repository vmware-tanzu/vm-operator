// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"k8s.io/client-go/util/workqueue"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/pkg/prober2/context"
	"github.com/vmware-tanzu/vm-operator/pkg/prober2/probe"
)

// Worker represents a prober worker interface.
type Worker interface {
	GetQueue() workqueue.DelayingInterface
	CreateProbeContext(vm *vmopv1.VirtualMachine) (*context.ProbeContext, error)
	DoProbe(ctx *context.ProbeContext) error
	ProcessProbeResult(ctx *context.ProbeContext, res probe.Result, resErr error) error
}
