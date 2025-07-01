// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinewebconsolerequest

import (
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinewebconsolerequest/validation"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := validation.AddToManager(ctx, mgr); err != nil {
		return err
	}
	// NOTE: In v1a1 this CRD has a different name - WebConsoleRequest - so this is
	// still required until we stop supporting v1a1.
	return v1alpha1.AddToManager(ctx, mgr)
}
