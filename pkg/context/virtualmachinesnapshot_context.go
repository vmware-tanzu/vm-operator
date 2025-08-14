// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
)

// VirtualMachineSnapshotContext is the context used for VirtualMachineSnapShotControllers.
type VirtualMachineSnapshotContext struct {
	context.Context
	Logger                 logr.Logger
	VirtualMachineSnapshot *vmopv1.VirtualMachineSnapshot
	VM                     *vmopv1.VirtualMachine
	StorageClassesToSync   sets.Set[string]
}

func (v *VirtualMachineSnapshotContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VirtualMachineSnapshot.GroupVersionKind(), v.VirtualMachineSnapshot.Namespace, v.VirtualMachineSnapshot.Name)
}
