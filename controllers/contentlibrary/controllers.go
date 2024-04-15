// Copyright (c) 2023-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/clustercontentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/contentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if err := clustercontentlibraryitem.AddToManager(ctx, mgr); err != nil {
		return err
	}
	return contentlibraryitem.AddToManager(ctx, mgr)
}
