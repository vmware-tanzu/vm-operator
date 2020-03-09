// Copyright (c) 2019 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/pkg/record"
)

// ControllerContext is the context of a controller.
type ControllerContext struct {
	*ControllerManagerContext

	// Name is the name of the controller.
	Name string

	// Logger is the controller's logger.
	Logger logr.Logger

	// Recorder is used to record events.
	Recorder record.Recorder
}

// String returns ControllerManagerName/ControllerName.
func (c *ControllerContext) String() string {
	return fmt.Sprintf("%s/%s", c.ControllerManagerContext.String(), c.Name)
}
