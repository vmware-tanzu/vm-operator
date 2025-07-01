// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/object"
)

// GetVMClusterComputeResource returns the VM's ClusterComputeResource.
func GetVMClusterComputeResource(
	ctx context.Context,
	vcVM *object.VirtualMachine) (*object.ClusterComputeResource, error) {

	rp, err := vcVM.ResourcePool(ctx)
	if err != nil {
		return nil, err
	}

	ccrRef, err := rp.Owner(ctx)
	if err != nil {
		return nil, err
	}

	cluster, ok := ccrRef.(*object.ClusterComputeResource)
	if !ok {
		return nil, fmt.Errorf("VM Owner is not a ClusterComputeResource but %T", ccrRef)
	}

	return cluster, nil
}
