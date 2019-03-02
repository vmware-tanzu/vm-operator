/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package resources

import (
	"context"

	"github.com/golang/glog"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
)

type ResourcePool struct {
	Name         string
	ResourcePool *object.ResourcePool
}

// Lookup a ResourcePool with a given name. If success, return a resources.ResourcePool, error otherwise.
func NewResourcePool(ctx context.Context, finder *find.Finder, rpName string) (*ResourcePool, error) {
	rp, err := finder.ResourcePoolOrDefault(ctx, rpName)
	if err != nil {
		glog.Errorf("No ResourcePool with name: [%s] found. Error: [%s]", rpName, err)
		return nil, err
	}

	return &ResourcePool{
		Name:         rpName,
		ResourcePool: rp,
	}, nil
}
