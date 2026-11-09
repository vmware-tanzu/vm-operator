// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mobility

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/mobility/virtualmachineinframigration"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	features := pkgcfg.FromContext(ctx).Features

	if features.StoragePolicyMutability {
		if err := virtualmachineinframigration.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize VirtualMachineInfraMigration controller: %w", err)
		}
	}

	return nil
}
