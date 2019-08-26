/* **********************************************************
 * Copyright 2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"
	"fmt"

	"k8s.io/klog"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
)

//GetResourcePoolOwner - Get owner (i.e. parent cluster) of the resource pool
func GetResourcePoolOwner(ctx context.Context, rp *object.ResourcePool) (*object.ClusterComputeResource, error) {
	var owner mo.ResourcePool
	err := rp.Properties(ctx, rp.Reference(), []string{"owner"}, &owner)
	if err != nil {
		klog.Errorf("Failed to retrieve owner of %v: %v", rp.Reference(), err)
		return nil, err
	}
	if owner.Owner.Type != "ClusterComputeResource" {
		return nil, fmt.Errorf("owner of the ResourcePool is not a cluster")
	}
	return object.NewClusterComputeResource(rp.Client(), owner.Owner), nil
}
