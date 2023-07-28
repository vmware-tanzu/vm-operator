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

// VirtualMachineServiceContext is the context used for VirtualMachineServiceController.
type VirtualMachineServiceContext struct {
	context.Context
	Logger    logr.Logger
	VMService *v1alpha1.VirtualMachineService
}

func (v *VirtualMachineServiceContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMService.GroupVersionKind(), v.VMService.Namespace, v.VMService.Name)
}

// VirtualMachineServiceContextA2 is the context used for VirtualMachineServiceController.
type VirtualMachineServiceContextA2 struct {
	context.Context
	Logger    logr.Logger
	VMService *vmopv1.VirtualMachineService
}

func (v *VirtualMachineServiceContextA2) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMService.GroupVersionKind(), v.VMService.Namespace, v.VMService.Name)
}
