// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	clientrecord "k8s.io/client-go/tools/record"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// NewControllerContext returns a fake ControllerContext for unit testing
// reconcilers with a fake client.
func NewControllerContext(ctx *context.ControllerManagerContext) *context.ControllerContext {
	return &context.ControllerContext{
		ControllerManagerContext: ctx,
		Name:                     ControllerName,
		Logger:                   ctx.Logger.WithName(ControllerName),
		Recorder:                 record.New(clientrecord.NewFakeRecorder(1024)),
	}
}
