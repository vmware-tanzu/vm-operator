// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/clustercontentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/contentlibraryitem"
	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/v1alpha1/contentsource"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
)

// AddToManager adds the controllers to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if lib.IsWCPVMImageRegistryEnabled() {
		if err := clustercontentlibraryitem.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize ClusterContentLibraryItem controller")
		}
		if err := contentlibraryitem.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize ContentLibraryItem controller")
		}
	} else {
		if err := contentsource.AddToManager(ctx, mgr); err != nil {
			return errors.Wrap(err, "failed to initialize ContentSource controller")
		}
	}

	return nil
}
