// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

// IsVMOwnedStorageVM returns true when the VM carries the VM-owned storage annotation,
// indicating that all volume operations on the VM use the new
// ownership-transfer attach/detach path.
func IsVMOwnedStorageVM(vm *vmopv1.VirtualMachine) bool {
	if vm == nil {
		return false
	}
	return vm.Annotations[pkgconst.VMOwnedVolumesAnnotation] == "true"
}

// cviNameForVolumeID returns the deterministic CsiVolumeInfo name for a given
// CNS volume ID. The naming convention is stable and matches what CSI uses when
// creating CsiVolumeInfo objects.
func cviNameForVolumeID(volumeID string) string {
	return "csi-volume-info-" + volumeID
}

// GetCVIByVolumeID returns the CsiVolumeInfo for the given CNS volume ID using
// a direct O(1) lookup by the deterministic name. Returns a not-found error if
// no CsiVolumeInfo exists for the volume.
func GetCVIByVolumeID(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, volumeID string,
) (*cnsv1alpha1.CsiVolumeInfo, error) {

	cvi := &cnsv1alpha1.CsiVolumeInfo{}
	key := ctrlclient.ObjectKey{
		Namespace: namespace,
		Name:      cviNameForVolumeID(volumeID),
	}
	if err := c.Get(ctx, key, cvi); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get CsiVolumeInfo %s: %w", key.Name, err)
	}
	return cvi, nil
}

// GetCVIByDiskUUID returns the CsiVolumeInfo for the given disk UUID using
// an API-server-indexed label selector. Returns nil when no CsiVolumeInfo
// carries the label, which means no volume with this disk UUID is currently
// tracked in the new ownership-transfer path.
//
// The lookup relies on the cns.vmware.com/disk-uuid label being indexed by the
// API server. At scale (200,000 PVCs) this is O(1) — do not replace with an
// unbounded List without that label selector.
func GetCVIByDiskUUID(
	ctx context.Context,
	c ctrlclient.Client,
	namespace, diskUUID string,
) (*cnsv1alpha1.CsiVolumeInfo, error) {

	cviList := &cnsv1alpha1.CsiVolumeInfoList{}
	if err := c.List(ctx, cviList,
		ctrlclient.InNamespace(namespace),
		ctrlclient.MatchingLabels{
			cnsv1alpha1.CVIDiskUUIDLabel: diskUUID,
		},
	); err != nil {
		return nil, fmt.Errorf(
			"failed to list CsiVolumeInfo by diskUUID %s in namespace %s: %w",
			diskUUID, namespace, err)
	}
	if len(cviList.Items) == 0 {
		return nil, nil
	}
	return &cviList.Items[0], nil
}
