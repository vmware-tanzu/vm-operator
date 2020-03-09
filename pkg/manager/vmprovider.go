// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"k8s.io/client-go/rest"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere"
)

func setupVmProvider(ctx *context.ControllerManagerContext, cfg *rest.Config) error {
	ctx.Logger.Info("Setting up vSphere Provider")
	vmProvider := vsphere.NewVsphereMachineProviderFromRestConfig(cfg)

	// Register the vSphere provider
	providerService := vmprovider.GetService()
	providerService.RegisterVmProvider(vmProvider)

	ctx.VmProvider = vmProvider
	return nil
}
