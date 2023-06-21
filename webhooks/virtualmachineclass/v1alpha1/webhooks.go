// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/pkg/errors"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineclass/v1alpha1/mutation"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineclass/v1alpha1/validation"
)

func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := validation.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize validation webhook")
	}
	if err := mutation.AddToManager(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed to initialize mutation webhook")
	}
	return nil
}
