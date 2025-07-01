// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// NewWebhookRequestContext returns a fake WebhookRequestContext for unit
// testing webhooks with a fake client.
func NewWebhookRequestContext(ctx *pkgctx.WebhookContext, obj, oldObj *unstructured.Unstructured) *pkgctx.WebhookRequestContext {
	return &pkgctx.WebhookRequestContext{
		WebhookContext: ctx,
		Obj:            obj,
		OldObj:         oldObj,
		Logger:         ctx.Logger.WithName(obj.GetName()),
	}
}
