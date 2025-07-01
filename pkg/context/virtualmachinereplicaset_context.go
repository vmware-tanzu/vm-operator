// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
)

// VirtualMachineReplicaSetContext is the context used for VirtualMachineReplicaSetContext reconciliation.
type VirtualMachineReplicaSetContext struct {
	context.Context
	Logger     logr.Logger
	ReplicaSet *vmopv1.VirtualMachineReplicaSet
}

func (v *VirtualMachineReplicaSetContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.ReplicaSet.GroupVersionKind(), v.ReplicaSet.Namespace, v.ReplicaSet.Name)
}
