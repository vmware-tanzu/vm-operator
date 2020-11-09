// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
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

	// Logger is the webhook's logger.
	Logger logr.Logger

	// Recorder is used to record events.
	Recorder record.Recorder
}

// String returns WebhookName.
func (c *WebhookContext) String() string {
	return fmt.Sprintf("%s/%s", c.Name, c.Name)
}
