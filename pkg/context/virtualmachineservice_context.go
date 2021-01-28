// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

// VirtualMachineServiceContext is the context used for VirtualMachineServiceController.
type VirtualMachineServiceContext struct {
	context.Context
	Logger    logr.Logger
	VMService *vmopv1.VirtualMachineService
}

func (v *VirtualMachineServiceContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMService.GroupVersionKind(), v.VMService.Namespace, v.VMService.Name)
}
