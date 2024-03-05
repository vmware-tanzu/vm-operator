// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package infra

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/infra/configmap"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/node"
	"github.com/vmware-tanzu/vm-operator/controllers/infra/secret"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if err := configmap.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize infra configmap controller")
	}
	if err := node.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize infra node controller")
	}
	if err := secret.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize infra secret controller")
	}

	return nil
}
