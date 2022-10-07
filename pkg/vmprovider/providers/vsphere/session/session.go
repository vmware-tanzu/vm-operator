// Copyright (c) 2018-2021 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/object"
	ctrlruntime "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/client"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

var log = logf.Log.WithName("vsphere").WithName("session")

type Session struct {
	Client    *client.Client
	K8sClient ctrlruntime.Client
	Finder    *find.Finder

	// Fields only used during Update
	Cluster         *object.ClusterComputeResource
	NetworkProvider network.Provider
}

func NewSessionAndConfigure(
	ctx goctx.Context,
	client *client.Client,
	config *config.VSphereVMProviderConfig,
	k8sClient ctrlruntime.Client) (*Session, error) {

	return nil, fmt.Errorf("all this will be removed next")
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
