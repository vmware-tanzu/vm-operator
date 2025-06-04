// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinegroup

import (
	"fmt"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinegroup/mutation"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinegroup/validation"
)

// AddToManager adds the webhook to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := mutation.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize mutation webhook: %w", err)
	}

	if err := validation.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize validation webhook: %w", err)
	}

	return nil
}
