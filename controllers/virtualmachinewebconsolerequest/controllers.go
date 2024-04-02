// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package virtualmachinewebconsolerequest

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/controllers/virtualmachinewebconsolerequest/v1alpha2"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
)

// AddToManager adds the controller to the provided manager.
func AddToManager(ctx *context.ControllerManagerContext, mgr manager.Manager) error {
	if err := v1alpha2.AddToManager(ctx, mgr); err != nil {
		return err
	}
	// NOTE: In v1a1 this CRD has a different name - WebConsoleRequest - so this is
	// still required until we stop supporting v1a1.
	return v1alpha1.AddToManager(ctx, mgr)
}
