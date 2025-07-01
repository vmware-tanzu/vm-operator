// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinereplicaset

import (
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinereplicaset/mutation"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinereplicaset/validation"
)

func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := mutation.AddToManager(ctx, mgr); err != nil {
		return err
	}

	return validation.AddToManager(ctx, mgr)
}
