// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package contentlibrary

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/clustercontentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/contentlibraryitem"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	if err := clustercontentlibraryitem.AddToManager(ctx, mgr); err != nil {
		return err
	}
	return contentlibraryitem.AddToManager(ctx, mgr)
}
