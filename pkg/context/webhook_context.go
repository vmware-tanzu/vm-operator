// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// WebhookContext is the context of a webhook.
type WebhookContext struct {
	// ControllerManagerContext is the context of the controller manager to
	// which this webhook belongs.
	*ControllerManagerContext

	// Name is the name of the webhook.
	Name string

	// Logger is the webhook's logger.
	Logger logr.Logger

	// Recorder is used to record events.
	Recorder record.Recorder
}

// String returns ControllerManagerName/WebhookName.
func (c *WebhookContext) String() string {
	return fmt.Sprintf("%s/%s", c.ControllerManagerContext.String(), c.Name)
}
