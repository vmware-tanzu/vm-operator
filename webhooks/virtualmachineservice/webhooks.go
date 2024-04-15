// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachineservice

import (
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineservice/mutation"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachineservice/validation"
)

func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if err := mutation.AddToManager(ctx, mgr); err != nil {
		return err
	}
	return validation.AddToManager(ctx, mgr)
}
