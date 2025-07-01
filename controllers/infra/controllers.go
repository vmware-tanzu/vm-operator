// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
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
	"github.com/vmware-tanzu/vm-operator/controllers/infra/zone"
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
	if err := validatingwebhookconfiguration.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize validatingwebhookconfiguration webhook controller: %w", err)
	}
	if err := zone.AddToManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to initialize infra zone controller: %w", err)
	}

	return nil
}
