// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

// VirtualMachineClassContext is the context used for VirtualMachineClassControllers.
type VirtualMachineClassContext struct {
	context.Context
	Logger  logr.Logger
	VMClass *vmopv1.VirtualMachineClass
}

func (v *VirtualMachineClassContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMClass.GroupVersionKind(), v.VMClass.Namespace, v.VMClass.Name)
}
