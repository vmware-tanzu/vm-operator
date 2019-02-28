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

func (v *VSphereManager) LookupVm(ctx context.Context, sc *session.SessionContext, vmName string) (*resources.VirtualMachine, error) {
	glog.Info("Lookup VM")

	vm, err := resources.NewVM(ctx, sc.Finder, vmName)
	if err != nil {
		return nil, err
	}

	glog.Infof("vm: %s path: %s", vm.Name, vm.VirtualMachine.InventoryPath)

	return vm, nil
}
