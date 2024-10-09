// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infra

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/capability"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/configmap"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/node"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/secret"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/validatingwebhookconfiguration"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	if err := capability.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize infra capability controller: %w", err)
	}
	if err := configmap.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize infra configmap controller: %w", err)
	}
	if err := node.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize infra node controller: %w", err)
	}
	if err := secret.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize infra secret controller: %w", err)
	}
	if pkgcfg.FromContext(ctx).Features.UnifiedStorageQuota {
		if err := validatingwebhookconfiguration.AddToManager(ctx, mgr); err != nil {
			return fmt.Errorf("failed to initialize storagepolicyusage webhook controller: %w", err)
		}
	}

	return nil
}
