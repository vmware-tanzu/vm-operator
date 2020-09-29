// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

// VirtualMachineContext is the context used for VirtualMachineControllers.
type VirtualMachineContext struct {
	context.Context
	VM          *vmopv1.VirtualMachine
	VMObjectKey client.ObjectKey
}

func (v *VirtualMachineContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VM.GroupVersionKind(), v.VM.Namespace, v.VM.Name)
}
