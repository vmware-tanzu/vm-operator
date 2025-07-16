// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// VirtualMachineGroupPublishRequestContext is the context used for the
// VirtualMachineGroupPublishRequest controller.
type VirtualMachineGroupPublishRequestContext struct {
	context.Context
	Logger logr.Logger
	Obj    *vmopv1.VirtualMachineGroupPublishRequest
}

func (v VirtualMachineGroupPublishRequestContext) String() string {
	return fmt.Sprintf("%s %s/%s",
		v.Obj.GroupVersionKind(),
		v.Obj.Namespace,
		v.Obj.Name)
}
