// Copyright (c) 2019-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"github.com/pkg/errors"

	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachine/v1alpha2"
)

func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		// TODO: We'll likely still need the v1a1 mutation wehbook (at least some limited version of it)
		if err := v1alpha2.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize v1alpha2 webhooks")
		}
	} else {
		if err := v1alpha1.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize v1alpha1 webhooks")
		}
	}

	return nil
}
