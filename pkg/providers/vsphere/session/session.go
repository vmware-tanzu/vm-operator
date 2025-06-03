// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"

	"github.com/vmware/govmomi/find"
	"github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/internal"
	pkgclient "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/client"
)

type Session struct {
	Client       *pkgclient.Client
	K8sClient    ctrlclient.Client
	Finder       *find.Finder
	ClusterMoRef types.ManagedObjectReference
}

func (s *Session) invokeFsrVirtualMachine(vmCtx pkgctx.VirtualMachineContext) error {
	vmCtx.Logger.Info("Invoking FSR on VM")

	task, err := internal.VirtualMachineFSR(vmCtx, vmCtx.MoVM.Self, s.Client.VimClient())
	if err != nil {
		return fmt.Errorf("failed to invoke FSR: %w", err)
	}

	if err = task.Wait(vmCtx); err != nil {
		return fmt.Errorf("failed to wait on FSR task: %w", err)
	}

	return nil
}
