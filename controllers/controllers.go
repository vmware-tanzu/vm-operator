// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"github.com/pkg/errors"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary"
	"github.com/vmware-tanzu/vm-operator/controllers/infracluster"
	"github.com/vmware-tanzu/vm-operator/controllers/infraprovider"
	"github.com/vmware-tanzu/vm-operator/controllers/providerconfigmap"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineclass"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinepublishrequest"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest"
	"github.com/vmware-tanzu/vm-operator/controllers/volume"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds all controllers to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if err := contentlibrary.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize ContentLibrary controllers")
	}
	if err := infracluster.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize InfraCluster controller")
	}
	if err := infraprovider.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize InfraProvider controller")
	}
	if err := virtualmachine.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachine controller")
	}
	if err := virtualmachineclass.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineClass controller")
	}
	if err := virtualmachineservice.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineService controller")
	}
	if err := virtualmachinesetresourcepolicy.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineSetResourcePolicy controller")
	}
	if err := virtualmachinewebconsolerequest.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineWebConsoleRequest controller")
	}
	if err := volume.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize Volume controller")
	}
	if pkgconfig.FromContext(ctx).Features.ImageRegistry {
		if err := virtualmachinepublishrequest.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize VirtualMachinePublishRequest controller")
		}
	} else {
		// We only update TKG related ContentSource/ContentLibraryProvider/ContentSourceBinding resources
		// in provider configmap reconcile. These resources will be removed when the FSS is enabled,
		// add provider configmap controller to the manager only when the FSS is disabled.
		if err := providerconfigmap.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize ProviderConfigMap controller")
		}
	}
	return nil
}
