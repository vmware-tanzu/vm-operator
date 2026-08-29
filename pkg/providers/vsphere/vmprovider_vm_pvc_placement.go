// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	"github.com/vmware/govmomi/cns"
	cnstypes "github.com/vmware/govmomi/cns/types"
	"github.com/vmware/govmomi/vim25"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// pvcDiskPaths returns the datastore path of each Bound PVC's volume, keyed by
// PVC name. PVCs that are not Bound are skipped, since nothing is provisioned
// for them yet.
//
// This does not care whether a volume is host-local. A path on shared storage
// names a datastore every host can reach and so constrains nothing, while a
// path on host-local storage names one only a single host can reach. Handing
// DRS the true location of every existing disk is what lets it work the host
// out, with no need to classify the volumes first.
//
// The path is not recorded on any Kubernetes object: the PV's volume attributes
// and the CnsVolumeInfo both omit it. The PV's volumeHandle is the FCD ID
// though, and CNS answers for that, so the path is resolved by querying CNS.
func pvcDiskPaths(
	ctx context.Context,
	vimClient *vim25.Client,
	k8sClient ctrlclient.Client,
	pvcs []corev1.PersistentVolumeClaim) (map[string]string, error) {

	volumeIDToPVC := map[string]string{}

	for _, pvc := range pvcs {
		if pvc.Status.Phase == corev1.ClaimBound && pvc.Spec.VolumeName != "" {
			var pv corev1.PersistentVolume
			if err := k8sClient.Get(
				ctx,
				ctrlclient.ObjectKey{Name: pvc.Spec.VolumeName},
				&pv); err != nil {

				return nil, fmt.Errorf("failed to get PV %s for PVC %s: %w",
					pvc.Spec.VolumeName, pvc.Name, err)
			}

			if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle != "" {
				volumeIDToPVC[pv.Spec.CSI.VolumeHandle] = pvc.Name
			}
		}
	}

	if len(volumeIDToPVC) == 0 {
		return nil, nil
	}

	cnsClient, err := cns.NewClient(ctx, vimClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create CNS client: %w", err)
	}

	volumeIDs := make([]cnstypes.CnsVolumeId, 0, len(volumeIDToPVC))
	for id := range volumeIDToPVC {
		volumeIDs = append(volumeIDs, cnstypes.CnsVolumeId{Id: id})
	}

	res, err := cnsClient.QueryVolume(ctx, &cnstypes.CnsQueryFilter{
		VolumeIds: volumeIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query CNS for volumes %v: %w",
			volumeIDs, err)
	}

	paths := map[string]string{}

	for _, vol := range res.Volumes {
		details, ok := vol.BackingObjectDetails.(*cnstypes.CnsBlockBackingDetails)
		if ok && details.BackingDiskPath != "" {
			if pvcName := volumeIDToPVC[vol.VolumeId.Id]; pvcName != "" {
				paths[pvcName] = details.BackingDiskPath
			}
		}
	}

	return paths, nil
}
