/* **********************************************************
 * Copyright 2018-2019 VMware, Inc.  All rights reserved. -- VMware Confidential
 * **********************************************************/

package vsphere

import (
	"context"

	"github.com/golang/glog"

	"vmware.com/kubevsphere/pkg/apis/vmoperator/v1alpha1"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/resources"
	"vmware.com/kubevsphere/pkg/vmprovider/providers/vsphere/session"
)

func (v *VSphereManager) DeleteVm(ctx context.Context, sc *session.SessionContext, vm v1alpha1.VirtualMachine) error {
	glog.Infof("DeleteVm %s", vm.Name)

	resVM, err := resources.NewVM(ctx, sc.Finder, vm.Name)
	if err != nil {
		glog.Errorf("Error retrieving VM: %s [%s]", vm.Name, err)
		return err
	}

	destroyTask, err := resVM.Delete(ctx)
	if err != nil {
		glog.Infof("Error while invoking delete VM task [%s]", err)
		return err
	}

	_, err = destroyTask.WaitForResult(ctx, nil)
	if err != nil {
		glog.Infof("VM delete task failed %s", err.Error())
		return err
	}

	return nil
}
