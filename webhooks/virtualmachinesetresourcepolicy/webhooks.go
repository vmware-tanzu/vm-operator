// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinesetresourcepolicy

import (
	ctrlmgr "sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinesetresourcepolicy/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/webhooks/virtualmachinesetresourcepolicy/v1alpha2"
)

func AddToManager(ctx *context.ControllerManagerContext, mgr ctrlmgr.Manager) error {
	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		return v1alpha2.AddToManager(ctx, mgr)
	}
	return v1alpha1.AddToManager(ctx, mgr)
}
