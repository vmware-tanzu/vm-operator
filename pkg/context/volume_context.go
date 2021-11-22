// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	vmopv1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

// VolumeContext is the context used for VolumeController.
type VolumeContext struct {
	context.Context
	Logger                    logr.Logger
	VM                        *vmopv1.VirtualMachine
	InstanceStorageFSSEnabled bool
}

func (v *VolumeContext) String() string {
	return fmt.Sprintf("%s %s/%s", v.VM.GroupVersionKind(), v.VM.Namespace, v.VM.Name)
}
