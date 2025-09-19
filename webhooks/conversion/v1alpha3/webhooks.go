// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha3

import (
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachine{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachineClass{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.ClusterVirtualMachineImage{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachinePublishRequest{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachineService{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachineSetResourcePolicy{}).
		Complete(); err != nil {

		return err
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&vmopv1a3.VirtualMachineWebConsoleRequest{}).
		Complete(); err != nil {

		return err
	}

	return nil
}
