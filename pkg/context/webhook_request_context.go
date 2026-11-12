// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WebhookRequestContext is a Go context used with a webhook request.
//
// The embedded context.Context is owned by this request. Handlers may replace
// it, ex. to add request-scoped values, without affecting the webhook or any
// other request.
type WebhookRequestContext struct {
	// Context is the request-scoped context. It carries the values of the
	// webhook's long-lived context and is canceled when the admission request
	// is done.
	context.Context

	// WebhookContext is the context of the webhook that spawned this request
	// context. It is shared by all requests and must not be modified.
	WebhookContext *WebhookContext

	// Obj is the resource associated with the webhook request.
	Obj *unstructured.Unstructured

	// RawObj is the raw object from the webhook request.
	RawObj []byte

	// OldObj is set only for Update requests.
	OldObj *unstructured.Unstructured

	// Operation is the operation.
	Op admissionv1.Operation

	// IsPrivilegedAccount is if this request is from a privileged account, ex.
	// - kube-admin
	// - system:masters
	// - VM Op service account
	// - in the privileged account env var
	IsPrivilegedAccount bool

	// IsVMOperatorAccount is if this request is from VM Operator.
	IsVMOperatorAccount bool

	// UserInfo is the user information associated with the webhook request.
	UserInfo authv1.UserInfo

	// Logger is the logger associated with the webhook request.
	Logger logr.Logger
}

// String returns Obj.GroupVersionKind Obj.Namespace/Obj.Name.
func (c *WebhookRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s", c.Obj.GroupVersionKind(), c.Obj.GetNamespace(), c.Obj.GetName())
}
