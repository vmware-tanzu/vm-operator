// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// VirtualMachineGroupContext is the context used for VirtualMachineGroupControllers.
type VirtualMachineGroupContext struct {
	context.Context
	Logger  logr.Logger
	VMGroup *vmopv1.VirtualMachineGroup
}

func (v *VirtualMachineGroupContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VMGroup.GroupVersionKind(), v.VMGroup.Namespace, v.VMGroup.Name)
}
