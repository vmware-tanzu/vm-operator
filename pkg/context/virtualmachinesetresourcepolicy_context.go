// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
)

// VirtualMachineSetResourcePolicyContext is the context used for VirtualMachineControllers.
type VirtualMachineSetResourcePolicyContext struct {
	context.Context
	Logger         logr.Logger
	ResourcePolicy *vmopv1alpha1.VirtualMachineSetResourcePolicy
}

func (v *VirtualMachineSetResourcePolicyContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.ResourcePolicy.GroupVersionKind(), v.ResourcePolicy.Namespace, v.ResourcePolicy.Name)
}
