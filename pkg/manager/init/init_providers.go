// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"fmt"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/record"
	vsphere2 "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2"
)

func InitializeProviders(
	ctx *context.ControllerManagerContext,
	mgr ctrlmgr.Manager) error {

	vmProviderName := fmt.Sprintf("%s/%s/vmProvider", ctx.Namespace, ctx.Name)
	recorder := record.New(mgr.GetEventRecorderFor(vmProviderName))
	ctx.VMProviderA2 = vsphere2.NewVSphereVMProviderFromClient(ctx, mgr.GetClient(), recorder)
	return nil
}
