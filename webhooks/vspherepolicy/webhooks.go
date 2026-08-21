// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vspherepolicy

import (
	"fmt"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/webhooks/vspherepolicy/tag"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the vSphere Policy webhooks to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if pkgcfg.FromContext(ctx).Features.TaggingAPI {
		err := tag.AddToManager(ctx, mgr)
		if err != nil {
			return fmt.Errorf("failed to initialize Tag webhook: %w", err)
		}
	}

	return nil
}
