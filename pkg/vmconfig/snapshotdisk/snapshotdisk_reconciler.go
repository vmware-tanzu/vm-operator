// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package snapshotdisk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for a VM's snapshot disks.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Name returns the unique name used to identify the reconciler.
func (r reconciler) Name() string {
	return "snapshotdisk"
}

func (r reconciler) OnResult(
	_ context.Context,
	_ *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
	_ error,
) error {
	return nil
}

// Reconcile configures the VM's snapshot disks.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
) error {
	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

func (r reconciler) Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
) error {
	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vimClient == nil {
		panic("vimClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}

	// CSIBackupAPI feature flag gates snapshot volume attachment.
	if !pkgcfg.FromContext(ctx).Features.CSIBackupAPI {
		for _, vol := range vm.Spec.Volumes {
			if vol.VirtualMachineSnapshot != nil {
				markSnapshotDiskFailed(ctx, vm, vol.Name, "VirtualMachineSnapshot volume source is not supported because CSIBackupAPI feature is disabled")
			}
		}
		return nil
	}

	var snapshotDisks []vmopv1.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if vol.VirtualMachineSnapshot != nil {
			snapshotDisks = append(snapshotDisks, vol)
		}
	}

	newDeviceKey := int32(-100)
	hasNewDisks := false
	var reterr error

	snapshotCache := make(map[string]snapshotFetchResult)

	// Attach newly requested snapshot disks.
	for _, vol := range snapshotDisks {
		added, err := ensureSnapshotDiskAttached(ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, snapshotCache)
		if err != nil && reterr == nil {
			reterr = err
		}
		if added {
			hasNewDisks = true
		}
	}

	if reterr != nil {
		return reterr
	}

	// Allow duplicate disk UUIDs and ensure controllers for newly added snapshot disks.
	if hasNewDisks {
		configSpec.ExtraConfig = append(configSpec.ExtraConfig, &vimtypes.OptionValue{
			Key:   vsphereconst.AllowDupDiskUUIDExtraConfigKey,
			Value: vsphereconst.ExtraConfigTrue,
		})

		var devices []vimtypes.BaseVirtualDevice
		if moVM.Config != nil {
			devices = moVM.Config.Hardware.Device
		}
		if err := pkgutil.EnsureDisksHaveControllers(configSpec, devices...); err != nil {
			pkglog.FromContextOrDefault(ctx).Error(err, "Failed to ensure controllers for snapshot disks")
			return fmt.Errorf("failed to ensure controllers for snapshot disks: %w", err)
		}
	}

	// Detach obsolete snapshot disks that are no longer in Spec.Volumes.
	removeObsoleteSnapshotDisks(moVM, vm, snapshotDisks, configSpec)

	return nil
}

// diskBackingInfo holds normalized backing properties and a helper to instantiate child delta backings.
type diskBackingInfo struct {
	UUID               string
	DiskMode           string
	FileName           string
	HasParent          bool
	ParentFileName     string
	CreateChildBacking func() vimtypes.BaseVirtualDeviceBackingInfo
}

// getDiskBackingInfo extracts backing metadata and the child backing constructor from a VirtualDisk.
func getDiskBackingInfo(disk *vimtypes.VirtualDisk) *diskBackingInfo {
	if disk == nil || disk.Backing == nil {
		return nil
	}

	switch backing := disk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		var parentFileName string
		if backing.Parent != nil {
			parentFileName = backing.Parent.FileName
		}
		return &diskBackingInfo{
			UUID:           backing.Uuid,
			DiskMode:       backing.DiskMode,
			FileName:       backing.FileName,
			HasParent:      backing.Parent != nil,
			ParentFileName: parentFileName,
			CreateChildBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		var parentFileName string
		if backing.Parent != nil {
			parentFileName = backing.Parent.FileName
		}
		return &diskBackingInfo{
			UUID:           backing.Uuid,
			DiskMode:       backing.DiskMode,
			FileName:       backing.FileName,
			HasParent:      backing.Parent != nil,
			ParentFileName: parentFileName,
			CreateChildBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskSeSparseBackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		var parentFileName string
		if backing.Parent != nil {
			parentFileName = backing.Parent.FileName
		}
		return &diskBackingInfo{
			UUID:           backing.Uuid,
			DiskMode:       backing.DiskMode,
			FileName:       backing.FileName,
			HasParent:      backing.Parent != nil,
			ParentFileName: parentFileName,
			CreateChildBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskSparseVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		var parentFileName string
		if backing.Parent != nil {
			parentFileName = backing.Parent.FileName
		}
		return &diskBackingInfo{
			UUID:           backing.Uuid,
			DiskMode:       backing.DiskMode,
			FileName:       backing.FileName,
			HasParent:      backing.Parent != nil,
			ParentFileName: parentFileName,
			CreateChildBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	default:
		return nil
	}
}

func getVirtualDiskUUID(disk *vimtypes.VirtualDisk) string {
	if info := getDiskBackingInfo(disk); info != nil {
		return info.UUID
	}
	return ""
}

// isDiskDerivedFromSnapshotDisk checks whether disk (from the VM's hardware) is a snapshot
// disk derived from the target snapshot disk targetDisk.
func isDiskDerivedFromSnapshotDisk(disk, targetDisk *vimtypes.VirtualDisk) bool {
	if disk == nil || targetDisk == nil {
		return false
	}

	diskInfo := getDiskBackingInfo(disk)
	targetInfo := getDiskBackingInfo(targetDisk)
	if diskInfo == nil || targetInfo == nil {
		return false
	}

	// 1. UUID must match.
	if !strings.EqualFold(diskInfo.UUID, targetInfo.UUID) {
		return false
	}

	// 2. Snapshot disks attached by vm-operator are always independent_nonpersistent.
	if !strings.EqualFold(diskInfo.DiskMode, string(vimtypes.VirtualDiskModeIndependent_nonpersistent)) {
		return false
	}

	// 3. Parent file name and target disk file name must both be non-empty and match.
	if diskInfo.ParentFileName == "" || targetInfo.FileName == "" {
		return false
	}

	return strings.EqualFold(diskInfo.ParentFileName, targetInfo.FileName)
}

// createSnapshotDiskBacking creates a child delta backing pointing to targetDisk as its parent.
func createSnapshotDiskBacking(targetDisk *vimtypes.VirtualDisk) (vimtypes.BaseVirtualDeviceBackingInfo, error) {
	if targetDisk == nil || targetDisk.Backing == nil {
		return nil, fmt.Errorf("target disk has no backing info")
	}

	info := getDiskBackingInfo(targetDisk)
	if info == nil {
		return nil, fmt.Errorf("unsupported disk backing type %T", targetDisk.Backing)
	}

	return info.CreateChildBacking(), nil
}

func markSnapshotDiskFailed(ctx context.Context, vm *vmopv1.VirtualMachine, volName string, errMsg string) {
	pkglog.FromContextOrDefault(ctx).Error(errors.New(errMsg), "Failed to reconcile snapshot disk", "volumeName", volName)
	for i, volStatus := range vm.Status.Volumes {
		if volStatus.Name == volName {
			vm.Status.Volumes[i].Type = vmopv1.VolumeTypeClassic
			vm.Status.Volumes[i].Error = errMsg
			vm.Status.Volumes[i].Attached = false
			return
		}
	}
	vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
		Name:     volName,
		Type:     vmopv1.VolumeTypeClassic,
		Attached: false,
		Error:    errMsg,
	})
}

func markSnapshotDiskAttached(_ context.Context, vm *vmopv1.VirtualMachine, volName, diskUUID string) {
	for i, volStatus := range vm.Status.Volumes {
		if volStatus.Name == volName {
			vm.Status.Volumes[i].Type = vmopv1.VolumeTypeClassic
			vm.Status.Volumes[i].Attached = true
			vm.Status.Volumes[i].DiskUUID = diskUUID
			vm.Status.Volumes[i].Error = ""
			return
		}
	}
	vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
		Name:     volName,
		Type:     vmopv1.VolumeTypeClassic,
		Attached: true,
		DiskUUID: diskUUID,
		Error:    "",
	})
}

// isSnapshotDiskAttached checks whether the snapshot disk derived from targetDisk is currently
// attached to the VM in vCenter (moVM) or scheduled to be added in the current configSpec.
// We intentionally inspect moVM hardware devices and pending configSpec rather than vm.Status.Volumes
// because moVM is the ground truth of the underlying VM hardware. Checking vm.Status alone can cause
// false negatives (re-adding an already-attached disk if status was not yet updated, causing vCenter errors)
// or false positives (skipping attach if status says attached but disk was detached out-of-band in vCenter).
func isSnapshotDiskAttached(moVM mo.VirtualMachine, configSpec *vimtypes.VirtualMachineConfigSpec, targetDisk *vimtypes.VirtualDisk) bool {
	if targetDisk == nil {
		return false
	}
	targetUUID := getVirtualDiskUUID(targetDisk)
	if targetUUID == "" {
		return false
	}

	if moVM.Config != nil {
		for _, dev := range moVM.Config.Hardware.Device {
			if disk, ok := dev.(*vimtypes.VirtualDisk); ok {
				if isDiskDerivedFromSnapshotDisk(disk, targetDisk) {
					isBeingRemoved := false
					for _, devChange := range configSpec.DeviceChange {
						if devChange.GetVirtualDeviceConfigSpec().Operation == vimtypes.VirtualDeviceConfigSpecOperationRemove {
							if removedDisk, ok := devChange.GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualDisk); ok {
								if removedDisk.Key == disk.Key {
									isBeingRemoved = true
									break
								}
							}
						}
					}

					if !isBeingRemoved {
						return true
					}
				}
			}
		}
	}

	for _, devChange := range configSpec.DeviceChange {
		if devChange.GetVirtualDeviceConfigSpec().Operation == vimtypes.VirtualDeviceConfigSpecOperationAdd {
			if disk, ok := devChange.GetVirtualDeviceConfigSpec().Device.(*vimtypes.VirtualDisk); ok {
				if isDiskDerivedFromSnapshotDisk(disk, targetDisk) {
					return true
				}
			}
		}
	}

	return false
}

type snapshotFetchResult struct {
	moSnap      *mo.VirtualMachineSnapshot
	err         error
	isTransient bool
}

var retrieveSnapshotHardware = defaultRetrieveSnapshotHardware

func defaultRetrieveSnapshotHardware(
	ctx context.Context,
	vimClient *vim25.Client,
	snapRef vimtypes.ManagedObjectReference,
) (*mo.VirtualMachineSnapshot, error) {
	var moSnap mo.VirtualMachineSnapshot
	pc := property.DefaultCollector(vimClient)
	if err := pc.RetrieveOne(ctx, snapRef, []string{"config.hardware.device"}, &moSnap); err != nil {
		return nil, fmt.Errorf("failed to fetch snapshot hardware configuration: %w", err)
	}
	return &moSnap, nil
}

// fetchSnapshotHardware retrieves the VirtualMachineSnapshot resource from Kubernetes
// and fetches the snapshot VM's hardware configuration from vCenter.
func fetchSnapshotHardware(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	snapNamespace, snapName string,
) snapshotFetchResult {
	snapshot := &vmopv1.VirtualMachineSnapshot{}
	key := ctrlclient.ObjectKey{
		Namespace: snapNamespace,
		Name:      snapName,
	}
	if err := k8sClient.Get(ctx, key, snapshot); err != nil {
		if apierrors.IsNotFound(err) {
			return snapshotFetchResult{err: fmt.Errorf("VirtualMachineSnapshot %s not found", snapName), isTransient: false}
		}
		return snapshotFetchResult{err: fmt.Errorf("failed to get VirtualMachineSnapshot %s: %w", snapName, err), isTransient: true}
	}

	if !pkgcond.IsTrue(snapshot, vmopv1.VirtualMachineSnapshotReadyCondition) {
		return snapshotFetchResult{err: fmt.Errorf("VirtualMachineSnapshot %s is not ready", snapName), isTransient: false}
	}

	if snapshot.Status.UniqueID == "" {
		return snapshotFetchResult{err: fmt.Errorf("VirtualMachineSnapshot %s has no UniqueID", snapName), isTransient: false}
	}

	snapRef := vimtypes.ManagedObjectReference{
		Type:  "VirtualMachineSnapshot",
		Value: snapshot.Status.UniqueID,
	}

	moSnap, err := retrieveSnapshotHardware(ctx, vimClient, snapRef)
	if err != nil {
		return snapshotFetchResult{err: err, isTransient: true}
	}

	return snapshotFetchResult{moSnap: moSnap}
}

// ensureSnapshotDiskAttached ensures that a snapshot disk corresponding to vol is attached to the VM.
// If not already attached, it prepares a child delta backing and appends an Add device change to configSpec.
func ensureSnapshotDiskAttached(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vol vmopv1.VirtualMachineVolume,
	newDeviceKey *int32,
	snapshotCache map[string]snapshotFetchResult,
) (bool, error) {
	snapName := vol.VirtualMachineSnapshot.Name
	diskID := vol.VirtualMachineSnapshot.DiskID
	snapNamespace := vm.Namespace
	cacheKey := fmt.Sprintf("%s/%s", snapNamespace, snapName)

	fetchRes, ok := snapshotCache[cacheKey]
	if !ok {
		fetchRes = fetchSnapshotHardware(ctx, k8sClient, vimClient, snapNamespace, snapName)
		snapshotCache[cacheKey] = fetchRes
	}

	if fetchRes.err != nil {
		markSnapshotDiskFailed(ctx, vm, vol.Name, fetchRes.err.Error())
		if fetchRes.isTransient {
			return false, fetchRes.err
		}
		return false, nil
	}

	moSnap := fetchRes.moSnap
	var targetDisk *vimtypes.VirtualDisk
	if moSnap != nil {
		for _, dev := range moSnap.Config.Hardware.Device {
			if disk, ok := dev.(*vimtypes.VirtualDisk); ok {
				if getVirtualDiskUUID(disk) == diskID {
					targetDisk = disk
					break
				}
			}
		}
	}

	if targetDisk == nil {
		markSnapshotDiskFailed(ctx, vm, vol.Name, fmt.Sprintf("disk %s not found in VirtualMachineSnapshot %s", diskID, snapName))
		return false, nil
	}

	attached := isSnapshotDiskAttached(moVM, configSpec, targetDisk)

	if !attached {
		newBacking, err := createSnapshotDiskBacking(targetDisk)
		if err != nil {
			markSnapshotDiskFailed(ctx, vm, vol.Name, fmt.Sprintf("failed to create snapshot disk backing: %v", err))
			return false, nil
		}

		newDisk := &vimtypes.VirtualDisk{
			CapacityInBytes: targetDisk.CapacityInBytes,
			VirtualDevice: vimtypes.VirtualDevice{
				Key:     *newDeviceKey,
				Backing: newBacking,
			},
		}
		if controllerKey := findControllerKeyForSnapshotDisk(moVM, configSpec, vol, targetDisk); controllerKey != 0 {
			newDisk.ControllerKey = controllerKey
		}
		if vol.UnitNumber != nil {
			unitNumber := *vol.UnitNumber
			newDisk.UnitNumber = &unitNumber
		}
		*newDeviceKey--

		configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			Device:    newDisk,
		})

		markSnapshotDiskAttached(ctx, vm, vol.Name, diskID)
		return true, nil
	}

	markSnapshotDiskAttached(ctx, vm, vol.Name, diskID)
	return false, nil
}

// findControllerKeyForSnapshotDisk determines the controller key for attaching a snapshot disk:
// 1. Matches explicit ControllerType and ControllerBusNumber from volume spec.
// 2. Matches controller key of the target disk if present on the VM.
// 3. Defaults to the first SCSI controller.
func findControllerKeyForSnapshotDisk(
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vol vmopv1.VirtualMachineVolume,
	targetDisk *vimtypes.VirtualDisk,
) int32 {
	type controllerInfo struct {
		key       int32
		busNumber int32
		ctrlType  vmopv1.VirtualControllerType
	}

	var controllers []controllerInfo

	collectController := func(dev vimtypes.BaseVirtualDevice) {
		if ctrlID, ok := pkgutil.GetControllerIDFromDevice(dev); ok {
			bc := dev.(vimtypes.BaseVirtualController).GetVirtualController()
			controllers = append(controllers, controllerInfo{
				key:       bc.Key,
				busNumber: ctrlID.BusNumber,
				ctrlType:  ctrlID.ControllerType,
			})
		}
	}

	if moVM.Config != nil {
		for _, dev := range moVM.Config.Hardware.Device {
			collectController(dev)
		}
	}

	for _, devChange := range configSpec.DeviceChange {
		dcs := devChange.GetVirtualDeviceConfigSpec()
		if dcs != nil && dcs.Operation == vimtypes.VirtualDeviceConfigSpecOperationAdd && dcs.Device != nil {
			collectController(dcs.Device)
		}
	}

	// 1. If volume specifies controller type and bus number, find matching controller.
	if vol.ControllerType != "" && vol.ControllerBusNumber != nil {
		for _, c := range controllers {
			if c.ctrlType == vol.ControllerType && c.busNumber == *vol.ControllerBusNumber {
				return c.key
			}
		}
	}

	// 2. If targetDisk has a controller key, check if that controller exists on this VM.
	if targetDisk != nil && targetDisk.ControllerKey != 0 {
		for _, c := range controllers {
			if c.key == targetDisk.ControllerKey {
				return c.key
			}
		}
	}

	// 3. Otherwise, default to the first SCSI controller (default volume controller type).
	for _, c := range controllers {
		if c.ctrlType == vmopv1.VirtualControllerTypeSCSI {
			return c.key
		}
	}

	return 0
}

// volumePlacement represents a device placement tuple (ControllerType, BusNumber, UnitNumber).
type volumePlacement struct {
	ControllerType vmopv1.VirtualControllerType
	BusNumber      int32
	UnitNumber     *int32
}

func getVolumeStatusPlacement(vs vmopv1.VirtualMachineVolumeStatus) volumePlacement {
	ctrlType := vs.ControllerType
	if ctrlType == "" {
		ctrlType = vmopv1.VirtualControllerTypeSCSI
	}
	var busNumber int32
	if vs.ControllerBusNumber != nil {
		busNumber = *vs.ControllerBusNumber
	}
	return volumePlacement{
		ControllerType: ctrlType,
		BusNumber:      busNumber,
		UnitNumber:     vs.UnitNumber,
	}
}

func getEffectivePlacement(vol vmopv1.VirtualMachineVolume, statusPlacementByName map[string]volumePlacement) volumePlacement {
	statusPlacement, hasStatus := statusPlacementByName[vol.Name]

	ctrlType := vol.ControllerType
	if ctrlType == "" && hasStatus {
		ctrlType = statusPlacement.ControllerType
	}
	if ctrlType == "" {
		ctrlType = vmopv1.VirtualControllerTypeSCSI
	}

	var busNumber int32
	if vol.ControllerBusNumber != nil {
		busNumber = *vol.ControllerBusNumber
	} else if hasStatus {
		busNumber = statusPlacement.BusNumber
	}

	unitNumber := vol.UnitNumber
	if unitNumber == nil && hasStatus {
		unitNumber = statusPlacement.UnitNumber
	}

	return volumePlacement{
		ControllerType: ctrlType,
		BusNumber:      busNumber,
		UnitNumber:     unitNumber,
	}
}

func getDiskControllerID(disk *vimtypes.VirtualDisk, controllerMap map[int32]pkgutil.ControllerID) pkgutil.ControllerID {
	if ctrlID, ok := controllerMap[disk.ControllerKey]; ok {
		return ctrlID
	}
	return pkgutil.ControllerID{
		ControllerType: vmopv1.VirtualControllerTypeSCSI,
		BusNumber:      0,
	}
}

func (p volumePlacement) matchesDisk(disk *vimtypes.VirtualDisk, diskCtrlID pkgutil.ControllerID) bool {
	if p.UnitNumber == nil || disk.UnitNumber == nil {
		return false
	}
	return *p.UnitNumber == *disk.UnitNumber &&
		p.BusNumber == diskCtrlID.BusNumber &&
		p.ControllerType == diskCtrlID.ControllerType
}

// isObsoleteSnapshotDisk determines whether an attached virtual disk is an obsolete snapshot disk:
// 1. Verifies physical delta disk attributes (independent_nonpersistent mode and parent backing).
// 2. Skips managed volumes (PVCs) to prevent accidental detachment.
// 3. Claims active snapshot volumes by slot placement first, then by unassigned slot.
func isObsoleteSnapshotDisk(
	disk *vimtypes.VirtualDisk,
	uuid string,
	diskCtrlID pkgutil.ControllerID,
	claimedSnapshotVols map[int]struct{},
	snapshotDisks []vmopv1.VirtualMachineVolume,
	statusPlacementByName map[string]volumePlacement,
	vm *vmopv1.VirtualMachine,
) bool {
	info := getDiskBackingInfo(disk)
	if info == nil {
		return false
	}

	// Snapshot disks attached by vm-operator are always independent_nonpersistent and have a parent backing.
	// Regular disks (boot disks, PVCs, linked clones) or standalone independent_nonpersistent disks must never be detached here.
	if !strings.EqualFold(info.DiskMode, string(vimtypes.VirtualDiskModeIndependent_nonpersistent)) || !info.HasParent {
		return false
	}

	// Managed volumes (PVCs) are managed by CNS and must never be detached here.
	if vm != nil {
		for _, vs := range vm.Status.Volumes {
			if vs.Type == vmopv1.VolumeTypeManaged {
				vsPlacement := getVolumeStatusPlacement(vs)
				if vsPlacement.matchesDisk(disk, diskCtrlID) {
					return false
				}
				if strings.EqualFold(vs.DiskUUID, uuid) {
					return false
				}
			}
		}

		for _, vol := range vm.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				volPlacement := getEffectivePlacement(vol, statusPlacementByName)
				if volPlacement.matchesDisk(disk, diskCtrlID) {
					return false
				}
			}
		}
	}

	// Check if there is an unclaimed volume in snapshotDisks matching this disk.
	// First try to match by placement if available (using effective placement from spec or status).
	if disk.UnitNumber != nil {
		for i, vol := range snapshotDisks {
			if vol.VirtualMachineSnapshot == nil || !strings.EqualFold(vol.VirtualMachineSnapshot.DiskID, uuid) {
				continue
			}
			if _, claimed := claimedSnapshotVols[i]; claimed {
				continue
			}
			effectivePlacement := getEffectivePlacement(vol, statusPlacementByName)
			if effectivePlacement.matchesDisk(disk, diskCtrlID) {
				claimedSnapshotVols[i] = struct{}{}
				return false
			}
		}
	}

	// If not matched by placement, match any unclaimed volume with the same DiskID
	// that does not have an effective UnitNumber assigned (e.g. newly added spec volume
	// not yet attached or observed). Volumes that already have an effective UnitNumber
	// must only match their designated slot and not claim a different slot.
	for i, vol := range snapshotDisks {
		if vol.VirtualMachineSnapshot == nil || !strings.EqualFold(vol.VirtualMachineSnapshot.DiskID, uuid) {
			continue
		}
		if _, claimed := claimedSnapshotVols[i]; claimed {
			continue
		}
		effectivePlacement := getEffectivePlacement(vol, statusPlacementByName)
		if effectivePlacement.UnitNumber == nil {
			claimedSnapshotVols[i] = struct{}{}
			return false
		}
	}

	return true
}

// removeObsoleteSnapshotDisks scans VM hardware devices and adds Remove device changes
// for obsolete snapshot disks that are no longer referenced in vm.Spec.Volumes.
func removeObsoleteSnapshotDisks(
	moVM mo.VirtualMachine,
	vm *vmopv1.VirtualMachine,
	snapshotDisks []vmopv1.VirtualMachineVolume,
	configSpec *vimtypes.VirtualMachineConfigSpec,
) {
	if moVM.Config == nil {
		return
	}

	claimedSnapshotVols := make(map[int]struct{})

	controllerMap := make(map[int32]pkgutil.ControllerID)
	for _, dev := range moVM.Config.Hardware.Device {
		if ctrlID, ok := pkgutil.GetControllerIDFromDevice(dev); ok {
			controllerMap[dev.GetVirtualDevice().Key] = ctrlID
		}
	}

	statusPlacementByName := make(map[string]volumePlacement)
	if vm != nil {
		for _, vs := range vm.Status.Volumes {
			statusPlacementByName[vs.Name] = getVolumeStatusPlacement(vs)
		}
	}

	for _, dev := range moVM.Config.Hardware.Device {
		disk, ok := dev.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}
		uuid := getVirtualDiskUUID(disk)
		if uuid == "" {
			continue
		}

		diskCtrlID := getDiskControllerID(disk, controllerMap)

		if isObsoleteSnapshotDisk(disk, uuid, diskCtrlID, claimedSnapshotVols, snapshotDisks, statusPlacementByName, vm) {
			configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
				Device:    disk,
			})

			if vm != nil {
				for i, volStatus := range vm.Status.Volumes {
					if strings.EqualFold(volStatus.DiskUUID, uuid) && volStatus.Type == vmopv1.VolumeTypeClassic {
						// Only mark as detached if it is no longer in vm.Spec.Volumes.
						// This prevents marking active regular volumes or other active snapshot volumes
						// with the same DiskUUID as detached.
						inSpec := false
						for _, v := range vm.Spec.Volumes {
							if v.Name == volStatus.Name {
								inSpec = true
								break
							}
						}
						if !inSpec {
							vm.Status.Volumes[i].Attached = false
						}
					}
				}
			}
		}
	}
}
