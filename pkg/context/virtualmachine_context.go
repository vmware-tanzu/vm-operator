// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
)

// VirtualMachineContext is the context used for VirtualMachineControllers.
type VirtualMachineContext struct {
	context.Context
	Logger logr.Logger
	VM     *v1alpha1.VirtualMachine
}

func (v *VirtualMachineContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VM.GroupVersionKind(), v.VM.Namespace, v.VM.Name)
}

// VirtualMachineContextA2 is the context used for VirtualMachineControllers.
type VirtualMachineContextA2 struct {
	context.Context
	Logger logr.Logger
	VM     *vmopv1.VirtualMachine
}

func (v *VirtualMachineContextA2) String() string {
	return fmt.Sprintf("%s %s/%s", v.VM.GroupVersionKind(), v.VM.Namespace, v.VM.Name)
}
