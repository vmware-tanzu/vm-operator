// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/vmware/govmomi/object"
)

// BackupVirtualMachineContext is the context used for storing backup data of VM
// and its related objects.
type BackupVirtualMachineContext struct {
	VMCtx         VirtualMachineContext
	VcVM          *object.VirtualMachine
	BootstrapData map[string]string
	DiskUUIDToPVC map[string]corev1.PersistentVolumeClaim
}

func (c *BackupVirtualMachineContext) String() string {
	return fmt.Sprintf("Backup %s", c.VMCtx.String())
}
