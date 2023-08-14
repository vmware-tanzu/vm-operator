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

// VirtualMachineClassContext is the context used for VirtualMachineClassControllers.
type VirtualMachineClassContext struct {
	context.Context
	Logger  logr.Logger
	VMClass *v1alpha1.VirtualMachineClass
}

func (v *VirtualMachineClassContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMClass.GroupVersionKind(), v.VMClass.Namespace, v.VMClass.Name)
}

// VirtualMachineClassContextA2 is the context used for VirtualMachineClassControllers.
type VirtualMachineClassContextA2 struct {
	context.Context
	Logger  logr.Logger
	VMClass *vmopv1.VirtualMachineClass
}

func (v *VirtualMachineClassContextA2) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMClass.GroupVersionKind(), v.VMClass.Namespace, v.VMClass.Name)
}
