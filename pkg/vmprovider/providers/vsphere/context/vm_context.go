// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"

	vmopv1alpha1 "github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
)

type VMContext struct {
	context.Context
	Logger logr.Logger
	VM     *vmopv1alpha1.VirtualMachine
}

type VMCloneContext struct {
	VMContext
	ResourcePool        *object.ResourcePool
	Folder              *object.Folder
	StorageProvisioning string
}
