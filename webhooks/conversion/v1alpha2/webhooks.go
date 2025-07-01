// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha2

import (
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachine{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachineClass{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachinePublishRequest{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachineService{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachineSetResourcePolicy{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a2.VirtualMachineWebConsoleRequest{}).
		Complete(); err != nil {

		return err
	}

	return nil
}
