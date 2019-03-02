/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"

	"github.com/golang/glog"

	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/session"
)

// TODO(bryanv) vmFolder isn't used?
func (v *VSphereManager) ListVms(ctx context.Context, sc *session.SessionContext, vmFolder string) ([]*resources.VirtualMachine, error) {
	glog.Info("Listing VMs")

	var vms []*resources.VirtualMachine
	rc := v.ResourceContext

	list, err := rc.Datacenter.ListVms(ctx, sc.Finder, "*")
	if err != nil {
		glog.Infof("Failed to list Vms: %s", err)
		return nil, err
	}

	for _, vmiter := range list {
		glog.Infof("Found VM: %s %s %s", vmiter.Name(), vmiter.Reference().Type, vmiter.Reference().Value)
		vm, err := v.LookupVm(ctx, sc, vmiter.Name())
		if err == nil {
			glog.Infof("Append VM: %s", vm.Name)
			vms = append(vms, vm)
		}
	}

	return vms, nil
}
