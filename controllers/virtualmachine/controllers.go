// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/storagepolicyusage"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volume"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachine/volumebatch"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	if err := virtualmachine.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize virtualmachine controller: %w", err)
	}
	if err := storagepolicyusage.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize virtualmachine storagepolicyusage controller: %w", err)
	}

	if pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		if err := volumebatch.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize Volume Batch controller: %w", err)
		}
	} else {
		if err := volume.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize virtualmachine volume controller: %w", err)
		}
	}

	return nil
}
