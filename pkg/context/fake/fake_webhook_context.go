// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	clientrecord "k8s.io/client-go/tools/record"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// NewWebhookContext returns a fake WebhookContext for unit testing
// webhooks with a fake client.
func NewWebhookContext(ctx *context.ControllerManagerContext) *context.WebhookContext {
	return &context.WebhookContext{
		Context:  ctx,
		Name:     WebhookName,
		Logger:   ctx.Logger.WithName(WebhookName),
		Recorder: record.New(clientrecord.NewFakeRecorder(1024)),
	}
}
