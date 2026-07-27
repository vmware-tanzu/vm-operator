// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/storage"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
)

// resolveHostLocalPlacement resolves host-local storage placement for the
// given VM, in priority order:
//
//  1. A host has already been resolved (HostLocalSelectedNodeMOIDAnnotationKey
//     is set) - nothing to do.
//  2. An explicit host override (HostLocalSelectedNodeAnnotationKey) is set
//     but not yet resolved to a MoID - resolve and pin it.
//  3. A Bound PVC on a host-local StorageClass names its host via CSI's
//     accessible/requested-topology annotation - resolve and pin it.
//  4. Otherwise, any Pending PVCs on a host-local StorageClass with no host
//     resolved anywhere yet are returned so the caller can force a DRS host
//     recommendation compliant with their storage policy.
//
// Cases 1-3 record the host on the VM - both the MoID annotation and the zone
// label - so that doesVMNeedPlacement returns it as an already-resolved host.
// Placement then either returns it directly, or, when a datastore
// recommendation is also required, constrains the recommendation to that host
// so the datastore returned is one the host can access.
//
// It is a no-op unless the HostLocalStorage capability is enabled and the VM
// references at least one host-local StorageClass PVC.
func resolveHostLocalPlacement(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	storageData storage.VMStorageData) ([]corev1.PersistentVolumeClaim, error) {

	if !pkgcfg.FromContext(vmCtx).Features.HostLocalStorage {
		return nil, nil
	}

	hostLocalPVCs := make([]corev1.PersistentVolumeClaim, 0, len(storageData.PVCs))
	for _, pvc := range storageData.PVCs {
		if pvc.Spec.StorageClassName == nil {
			continue
		}
		sc, ok := storageData.StorageClasses[*pvc.Spec.StorageClassName]
		if !ok || !kubeutil.IsHostLocalStorageClass(sc) {
			continue
		}
		hostLocalPVCs = append(hostLocalPVCs, pvc)
	}

	if len(hostLocalPVCs) == 0 {
		return nil, nil
	}

	if vmCtx.VM.Annotations[constants.HostLocalSelectedNodeMOIDAnnotationKey] != "" {
		// Host has already been resolved.
		return nil, nil
	}

	if nodeName := vmCtx.VM.Annotations[constants.HostLocalSelectedNodeAnnotationKey]; nodeName != "" {
		// Explicit override, not yet resolved to a MoID.
		return nil, pinHostLocalNode(vmCtx, k8sClient, nodeName)
	}

	var boundHostname string
	for _, pvc := range hostLocalPVCs {
		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}

		hostname := kubeutil.GetPVCHostLocalHostname(pvc)
		if hostname == "" {
			continue
		}

		if boundHostname == "" {
			boundHostname = hostname
		} else if boundHostname != hostname {
			return nil, fmt.Errorf(
				"VM has host-local PVCs bound to conflicting hosts %q and %q",
				boundHostname, hostname)
		}
	}

	if boundHostname != "" {
		return nil, pinHostLocalNode(vmCtx, k8sClient, boundHostname)
	}

	pendingPVCs := make([]corev1.PersistentVolumeClaim, 0, len(hostLocalPVCs))
	for _, pvc := range hostLocalPVCs {
		if pvc.Status.Phase == corev1.ClaimPending && kubeutil.GetPVCHostLocalHostname(pvc) == "" {
			pendingPVCs = append(pendingPVCs, pvc)
		}
	}

	return pendingPVCs, nil
}

// pinHostLocalNode resolves the given Supervisor node name to its ESXi host
// MoID and availability zone, and records both on the VM so that placement
// treats the host as already resolved. A Supervisor node belongs to exactly
// one zone and one cluster, so the host determines all three.
func pinHostLocalNode(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	nodeName string) error {

	hostMoID, zoneName, err := kubeutil.GetESXHostInfoForNode(vmCtx, k8sClient, nodeName)
	if err != nil {
		return fmt.Errorf("failed to resolve host-local node %q: %w", nodeName, err)
	}

	if zoneName != "" {
		if existing := vmCtx.VM.Labels[corev1.LabelTopologyZone]; existing != "" && existing != zoneName {
			return fmt.Errorf(
				"host-local node %q zone %q conflicts with VM's zone label %q",
				nodeName, zoneName, existing)
		}

		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = map[string]string{}
		}
		vmCtx.VM.Labels[corev1.LabelTopologyZone] = zoneName
	}

	if vmCtx.VM.Annotations == nil {
		vmCtx.VM.Annotations = map[string]string{}
	}
	vmCtx.VM.Annotations[constants.HostLocalSelectedNodeAnnotationKey] = nodeName
	vmCtx.VM.Annotations[constants.HostLocalSelectedNodeMOIDAnnotationKey] = hostMoID

	return nil
}
