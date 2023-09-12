// Copyright (c) 2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
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

// WebConsoleRequestContextA2 is the context used for WebConsoleRequestControllers.
type WebConsoleRequestContextA2 struct {
	context.Context
	Logger            logr.Logger
	WebConsoleRequest *vmopv1.VirtualMachineWebConsoleRequest
	VM                *vmopv1.VirtualMachine
}

func (v *WebConsoleRequestContextA2) String() string {
	return fmt.Sprintf("%s %s/%s", v.WebConsoleRequest.GroupVersionKind(), v.WebConsoleRequest.Namespace, v.WebConsoleRequest.Name)
}
