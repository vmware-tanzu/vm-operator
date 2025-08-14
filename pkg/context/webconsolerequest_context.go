// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// WebConsoleRequestContext is the context used for WebConsoleRequestControllers.
type WebConsoleRequestContext struct {
	context.Context
	Logger            logr.Logger
	WebConsoleRequest *v1alpha1.WebConsoleRequest
	VM                *v1alpha1.VirtualMachine
}

func (v *WebConsoleRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.WebConsoleRequest.GroupVersionKind(), v.WebConsoleRequest.Namespace, v.WebConsoleRequest.Name)
}

// WebConsoleRequestContextV1 is the context used for WebConsoleRequestControllers.
type WebConsoleRequestContextV1 struct {
	context.Context
	Logger            logr.Logger
	WebConsoleRequest *vmopv1.VirtualMachineWebConsoleRequest
	VM                *vmopv1.VirtualMachine
}

func (v *WebConsoleRequestContextV1) String() string {
	return fmt.Sprintf("%s %s/%s", v.WebConsoleRequest.GroupVersionKind(), v.WebConsoleRequest.Namespace, v.WebConsoleRequest.Name)
}
