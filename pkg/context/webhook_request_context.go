// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	"github.com/go-logr/logr"
	authv1 "k8s.io/api/authentication/v1"
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

	// Logger is the logger associated with the webhook request.
	Logger logr.Logger

	// UserInfo is the user information associated with the webhook request
	*authv1.UserInfo
}

// String returns Obj.GroupVersionKind Obj.Namespace/Obj.Name.
func (c *WebhookRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s", c.Obj.GroupVersionKind(), c.Obj.GetNamespace(), c.Obj.GetName())
}
