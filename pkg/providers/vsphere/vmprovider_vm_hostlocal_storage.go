// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	storagehelpers "k8s.io/component-helpers/storage/volume"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/storage"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// pvcPlacementStartDeviceKey is the starting key for the placement-only PVC
// disks. A negative range is traditionally used, and this is kept clear of the
// ranges used for PCI passthrough and instance storage devices.
const pvcPlacementStartDeviceKey = int32(-1000)

// esxHostMoIDNodeAnnotationKey is the annotation a Supervisor Node object
// carries naming the MOID of the ESXi host it corresponds to.
const esxHostMoIDNodeAnnotationKey = "vmware-system-esxi-node-moid"

// getESXHostInfoForNode returns the ESXi HostSystem MoID and availability zone
// for the Supervisor Node with the given name. It returns an error if the Node
// does not exist or does not carry the ESXi host MOID annotation.
func getESXHostInfoForNode(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	nodeName string) (hostMoID, zoneName string, err error) {

	node := &corev1.Node{}
	if err := k8sClient.Get(
		ctx, ctrlclient.ObjectKey{Name: nodeName}, node); err != nil {

		return "", "", fmt.Errorf("failed to get Node %q: %w", nodeName, err)
	}

	hostMoID = node.Annotations[esxHostMoIDNodeAnnotationKey]
	if hostMoID == "" {
		return "", "", fmt.Errorf(
			"node %q does not have the %q annotation",
			nodeName, esxHostMoIDNodeAnnotationKey)
	}

	return hostMoID, node.Labels[corev1.LabelTopologyZone], nil
}

// getNodeNameForESXHostMoID returns the name of the Supervisor Node that
// corresponds to the ESXi host with the given MOID. It is the inverse of
// getESXHostInfoForNode and reads the same annotation, so the name it returns is
// the node's Kubernetes name rather than one derived from vCenter — which
// matters because CNS matches the selected node against
// kubernetes.io/hostname.
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
// PVCs whose data source is the VM itself are skipped: those disks originate
// from the image and are already present in the ConfigSpec, where
// vmCreateGenConfigSpecImagePVCDataSourceRefs applies their policy and size.
// Adding them again would count the same storage twice.
func AddPVCPlacementDisks(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	storageData storage.VMStorageData) error {

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

		configSpec.DeviceChange = append(
			configSpec.DeviceChange,
			&vimtypes.VirtualDeviceConfigSpec{
				Operation:     vimtypes.VirtualDeviceConfigSpecOperationAdd,
				FileOperation: vimtypes.VirtualDeviceConfigSpecFileOperationCreate,
				Device: &vimtypes.VirtualDisk{
					CapacityInBytes: capacity.Value(),
					VirtualDevice: vimtypes.VirtualDevice{
						Key: deviceKey,
						Backing: &vimtypes.VirtualDiskFlatVer2BackingInfo{
							ThinProvisioned: ptr.To(false),
						},
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

// hostLocalPlacement describes what is known about a VM's host-local storage
// before placement runs.
type hostLocalPlacement struct {
	// NodeName is the Supervisor node whose ESXi host the VM must be created
	// on, or empty when no host is known yet.
	NodeName string

	// HostMoID is NodeName's ESXi host MoID. Empty when NodeName is.
	HostMoID string

	// ZoneName is NodeName's availability zone. A Supervisor node belongs to
	// exactly one zone and one cluster, so a known host implies both.
	ZoneName string

	// PendingPVCs are the VM's host-local PVCs that are not yet provisioned
	// and carry no host. Populated only when no host is known yet, in which
	// case placement must select a host compliant with their storage policies.
	PendingPVCs []corev1.PersistentVolumeClaim
}

// Resolved returns true when the VM's host is already determined, so that
// placement must honor it rather than choosing one.
func (p hostLocalPlacement) Resolved() bool {
	return p.HostMoID != ""
}

// resolveHostLocalStorage determines what is known about a VM's host-local
// storage placement. It derives this fresh on every call and records nothing
// on the VM, so a VM whose creation fails is free to be placed elsewhere on a
// subsequent attempt.
//
// A host is known when either of the following names one. Both are facts
// established outside of placement, so re-deriving them is stable:
//
//  1. A host-local PVC already stamped with a selected node, which is how a
//     host chosen by an earlier placement is remembered.
//  2. A Bound host-local PVC, whose volume physically resides on that host.
//
// PVCs naming different hosts are a hard error rather than a pick-one, since
// the VM has to reach every one of its disks.
//
// When no host is known, the VM's unprovisioned host-local PVCs are returned
// so that placement can be forced to recommend a host with a datastore
// compliant with their storage policies.
//
// Callers are responsible for checking the HostLocalStorage capability before
// calling this.
func resolveHostLocalStorage(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	storageData storage.VMStorageData) (hostLocalPlacement, error) {

	hostLocalPVCs := hostLocalPVCsForVM(storageData)
	if len(hostLocalPVCs) == 0 {
		return hostLocalPlacement{}, nil
	}

	nodeName, err := resolveHostLocalNodeName(hostLocalPVCs)
	if err != nil {
		return hostLocalPlacement{}, err
	}

	if nodeName == "" {
		return hostLocalPlacement{
			PendingPVCs: unprovisionedHostLocalPVCs(hostLocalPVCs, storageData),
		}, nil
	}

	hostMoID, zoneName, err := getESXHostInfoForNode(
		vmCtx, k8sClient, nodeName)
	if err != nil {
		return hostLocalPlacement{}, fmt.Errorf(
			"failed to resolve host-local node %q: %w", nodeName, err)
	}

	if zoneName != "" {
		if existing := vmCtx.VM.Labels[corev1.LabelTopologyZone]; existing != "" &&
			existing != zoneName {

			return hostLocalPlacement{}, fmt.Errorf(
				"host-local node %q zone %q conflicts with VM's zone label %q",
				nodeName, zoneName, existing)
		}
	}

	return hostLocalPlacement{
		NodeName: nodeName,
		HostMoID: hostMoID,
		ZoneName: zoneName,
	}, nil
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

	pvcs := unprovisionedHostLocalPVCs(hostLocalPVCsForVM(storageData), storageData)
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

// hostLocalPVCsForVM returns the VM's PVCs that are backed by a host-local
// StorageClass.
func hostLocalPVCsForVM(
	storageData storage.VMStorageData) []corev1.PersistentVolumeClaim {

	pvcs := make([]corev1.PersistentVolumeClaim, 0, len(storageData.PVCs))

	for _, pvc := range storageData.PVCs {
		// A PVC whose data source is the VM itself describes one of the VM's
		// own disks, which already exists wherever the VM does. It neither
		// constrains where the VM may go nor waits to be told a host, so it
		// takes no part in host resolution or in the handoff to CNS. This is
		// the same exclusion that placement and zone-constraint derivation
		// make.
		if isHostLocalPVC(pvc, storageData) &&
			!kubeutil.HasVirtualMachineDataSourceRef(pvc) {

			pvcs = append(pvcs, pvc)
		}
	}

	return pvcs
}

// isHostLocalPVC returns true if the given PVC is backed by a host-local
// StorageClass.
func isHostLocalPVC(
	pvc corev1.PersistentVolumeClaim,
	storageData storage.VMStorageData) bool {

	scName := pvc.Spec.StorageClassName
	if scName == nil {
		return false
	}

	sc, ok := storageData.StorageClasses[*scName]

	return ok && kubeutil.IsHostLocalStorageClass(sc)
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

// resolveHostLocalNodeName returns the Supervisor node named by any of the VM's
// host-local PVCs, or empty if none names one. Disagreeing names are an error,
// since the VM has to reach every one of its disks.
func resolveHostLocalNodeName(
	hostLocalPVCs []corev1.PersistentVolumeClaim) (string, error) {

	var nodeName string

	for _, pvc := range hostLocalPVCs {
		pvcNodeName := hostLocalPVCNodeName(pvc)

		switch {
		case pvcNodeName == "":
			// This PVC does not name a host yet.
		case nodeName == "":
			nodeName = pvcNodeName
		case nodeName != pvcNodeName:
			return "", fmt.Errorf(
				"VM has host-local PVCs bound to conflicting hosts %q and %q",
				nodeName, pvcNodeName)
		}
	}

	return nodeName, nil
}

// hostLocalPVCNodeName returns the Supervisor node that a host-local PVC's
// volume resides on, or has already been selected for, or empty if neither is
// known yet.
func hostLocalPVCNodeName(pvc corev1.PersistentVolumeClaim) string {
	// A selected node is authoritative even before the volume is provisioned,
	// since CNS provisions the volume where the annotation says. A host-local
	// PVC's selected node is a host rather than a zone.
	nodeName := pvc.Annotations[storagehelpers.AnnSelectedNode]
	if nodeName != "" &&
		pvc.Annotations[constants.CNSSelectedNodeIsZoneAnnotationKey] != "true" {

		return nodeName
	}

	return kubeutil.GetPVCHostLocalHostname(pvc)
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
			hostLocalPVCNodeName(pvc) == "" &&
			isWaitForFirstConsumerPVC(pvc, storageData) {

			pvcs = append(pvcs, pvc)
		}
	}

	return pvcs
}
