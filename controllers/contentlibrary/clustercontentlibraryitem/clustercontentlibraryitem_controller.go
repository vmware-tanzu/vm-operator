// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package clustercontentlibraryitem

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"
	imgregv1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha2"

	"github.com/vmware-tanzu/vm-operator/controllers/contentlibrary/utils"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
)

// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=clustercontentlibraryitems,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=imageregistry.vmware.com,resources=clustercontentlibraryitems/status,verbs=get
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vmoperator.vmware.com,resources=clustervirtualmachineimages/status,verbs=get;update;patch

// AddToManager adds this package's controller to the provided manager.
func AddToManager(ctx *pkgctx.ControllerManagerContext, mgr manager.Manager) error {
	if pkgcfg.FromContext(ctx).Features.InventoryContentLibrary {
		return utils.AddToManagerV1A2(ctx, mgr, &imgregv1.ClusterContentLibraryItem{})
	}
	return utils.AddToManager(ctx, mgr, &imgregv1a1.ClusterContentLibraryItem{})
}
