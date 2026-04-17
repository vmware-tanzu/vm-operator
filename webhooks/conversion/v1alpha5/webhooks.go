// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha5

import (
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.ClusterVirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachine{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineClass{}).
		Complete(); err != nil {

		return err
	}

	if pkgcfg.FromContext(ctx).Features.ImmutableClasses {
		if err := ctrl.NewWebhookManagedBy(mgr).
			For(&vmopv1a5.VirtualMachineClassInstance{}).
			Complete(); err != nil {

			return err
		}
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineGroup{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineGroupPublishRequest{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineImageCache{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachinePublishRequest{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineReplicaSet{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineService{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineSetResourcePolicy{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineSnapshot{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a5.VirtualMachineWebConsoleRequest{}).
		Complete(); err != nil {

		return err
	}

	return nil
}
