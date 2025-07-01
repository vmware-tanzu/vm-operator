// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// WebhookContext is the context of a webhook.
type WebhookContext struct {
	context.Context

	// Name is the name of the webhook.
	Name string

	// Namespace is the namespace the webhook is running in.
	Namespace string

	// ServiceAccountName is the service account name of the pod.
	ServiceAccountName string

	// Logger is the webhook's logger.
	Logger logr.Logger

	// Recorder is used to record events.
	Recorder record.Recorder
}

// String returns WebhookName.
func (c *WebhookContext) String() string {
	return fmt.Sprintf("%s/%s", c.Namespace, c.Name)
}
