// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package tag

import (
	"fmt"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/vspherepolicy/tag/validation"
)

// AddToManager adds the Tag webhooks to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := validation.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to add tag validation webhooks to manager: %w", err)
	}
	return nil
}
