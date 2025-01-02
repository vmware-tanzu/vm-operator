// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WebhookRequestContext is a Go context used with a webhook request.
type WebhookRequestContext struct {
	// WebhookContext is the context of the webhook that spawned this request context.
	*WebhookContext

	// Obj is the resource associated with the webhook request.
	Obj *unstructured.Unstructured

	// RawObj is the raw object from the webhook request.
	RawObj []byte

	// OldObj is set only for Update requests.
	OldObj *unstructured.Unstructured

	// Operation is the operation.
	Op admissionv1.Operation

	// IsPrivilegedAccount is if this request is from a privileged account (currently
	// that's either kube-admin or the pod's system account).
	IsPrivilegedAccount bool

	// UserInfo is the user information associated with the webhook request.
	UserInfo authv1.UserInfo

	// Logger is the logger associated with the webhook request.
	Logger logr.Logger
}

// String returns Obj.GroupVersionKind Obj.Namespace/Obj.Name.
func (c *WebhookRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s", c.Obj.GroupVersionKind(), c.Obj.GetNamespace(), c.Obj.GetName())
}
