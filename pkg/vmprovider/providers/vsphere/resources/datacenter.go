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

type Datacenter struct {
	Datacenter *object.Datacenter
	name       string
}

// Look up a Datacenter with a given name. If success, initialize and return a resources.Datacenter, error otherwise.
func NewDatacenter(ctx context.Context, finder *find.Finder, dsName string) (*Datacenter, error) {
	dc, err := finder.Datacenter(ctx, dsName)
	if err != nil {
		glog.Errorf("No Datacenter with name: %s found.", dsName)
		return nil, err
	}

	return &Datacenter{
		name:       dsName,
		Datacenter: dc,
	}, nil
}

func (dc *Datacenter) ListVms(ctx context.Context, finder *find.Finder, path string) ([]*object.VirtualMachine, error) {
	glog.Infof("Listing VMs for folder: %s", dc.name)

	vms, err := finder.VirtualMachineList(ctx, path)
	if err != nil {
		switch err.(type) {
		case *find.NotFoundError:
			glog.Infof("No Vms were found")
			return vms, nil
		default:
			return nil, err
		}
	}

	return vms, nil
}
