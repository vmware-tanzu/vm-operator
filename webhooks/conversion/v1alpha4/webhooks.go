// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha4

import (
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachine{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachineClass{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.ClusterVirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachineImageCache{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachinePublishRequest{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachineService{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachineSetResourcePolicy{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a4.VirtualMachineWebConsoleRequest{}).
		Complete(); err != nil {

		return err
	}

	return nil
}
