// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	clientrecord "k8s.io/client-go/tools/record"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// NewWebhookContext returns a fake WebhookContext for unit testing
// webhooks with a fake client.
func NewWebhookContext(ctx *pkgctx.ControllerManagerContext) *pkgctx.WebhookContext {
	return &pkgctx.WebhookContext{
		Context:   ctx,
		Name:      WebhookName,
		Namespace: ctx.Namespace,
		Logger:    ctx.Logger.WithName(WebhookName),
		Recorder:  record.New(clientrecord.NewFakeRecorder(1024)),
	}
}
