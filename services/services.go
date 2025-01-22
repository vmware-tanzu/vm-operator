// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmwatcher "github.com/vmware-tanzu/vm-operator/services/vm-watcher"
)

// AddToManager adds all services to the provided manager.
func AddToManager(
	ctx *pkgctx.ControllerManagerContext,
	mgr manager.Manager) error {

	if pkgcfg.FromContext(ctx).AsyncSignalEnabled {
		if err := vmwatcher.AddToManager(ctx, mgr); err != nil {
			return err
		}
	}

	return nil
}
