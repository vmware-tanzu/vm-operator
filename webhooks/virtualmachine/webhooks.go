// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/pkg/errors"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha1"
	v1a1mut "github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha1/mutation"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha2"
)

func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		if err := v1alpha2.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize v1alpha2 webhooks")
		}
		// With v1a2 FSS enabled, the v1a1 VM mutation webhook is added to the manager
		if err := v1a1mut.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize v1alpha1 virtual machine mutation webhooks")
		}
	} else {
		if err := v1alpha1.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize v1alpha1 webhooks")
		}
	}

	return nil
}
