// Copyright (c) 2019-2022 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"github.com/pkg/errors"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/contentsource"
	"github.com/vmware-tanzu/vm-operator/controllers/infracluster"
	"github.com/vmware-tanzu/vm-operator/controllers/infraprovider"
	"github.com/vmware-tanzu/vm-operator/controllers/providerconfigmap"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineimage"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/controllers/volume"
	"github.com/vmware-tanzu/vm-operator/controllers/webconsolerequest"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

// AddToManager adds all controllers to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if err := contentsource.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize ContentSource controller")
	}
	if err := infracluster.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize InfraCluster controller")
	}
	if err := infraprovider.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize InfraProvider controller")
	}
	if err := providerconfigmap.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize ProviderConfigMap controller")
	}
	if err := virtualmachine.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachine controller")
	}
	if err := virtualmachineclass.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineClass controller")
	}
	if err := virtualmachineimage.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineImage controller")
	}
	if lib.IsWCPVMImageRegistryEnabled() {
		if err := virtualmachinepublishrequest.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize VirtualMachinePublishRequest controller")
		}
	}
	if err := virtualmachineservice.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineService controller")
	}
	if err := virtualmachinesetresourcepolicy.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineSetResourcePolicy controller")
	}
	if err := volume.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize Volume controller")
	}
	if err := webconsolerequest.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize WebConsoleRequest controller")
	}
	return nil
}
