// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// WebConsoleRequestContext is the context used for WebConsoleRequestControllers.
type WebConsoleRequestContext struct {
	context.Context
	Logger            logr.Logger
	WebConsoleRequest *vmopv1.WebConsoleRequest
	VM                *vmopv1.VirtualMachine
}

func (v *WebConsoleRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.WebConsoleRequest.GroupVersionKind(), v.WebConsoleRequest.Namespace, v.WebConsoleRequest.Name)
}
