// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	res "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
)

type Session struct {
	Client    *client.Client
	K8sClient ctrlruntime.Client
	Finder    *find.Finder

	// Fields only used during Update
	Cluster *object.ClusterComputeResource
}

func (s *Session) invokeFsrVirtualMachine(vmCtx context.VirtualMachineContext, resVM *res.VirtualMachine) error {
	vmCtx.Logger.Info("Invoking FSR on VM")

	task, err := internal.VirtualMachineFSR(vmCtx, resVM.MoRef(), s.Client.VimClient())
	if err != nil {
		vmCtx.Logger.Error(err, "InvokeFSR call failed")
		return err
	}

	if err = task.Wait(vmCtx); err != nil {
		vmCtx.Logger.Error(err, "InvokeFSR task failed")
		return err
	}

	return nil
}
