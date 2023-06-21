// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package webhooks

import (
	"github.com/pkg/errors"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/persistentvolumeclaim"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineclass"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinepublishrequest"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineservice"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinesetresourcepolicy"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinewebconsolerequest"
)

// AddToManager adds all webhooks and a certificate manager to the provided controller manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := persistentvolumeclaim.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize PersistentVolumeClaim webhook")
	}
	if err := virtualmachine.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachine webhooks")
	}
	if err := virtualmachineclass.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineClass webhooks")
	}
	if err := virtualmachinepublishrequest.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachinePublishRequest webhooks")
	}
	if err := virtualmachineservice.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineService webhooks")
	}
	if err := virtualmachinesetresourcepolicy.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineSetResourcePolicy webhooks")
	}
	if err := virtualmachinewebconsolerequest.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize VirtualMachineWebConsoleRequest webhooks")
	}
	return nil
}
