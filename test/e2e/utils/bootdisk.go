// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/pbm"
	pbmtypes "github.com/vmware/govmomi/pbm/types"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1a6 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// FindBootDiskVolumeStatus identifies the boot disk entry inside
// vm.Status.Volumes without relying on any dedicated "isBootDisk" marker.
//
// Detection order (first match wins):
//  1. A status entry whose Type == VolumeTypeClassic.  Classic disks are
//     OVF/template-deployed boot disks;
//  2. A status entry whose Name matches a spec.volumes entry that has
//     removable == false.  When a VM is deployed from an OVF image the
//     controller marks image disks as removable=false to protect them.
//  3. The first status entry whose Name matches any spec.volumes entry
//     (regardless of type).  Catches VMs that only have a single managed
//     data-disk listed in spec (unusual but possible in test fixtures).
//
// Returns nil when no boot disk can be identified (e.g. no volumes have
// been attached yet).
func FindBootDiskVolumeStatus(vm *vmopv1a6.VirtualMachine) *vmopv1a6.VirtualMachineVolumeStatus {
	// Pass 1 – Classic disk (OVF boot disk).
	for i := range vm.Status.Volumes {
		if vm.Status.Volumes[i].Type == vmopv1a6.VolumeTypeClassic {
			return &vm.Status.Volumes[i]
		}
	}

	// Build a name→index map for quick status lookups.
	statusIdx := make(map[string]int, len(vm.Status.Volumes))
	for i := range vm.Status.Volumes {
		statusIdx[vm.Status.Volumes[i].Name] = i
	}

	// Pass 2 – spec.volumes entry that is explicitly non-removable.
	for _, sv := range vm.Spec.Volumes {
		if sv.Removable != nil && !*sv.Removable {
			if i, ok := statusIdx[sv.Name]; ok {
				return &vm.Status.Volumes[i]
			}
		}
	}

	// Pass 3 – first spec.volumes entry present in status (fallback).
	for _, sv := range vm.Spec.Volumes {
		if i, ok := statusIdx[sv.Name]; ok {
			return &vm.Status.Volumes[i]
		}
	}

	return nil
}

// StorageClassNameForBootDisk resolves the Kubernetes StorageClass name that
// backs the boot disk identified by bootVol.
//
// Two paths are used depending on the volume type:
//
//   - Managed (PVC-backed): reads spec.volumes to find the PersistentVolumeClaim
//     claim name, fetches the PVC, and returns its spec.storageClassName.  This
//     is the common case for WCP VM-Service VMs whose boot disk is promoted to
//     an FCD and tracked as a PVC.  PBM is not used because QueryAssociatedProfiles
//     with "virtualDiskId" does not reliably return a profile for FCD-backed disks.
//
//   - Classic (OVF template disk, no PVC): queries PBM via QueryAssociatedProfiles
//     using the disk's device key and maps the returned profile ID to a
//     StorageClass by matching the "storagePolicyID" parameter.
func StorageClassNameForBootDisk(
	ctx context.Context,
	vimClient *vim25.Client,
	k8sClient ctrlclient.Client,
	vm *vmopv1a6.VirtualMachine,
	bootVol *vmopv1a6.VirtualMachineVolumeStatus,
) (string, error) {
	if bootVol.Type == vmopv1a6.VolumeTypeManaged {
		return storageClassNameForManagedDisk(ctx, k8sClient, vm, bootVol.Name)
	}
	// Classic disk: use PBM.
	deviceKey, err := diskDeviceKeyByUUID(ctx, vimClient, vm.Status.UniqueID, bootVol.DiskUUID)
	if err != nil {
		return "", err
	}
	profileID, err := pbmProfileIDForDisk(ctx, vimClient, vm.Status.UniqueID, deviceKey)
	if err != nil {
		return "", err
	}
	if profileID == "" {
		return "", fmt.Errorf("PBM returned no storage profile for classic boot disk %q on VM %s/%s",
			bootVol.Name, vm.Namespace, vm.Name)
	}
	candidates, err := storageClassNameForPolicyID(ctx, k8sClient, profileID)
	if err != nil {
		return "", err
	}

	// Honour the class the user explicitly requested.
	for _, c := range candidates {
		if c == vm.Spec.StorageClass {
			return c, nil
		}
	}
	return "", fmt.Errorf("StorageClass %q not found among candidates %v for storage policy ID %q on VM %s/%s",
		vm.Spec.StorageClass, candidates, profileID, vm.Namespace, vm.Name)
}

// storageClassNameForManagedDisk finds the spec.volumes entry matching volName,
// fetches its backing PVC, and returns the PVC's spec.storageClassName.
func storageClassNameForManagedDisk(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1a6.VirtualMachine,
	volName string,
) (string, error) {
	// Find the matching spec.volumes entry to get the PVC claim name.
	var claimName string
	for _, sv := range vm.Spec.Volumes {
		if sv.Name == volName && sv.PersistentVolumeClaim != nil {
			claimName = sv.PersistentVolumeClaim.ClaimName
			break
		}
	}
	if claimName == "" {
		return "", fmt.Errorf("no PersistentVolumeClaim found in spec.volumes for volume %q on VM %s/%s",
			volName, vm.Namespace, vm.Name)
	}

	var pvc corev1.PersistentVolumeClaim
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Namespace: vm.Namespace,
		Name:      claimName,
	}, &pvc); err != nil {
		return "", fmt.Errorf("get PVC %s/%s for boot disk %q: %w",
			vm.Namespace, claimName, volName, err)
	}

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		return "", fmt.Errorf("PVC %s/%s for boot disk %q has no storageClassName",
			vm.Namespace, claimName, volName)
	}

	return *pvc.Spec.StorageClassName, nil
}

// diskDeviceKeyByUUID retrieves the vSphere device key for a virtual disk
// identified by its backing UUID.  It fetches config.hardware.device from the
// VM via the property collector and searches for a VirtualDisk whose
// VirtualDiskFlatVer2BackingInfo (or similar) reports the given UUID.
func diskDeviceKeyByUUID(
	ctx context.Context,
	vimClient *vim25.Client,
	vmMoID string,
	diskUUID string,
) (int32, error) {
	pc := property.DefaultCollector(vimClient)
	moRef := vimtypes.ManagedObjectReference{Type: "VirtualMachine", Value: vmMoID}

	var vmMO mo.VirtualMachine
	if err := pc.RetrieveOne(ctx, moRef, []string{"config.hardware.device"}, &vmMO); err != nil {
		return 0, fmt.Errorf("retrieve VM hardware devices for %s: %w", vmMoID, err)
	}
	if vmMO.Config == nil {
		return 0, fmt.Errorf("VM %s has no config", vmMoID)
	}

	normalised := strings.ToLower(diskUUID)

	for _, dev := range object.VirtualDeviceList(vmMO.Config.Hardware.Device).SelectByType((*vimtypes.VirtualDisk)(nil)) {
		disk := dev.(*vimtypes.VirtualDisk)
		if uuid := virtualDiskBackingUUID(disk); strings.ToLower(uuid) == normalised {
			return disk.Key, nil
		}
	}

	return 0, fmt.Errorf("no virtual disk with UUID %q found on VM %s", diskUUID, vmMoID)
}

// virtualDiskBackingUUID extracts the UUID from the most common virtual disk
// backing types used on WCP supervisor clusters.
func virtualDiskBackingUUID(disk *vimtypes.VirtualDisk) string {
	switch b := disk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		return b.Uuid
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		return b.Uuid
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		return b.Uuid
	case *vimtypes.VirtualDiskRawDiskVer2BackingInfo:
		return b.Uuid
	}
	return ""
}

// pbmProfileIDForDisk queries the vSphere Storage Policy Service (PBM) for
// the profile assigned to a specific virtual disk identified by its device key
// on the given VM.
//
// The PBM objectRef key format for a virtual disk is "<vm-moid>:<device-key>".
// See https://developer.broadcom.com/xapis/vmware-storage-policy-api/latest/pbm.ServerObjectRef.html
func pbmProfileIDForDisk(
	ctx context.Context,
	vimClient *vim25.Client,
	vmMoID string,
	deviceKey int32,
) (string, error) {
	pbmClient, err := pbm.NewClient(ctx, vimClient)
	if err != nil {
		return "", fmt.Errorf("create PBM client: %w", err)
	}

	ref := pbmtypes.PbmServerObjectRef{
		ObjectType: "virtualDiskId",
		Key:        fmt.Sprintf("%s:%d", vmMoID, deviceKey),
	}

	results, err := pbmClient.QueryAssociatedProfiles(ctx, []pbmtypes.PbmServerObjectRef{ref})
	if err != nil {
		return "", fmt.Errorf("PBM QueryAssociatedProfiles for disk %s:%d: %w", vmMoID, deviceKey, err)
	}

	for _, r := range results {
		for _, p := range r.ProfileId {
			if p.UniqueId != "" {
				return p.UniqueId, nil
			}
		}
	}

	return "", nil
}

// storageClassNameForPolicyID scans all StorageClass objects visible to
// k8sClient and returns the names of all StorageClasses whose "storagePolicyID"
// parameter matches profileID.  The caller is responsible for selecting the
// most appropriate candidate (e.g. by honouring the user's spec.storageClass).
func storageClassNameForPolicyID(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	profileID string,
) ([]string, error) {
	var list storagev1.StorageClassList
	if err := k8sClient.List(ctx, &list); err != nil {
		return nil, fmt.Errorf("list StorageClasses: %w", err)
	}

	var matches []string
	for _, sc := range list.Items {
		if sc.Parameters["storagePolicyID"] == profileID {
			matches = append(matches, sc.Name)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no StorageClass found for storage policy ID %q", profileID)
	}
	return matches, nil
}
