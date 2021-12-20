// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WebhookRequestContext is a Go context used with a webhook request.
type WebhookRequestContext struct {
	// WebhookContext is the context of the webhook that spawned this request context.
	*WebhookContext

	// Obj is the resource associated with the webhook request.
	Obj *unstructured.Unstructured

	// OldObj is set only for Update requests.
	OldObj *unstructured.Unstructured

	// IsPrivilegedAccount is if this request is from a privileged account (currently
	// that's either kube-admin or the pod's system account).
	IsPrivilegedAccount bool

	// Logger is the logger associated with the webhook request.
	Logger logr.Logger
}

// String returns Obj.GroupVersionKind Obj.Namespace/Obj.Name.
func (c *WebhookRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s", c.Obj.GroupVersionKind(), c.Obj.GetNamespace(), c.Obj.GetName())
}
