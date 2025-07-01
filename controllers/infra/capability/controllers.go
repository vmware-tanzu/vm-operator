// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package capability

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	capv1 "github.com/vmware-tanzu/vm-operator/controllers/infra/capability/configmap"
	capv2 "github.com/vmware-tanzu/vm-operator/controllers/infra/capability/crd"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	if pkgcfg.FromContext(ctx).Features.SVAsyncUpgrade {
		if err := capv2.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize crd capability controller: %w", err)
		}
	} else if err := capv1.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize configmap capability controller: %w", err)
	}

	return nil
}
