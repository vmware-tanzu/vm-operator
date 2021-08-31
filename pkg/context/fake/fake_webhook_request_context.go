// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// NewWebhookRequestContext returns a fake WebhookRequestContext for unit
// testing webhooks with a fake client.
func NewWebhookRequestContext(ctx *context.WebhookContext, obj, oldObj *unstructured.Unstructured) *context.WebhookRequestContext {
	return &context.WebhookRequestContext{
		WebhookContext: ctx,
		Obj:            obj,
		OldObj:         oldObj,
		Logger:         ctx.Logger.WithName(obj.GetName()),
	}
}
