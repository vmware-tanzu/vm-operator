// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"
	"time"

	"github.com/vmware/govmomi/cns"
	cnstypes "github.com/vmware/govmomi/cns/types"
	"github.com/vmware/govmomi/vim25"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/storage"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// hostLocalPVCBindRequeueDelay is how long to wait before re-checking a
// host-local PVC that has been given a host but is not Bound yet.
const hostLocalPVCBindRequeueDelay = 10 * time.Second

// pvcPlacementStartDeviceKey is the starting key for the placement-only PVC
// disks. A negative range is traditionally used, and this is kept clear of the
// ranges used for PCI passthrough and instance storage devices.
const pvcPlacementStartDeviceKey = int32(-1000)

// esxHostMoIDNodeAnnotationKey is the annotation a Supervisor Node object
// carries naming the MOID of the ESXi host it corresponds to.
const esxHostMoIDNodeAnnotationKey = "vmware-system-esxi-node-moid"

// getNodeNameForESXHostMoID returns the name of the Supervisor Node that
// corresponds to the ESXi host with the given MOID. The name it returns is the
// node's Kubernetes name rather than one derived from vCenter — which matters
// because CNS matches the selected node against kubernetes.io/hostname.
func getNodeNameForESXHostMoID(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	hostMoID string) (string, error) {

	var nodes corev1.NodeList
	if err := k8sClient.List(ctx, &nodes); err != nil {
		return "", fmt.Errorf("failed to list Nodes: %w", err)
	}

	for i := range nodes.Items {
		if nodes.Items[i].Annotations[esxHostMoIDNodeAnnotationKey] == hostMoID {
			return nodes.Items[i].Name, nil
		}
	}

	return "", fmt.Errorf(
		"no Node has the %q annotation with value %q",
		esxHostMoIDNodeAnnotationKey, hostMoID)
}

// AddPVCPlacementDisks adds one placement-only disk to the given placement
// ConfigSpec for each of the VM's PVCs, carrying that PVC's storage policy, so
// that the recommended host and datastore are compatible with all of the VM's
// storage rather than only with the VM's own files.
//
// diskPaths optionally maps a PVC name to the datastore path of its
// already-provisioned volume. A disk given its real path is what lets DRS
// resolve the host for host-local storage: only one host can reach a host-local
// datastore, so naming the path narrows placement to that host without
// placement having to be told which host to use. Such a disk carries no file
// operation, since the file already exists.
//
// PVCs whose data source is the VM itself are skipped: those disks originate
// from the image and are already present in the ConfigSpec, where
// vmCreateGenConfigSpecImagePVCDataSourceRefs applies their policy and size.
// Adding them again would count the same storage twice.
func AddPVCPlacementDisks(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	storageData storage.VMStorageData,
	diskPaths map[string]string) error {

	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(storageData.PVCs))
	for _, pvc := range storageData.PVCs {
		if !kubeutil.HasVirtualMachineDataSourceRef(pvc) {
			pvcs = append(pvcs, pvc)
		}
	}

	if len(pvcs) == 0 {
		return nil
	}

	deviceKey := pvcPlacementStartDeviceKey

	for _, pvc := range pvcs {
		capacity, ok := pvc.Status.Capacity[corev1.ResourceStorage]
		if !ok {
			capacity = pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		}

		var policyID string
		if scName := pvc.Spec.StorageClassName; scName != nil {
			policyID = storageData.StorageClassToPolicyID[*scName]
		}

		backing := &vimtypes.VirtualDiskFlatVer2BackingInfo{
			ThinProvisioned: ptr.To(false),
		}

		// A provisioned volume is named by its path and is not created.
		fileOp := vimtypes.VirtualDeviceConfigSpecFileOperationCreate
		if p := diskPaths[pvc.Name]; p != "" {
			backing.FileName = p
			fileOp = ""
		}

		configSpec.DeviceChange = append(
			configSpec.DeviceChange,
			&vimtypes.VirtualDeviceConfigSpec{
				Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
				FileOperation: fileOp,
				Device: &vimtypes.VirtualDisk{
					CapacityInBytes: capacity.Value(),
					VirtualDevice: vimtypes.VirtualDevice{
						Key:     deviceKey,
						Backing: backing,
					},
				},
				Profile: []vimtypes.BaseVirtualMachineProfileSpec{
					&vimtypes.VirtualMachineDefinedProfileSpec{
						ProfileId: policyID,
					},
				},
			})

		deviceKey--
	}

	// The disks added above need controllers, exactly as the image-sourced PVC
	// disks do.
	if err := pkgutil.EnsureDisksHaveControllers(configSpec); err != nil {
		return fmt.Errorf(
			"failed to ensure disk/controller specs for placement pvcs: %w", err)
	}

	return nil
}

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
		if pvc.Status.Phase != corev1.ClaimBound || pvc.Spec.VolumeName == "" {
			continue
		}

		var pv corev1.PersistentVolume
		if err := k8sClient.Get(
			ctx, ctrlclient.ObjectKey{Name: pvc.Spec.VolumeName}, &pv); err != nil {

			return nil, fmt.Errorf("failed to get PV %s for PVC %s: %w",
				pvc.Spec.VolumeName, pvc.Name, err)
		}

		if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeHandle == "" {
			continue
		}

		volumeIDToPVC[pv.Spec.CSI.VolumeHandle] = pvc.Name
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
		if !ok || details.BackingDiskPath == "" {
			continue
		}
		if pvcName := volumeIDToPVC[vol.VolumeId.Id]; pvcName != "" {
			paths[pvcName] = details.BackingDiskPath
		}
	}

	return paths, nil
}

// hostLocalPlacementNeeded reports whether the VM has any host-local PVC, in
// which case placement must go through PlaceVm and return a host: for a
// provisioned volume so the host matches the disk path in the ConfigSpec, and
// for an unprovisioned one so the chosen host can be published back to the PVC.
//
// Nothing about the host is derived or recorded here. It is DRS's decision, and
// it is only published once the VM actually exists on it, so a VM whose create
// fails is free to be placed elsewhere next time.
//
// Callers are responsible for checking the HostLocalStorage capability before
// calling this.
func hostLocalPlacementNeeded(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	storageData storage.VMStorageData) (bool, error) {

	hostLocalPVCs, err := hostLocalPVCsForVM(vmCtx, k8sClient, storageData)
	if err != nil {
		return false, err
	}
	if len(hostLocalPVCs) == 0 {
		return false, nil
	}

	// A PVC that has already been told which host to provision on, but is not
	// Bound yet, has no datastore for placement to name in the ConfigSpec. DRS
	// would then be free to pick a different host than the one CNS was given,
	// leaving the VM unable to reach its volume. Wait for CNS to finish
	// provisioning instead: the host it was given is already recorded on the
	// PVC, so binding proceeds without any further help from placement.
	for _, pvc := range hostLocalPVCs {
		if pvc.Status.Phase != corev1.ClaimBound &&
			pvcSelectedHost(pvc) != "" {

			return false, pkgerr.RequeueError{
				After: hostLocalPVCBindRequeueDelay,
			}
		}
	}

	return true, nil
}

// reconcileHostLocalStorage publishes the ESXi host that the VM actually runs
// on to any of its host-local PVCs that are not yet provisioned, so that CNS
// provisions their volumes on that same host and the VM can reach them.
//
// This is deliberately driven by where the VM really is rather than by the
// host placement recommended, so that nothing is committed to before the VM
// exists, and so that a failed attempt is simply retried on the next
// reconcile.
func (vs *vSphereVMProvider) reconcileHostLocalStorage(
	vmCtx pkgctx.VirtualMachineContext) error {

	if vmCtx.MoVM.Summary.Runtime.Host == nil {
		// The VM is not assigned to a host, so there is nothing to publish.
		return nil
	}

	storageData, err := storage.GetVMStorageData(vmCtx, vs.k8sClient)
	if err != nil {
		return err
	}

	hostLocalPVCs, err := hostLocalPVCsForVM(vmCtx, vs.k8sClient, storageData)
	if err != nil {
		return err
	}

	pvcs := unprovisionedHostLocalPVCs(hostLocalPVCs, storageData)
	if len(pvcs) == 0 {
		return nil
	}

	// Resolve the host back to its Supervisor node name, since that is what
	// CNS matches the selected-node annotation against.
	hostMoID := vmCtx.MoVM.Summary.Runtime.Host.Value
	nodeName, err := getNodeNameForESXHostMoID(
		vmCtx, vs.k8sClient, hostMoID)
	if err != nil {
		return fmt.Errorf(
			"failed to resolve node for host %s: %w", hostMoID, err)
	}

	for i := range pvcs {
		pvc := &pvcs[i]

		objPatch := ctrlclient.MergeFrom(pvc.DeepCopy())
		if pvc.Annotations == nil {
			pvc.Annotations = map[string]string{}
		}
		pvc.Annotations[storagehelpers.AnnSelectedNode] = nodeName
		pvc.Annotations[constants.CNSSelectedNodeIsZoneAnnotationKey] = "false"

		if err := vs.k8sClient.Patch(vmCtx, pvc, objPatch); err != nil {
			return fmt.Errorf(
				"failed to set selected node on host-local PVC %s: %w",
				pvc.Name, err)
		}

		vmCtx.Logger.V(4).Info(
			"Published host-local host to PVC",
			"pvcName", pvc.Name,
			"nodeName", nodeName)
	}

	return nil
}

// hostLocalPVCsForVM returns the VM's PVCs whose storage policy requires
// host-local storage.
func hostLocalPVCsForVM(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	storageData storage.VMStorageData) ([]corev1.PersistentVolumeClaim, error) {

	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(storageData.PVCs))

	for _, pvc := range storageData.PVCs {
		// A PVC whose data source is the VM itself describes one of the VM's
		// own disks, which already exists wherever the VM does. It neither
		// constrains where the VM may go nor waits to be told a host, so it
		// takes no part in host resolution or in the handoff to CNS. This is
		// the same exclusion that placement and zone-constraint derivation
		// make. Check it first, since it avoids a lookup.
		if kubeutil.HasVirtualMachineDataSourceRef(pvc) {
			continue
		}

		hostLocal, err := isHostLocalPVC(ctx, k8sClient, pvc, storageData)
		if err != nil {
			return nil, err
		}
		if hostLocal {
			pvcs = append(pvcs, pvc)
		}
	}

	return pvcs, nil
}

// isHostLocalPVC returns true if the given PVC's storage policy requires
// host-local storage, determined from the policy's observed SPBM capability
// rather than from any annotation on the StorageClass.
func isHostLocalPVC(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	pvc corev1.PersistentVolumeClaim,
	storageData storage.VMStorageData) (bool, error) {

	scName := pvc.Spec.StorageClassName
	if scName == nil {
		return false, nil
	}

	policyID, ok := storageData.StorageClassToPolicyID[*scName]
	if !ok || policyID == "" {
		return false, nil
	}

	return kubeutil.IsHostLocalStorageProfile(ctx, k8sClient, policyID)
}

// isWaitForFirstConsumerPVC returns true if the given PVC's StorageClass defers
// provisioning until a consumer selects a node.
func isWaitForFirstConsumerPVC(
	pvc corev1.PersistentVolumeClaim,
	storageData storage.VMStorageData) bool {

	scName := pvc.Spec.StorageClassName
	if scName == nil {
		return false
	}

	sc, ok := storageData.StorageClasses[*scName]
	if !ok {
		return false
	}

	mode := sc.VolumeBindingMode

	return mode != nil && *mode == storagev1.VolumeBindingWaitForFirstConsumer
}

// pvcSelectedHost returns the node a PVC has already been told to provision
// on, or "" if it has not been told one. A zone-valued selected node does not
// count: that is the ordinary zonal handoff, not a host.
func pvcSelectedHost(pvc corev1.PersistentVolumeClaim) string {
	if pvc.Annotations[constants.CNSSelectedNodeIsZoneAnnotationKey] == "true" {
		return ""
	}

	return pvc.Annotations[storagehelpers.AnnSelectedNode]
}

// unprovisionedHostLocalPVCs returns the host-local PVCs that are still
// Pending, carry no host, and defer provisioning until a node is selected, so
// that placement has to choose a host for them and that host has to be
// published back to them.
//
// Only WaitForFirstConsumer PVCs qualify. An Immediate PVC is provisioned by
// CNS without waiting to be told a node, so it chooses its own host and there
// is nothing to publish; writing a selected node onto one would be asserting a
// host after the fact rather than before.
func unprovisionedHostLocalPVCs(
	hostLocalPVCs []corev1.PersistentVolumeClaim,
	storageData storage.VMStorageData) []corev1.PersistentVolumeClaim {

	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(hostLocalPVCs))

	for _, pvc := range hostLocalPVCs {
		if pvc.Status.Phase == corev1.ClaimPending &&
			pvcSelectedHost(pvc) == "" &&
			isWaitForFirstConsumerPVC(pvc, storageData) {

			pvcs = append(pvcs, pvc)
		}
	}

	return pvcs
}
