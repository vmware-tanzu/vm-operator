// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
)

// VirtualMachineSetResourcePolicyContext is the context used for VirtualMachineControllers.
type VirtualMachineSetResourcePolicyContext struct {
	context.Context
	Logger         logr.Logger
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
}

func (v *VirtualMachineSetResourcePolicyContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.ResourcePolicy.GroupVersionKind(), v.ResourcePolicy.Namespace, v.ResourcePolicy.Name)
}
