// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// NewWebhookRequestContext returns the context for a single admission
// request along with a function that releases its resources. The returned
// function must be called once the request has been handled.
//
// The request context carries the values of the webhook's long-lived
// context, ex. the configuration, logger, and other values set on the
// controller-manager context. It is canceled, with the same cause, when
// reqCtx is done and honors the deadline of reqCtx, so downstream operations
// do not outlive the admission request that started them. Values carried by
// reqCtx itself are not propagated.
//
// The returned context is owned by the request, so a handler may replace its
// context.Context without affecting the webhook or any other request.
func NewWebhookRequestContext(
	reqCtx context.Context,
	webhookCtx *pkgctx.WebhookContext,
	req admission.Request,
	obj, oldObj *unstructured.Unstructured) (*pkgctx.WebhookRequestContext, context.CancelFunc) {

	ctx, cancel := requestScopedContext(webhookCtx.Context, reqCtx)

	logObj := obj
	if logObj == nil {
		logObj = oldObj
	}

	return &pkgctx.WebhookRequestContext{
		Context:             ctx,
		WebhookContext:      webhookCtx,
		Op:                  req.Operation,
		Obj:                 obj,
		RawObj:              req.Object.Raw,
		OldObj:              oldObj,
		UserInfo:            req.UserInfo,
		IsPrivilegedAccount: IsPrivilegedAccount(webhookCtx, req.UserInfo),
		IsVMOperatorAccount: IsVMOperatorServiceAccount(webhookCtx, req.UserInfo),
		Logger: webhookCtx.Logger.
			WithName(logObj.GetNamespace()).
			WithName(logObj.GetName()),
	}, cancel
}

// requestScopedContext returns a context derived from parent that is also
// canceled when reqCtx is done and that honors the deadline of reqCtx.
func requestScopedContext(
	parent, reqCtx context.Context) (context.Context, context.CancelFunc) {

	ctx, cancelCause := context.WithCancelCause(parent)
	stop := context.AfterFunc(reqCtx, func() {
		cancelCause(context.Cause(reqCtx))
	})

	cancelDeadline := context.CancelFunc(func() {})
	if deadline, ok := reqCtx.Deadline(); ok {
		ctx, cancelDeadline = context.WithDeadline(ctx, deadline)
	}

	return ctx, func() {
		stop()
		cancelDeadline()
		cancelCause(context.Canceled)
	}
}
