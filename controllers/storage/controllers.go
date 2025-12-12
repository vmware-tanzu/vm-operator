// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/storage/storageclass"
	"github.com/vmware-tanzu/vm-operator/controllers/storage/storagepolicy"
	"github.com/vmware-tanzu/vm-operator/controllers/storage/storagepolicyquota"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	if err := storagepolicyquota.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize storagepolicyquota controller: %w", err)
	}

	features := pkgcfg.FromContext(ctx).Features

	if features.BringYourOwnEncryptionKey || features.FastDeploy {

		if err := storageclass.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize StorageClass controller: %w", err)
		}
	}

	if features.BringYourOwnEncryptionKey ||
		features.FastDeploy ||
		features.VMSharedDisks ||
		features.AllDisksArePVCs {

		if err := storagepolicy.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize StoragePolicy controller: %w", err)
		}
	}

	return nil
}
