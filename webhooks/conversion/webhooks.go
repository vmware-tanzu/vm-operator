// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package conversion

import (
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmopv1a1 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/v1alpha4"
	vmopv1 "github.com/vmware-tanzu/vm-operator/webhooks/conversion/v1alpha5"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {

	if err := vmopv1a1.AddToManager(ctx, mgr); err != nil {
		return err
	}
	if err := vmopv1a2.AddToManager(ctx, mgr); err != nil {
		return err
	}
	if err := vmopv1a3.AddToManager(ctx, mgr); err != nil {
		return err
	}
	if err := vmopv1a4.AddToManager(ctx, mgr); err != nil {
		return err
	}
	if err := vmopv1.AddToManager(ctx, mgr); err != nil {
		return err
	}

	return nil
}
