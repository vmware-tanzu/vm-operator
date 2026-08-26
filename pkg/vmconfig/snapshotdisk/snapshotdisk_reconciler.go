// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package snapshotdisk

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcond "github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
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

	if err := attachSnapshotDisks(ctx, k8sClient, vimClient, vm, moVM, configSpec); err != nil {
		return err
	}

	// Detach obsolete snapshot disks that are no longer in Spec.Volumes.
	detachSnapshotDisks(moVM, vm, configSpec)

	return nil
}

// attachSnapshotDisks attaches newly requested snapshot disks in configSpec and ensures required controllers.
func attachSnapshotDisks(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	controllerMapFns ...controllerMapFn,
) error {
	if vm == nil {
		return nil
	}

	var snapshotDisks []vmopv1.VirtualMachineVolume
	for _, vol := range vm.Spec.Volumes {
		if vol.VirtualMachineSnapshotDisk != nil {
			snapshotDisks = append(snapshotDisks, vol)
		}
	}

	// Lazy evaluator: build controllerMap from moVM at most once in a reconcile cycle,
	// only if and when needed.
	controllerMapFn := resolveControllerMapFn(moVM, controllerMapFns)

	newDeviceKey := int32(-100)
	hasNewDisks := false

	snapshotCache := make(map[string]snapshotFetchResult)

	// Save original status of snapshot disks so that if any volume encounters an error
	// and reconciliation terminates early, any newly attached status from previous volumes
	// in this reconcile cycle can be rolled back, preventing phantom attachment.
	origVolStatus := make(map[string]*vmopv1.VirtualMachineVolumeStatus)
	for _, vol := range snapshotDisks {
		for _, s := range vm.Status.Volumes {
			if s.Name == vol.Name {
				origVolStatus[vol.Name] = s.DeepCopy()
				break
			}
		}
	}

	var newlyAddedVols []vmopv1.VirtualMachineVolume

	// Attach newly requested snapshot disks.
	for _, vol := range snapshotDisks {
		added, err := addSnapshotDiskInConfigSpec(ctx, k8sClient, vimClient, vm, moVM, configSpec, vol, &newDeviceKey, snapshotCache, controllerMapFn)
		if err != nil {
			// Roll back any previously added volumes in this batch that were prematurely marked attached,
			// because Reconfigure will not be executed and the underlying hardware does not exist.
			for _, prevVol := range newlyAddedVols {
				if orig, exists := origVolStatus[prevVol.Name]; exists && orig != nil {
					updateSnapshotDiskVolumeStatus(vm, prevVol, orig.Attached, orig.DiskUUID, orig.Error)
				} else {
					vm.Status.Volumes = slices.DeleteFunc(vm.Status.Volumes, func(s vmopv1.VirtualMachineVolumeStatus) bool {
						return s.Name == prevVol.Name
					})
				}
			}
			return err
		}
		if added {
			newlyAddedVols = append(newlyAddedVols, vol)
			hasNewDisks = true
		}
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

	return nil
}

// diskBackingInfo holds normalized backing properties and a helper to instantiate child delta backings.
type diskBackingInfo struct {
	uuid          string
	createBacking func() vimtypes.BaseVirtualDeviceBackingInfo
}

// getDiskBackingInfo extracts backing metadata and the child backing constructor from a VirtualDisk.
func getDiskBackingInfo(disk *vimtypes.VirtualDisk) (*diskBackingInfo, error) {
	if disk == nil {
		return nil, fmt.Errorf("virtual disk is nil")
	}
	if disk.Backing == nil {
		return nil, fmt.Errorf("virtual disk backing is nil")
	}

	var info *diskBackingInfo
	switch backing := disk.Backing.(type) {
	case *vimtypes.VirtualDiskFlatVer2BackingInfo:
		info = &diskBackingInfo{
			uuid: backing.Uuid,
			createBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskFlatVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	case *vimtypes.VirtualDiskSeSparseBackingInfo:
		info = &diskBackingInfo{
			uuid: backing.Uuid,
			createBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskSeSparseBackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	case *vimtypes.VirtualDiskSparseVer2BackingInfo:
		info = &diskBackingInfo{
			uuid: backing.Uuid,
			createBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskSparseVer2BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	case *vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo:
		info = &diskBackingInfo{
			uuid: backing.Uuid,
			createBacking: func() vimtypes.BaseVirtualDeviceBackingInfo {
				return &vimtypes.VirtualDiskRawDiskMappingVer1BackingInfo{
					VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{FileName: backing.FileName},
					DiskMode:                     string(vimtypes.VirtualDiskModeIndependent_nonpersistent),
					Parent:                       backing,
					Uuid:                         backing.Uuid,
				}
			},
		}
	default:
		return nil, fmt.Errorf("unsupported disk backing type %T", disk.Backing)
	}

	if info.uuid == "" {
		return nil, fmt.Errorf("virtual disk backing UUID is empty")
	}

	return info, nil
}

func getVirtualDiskUUID(disk *vimtypes.VirtualDisk) (string, error) {
	info, err := getDiskBackingInfo(disk)
	if err != nil {
		return "", err
	}
	return info.uuid, nil
}

// findDiskInSnapshot finds a disk in moSnap matching the given diskID,
// returning both the VirtualDisk and its parsed diskBackingInfo.
func findDiskInSnapshot(moSnap *mo.VirtualMachineSnapshot, diskID string) (*vimtypes.VirtualDisk, *diskBackingInfo) {
	if moSnap == nil {
		return nil, nil
	}

	for _, dev := range moSnap.Config.Hardware.Device {
		disk, ok := dev.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}
		info, err := getDiskBackingInfo(disk)
		if err == nil && strings.EqualFold(info.uuid, diskID) {
			return disk, info
		}
	}

	return nil, nil
}

func updateSnapshotDiskVolumeStatus(
	vm *vmopv1.VirtualMachine,
	vol vmopv1.VirtualMachineVolume,
	attached bool,
	diskUUID string,
	errMsg string,
) {
	if vm == nil {
		return
	}

	for i, volStatus := range vm.Status.Volumes {
		if volStatus.Name == vol.Name {
			s := &vm.Status.Volumes[i]
			s.Type = vmopv1.VolumeTypeVirtualMachineSnapshotDisk
			s.Attached = attached
			s.Error = errMsg
			if diskUUID != "" {
				s.DiskUUID = diskUUID
			}
			if vol.UnitNumber != nil {
				s.UnitNumber = vol.UnitNumber
			}
			if vol.ControllerType != "" {
				s.ControllerType = vol.ControllerType
			}
			if vol.ControllerBusNumber != nil {
				s.ControllerBusNumber = vol.ControllerBusNumber
			}
			if vol.DiskMode != "" {
				s.DiskMode = vol.DiskMode
			}
			if vol.SharingMode != "" {
				s.SharingMode = vol.SharingMode
			}
			return
		}
	}

	vm.Status.Volumes = append(vm.Status.Volumes, vmopv1.VirtualMachineVolumeStatus{
		Name:                vol.Name,
		Type:                vmopv1.VolumeTypeVirtualMachineSnapshotDisk,
		Attached:            attached,
		DiskUUID:            diskUUID,
		UnitNumber:          vol.UnitNumber,
		ControllerType:      vol.ControllerType,
		ControllerBusNumber: vol.ControllerBusNumber,
		DiskMode:            vol.DiskMode,
		SharingMode:         vol.SharingMode,
		Error:               errMsg,
	})
}

func markSnapshotDiskFailed(ctx context.Context, vm *vmopv1.VirtualMachine, vol vmopv1.VirtualMachineVolume, errMsg string) {
	pkglog.FromContextOrDefault(ctx).Error(errors.New(errMsg), "Failed to reconcile snapshot disk", "volumeName", vol.Name)
	updateSnapshotDiskVolumeStatus(vm, vol, false, "", errMsg)
}

func markSnapshotDiskAttached(_ context.Context, vm *vmopv1.VirtualMachine, vol vmopv1.VirtualMachineVolume, diskUUID string) {
	updateSnapshotDiskVolumeStatus(vm, vol, true, diskUUID, "")
}

// getControllerMapFromMoVM builds a mapping from controller device key to ControllerID
// for all controllers found in moVM.Config.Hardware.Device.
func getControllerMapFromMoVM(moVM mo.VirtualMachine) map[int32]pkgutil.ControllerID {
	controllerMap := make(map[int32]pkgutil.ControllerID)
	if moVM.Config != nil {
		for _, dev := range moVM.Config.Hardware.Device {
			if ctrlID, ok := pkgutil.GetControllerIDFromDevice(dev); ok {
				controllerMap[dev.GetVirtualDevice().Key] = ctrlID
			}
		}
	}
	return controllerMap
}

// controllerMapFn is a function that returns a map of controller device key to ControllerID.
type controllerMapFn func() map[int32]pkgutil.ControllerID

// newControllerMapFn returns a lazy evaluator that builds controllerMap from moVM
// at most once in a reconcile cycle, only if and when called.
func newControllerMapFn(moVM mo.VirtualMachine) controllerMapFn {
	return sync.OnceValue(func() map[int32]pkgutil.ControllerID {
		return getControllerMapFromMoVM(moVM)
	})
}

func resolveControllerMapFn(moVM mo.VirtualMachine, controllerMapFns []controllerMapFn) controllerMapFn {
	if len(controllerMapFns) > 0 && controllerMapFns[0] != nil {
		return controllerMapFns[0]
	}
	return newControllerMapFn(moVM)
}

// isSnapshotDiskAttachedInVMMo checks whether there is a device in moVM.Config.Hardware.Device
// that matches the disk UUID and slot info (UnitNumber, ControllerType, ControllerBusNumber) of vol.
func isSnapshotDiskAttachedInVMMo(
	moVM mo.VirtualMachine,
	vol vmopv1.VirtualMachineVolume,
	controllerMapFns ...controllerMapFn,
) bool {
	if vol.VirtualMachineSnapshotDisk == nil {
		return false
	}
	targetUUID := vol.VirtualMachineSnapshotDisk.DiskID
	if targetUUID == "" || moVM.Config == nil {
		return false
	}

	controllerMapFn := resolveControllerMapFn(moVM, controllerMapFns)

	for _, dev := range moVM.Config.Hardware.Device {
		disk, ok := dev.(*vimtypes.VirtualDisk)
		if !ok || disk == nil {
			continue
		}

		// 1. Check disk UUID
		diskUUID, err := getVirtualDiskUUID(disk)
		if err != nil || !strings.EqualFold(diskUUID, targetUUID) {
			continue
		}

		// 2. Check slot: unit number
		if vol.UnitNumber != nil && !ptr.Equal(disk.UnitNumber, vol.UnitNumber) {
			continue
		}

		// 3. Check slot: controller info
		diskCtrlID := getDiskControllerID(disk, controllerMapFn())
		if vol.ControllerType != "" && !strings.EqualFold(string(diskCtrlID.ControllerType), string(vol.ControllerType)) {
			continue
		}
		if vol.ControllerBusNumber != nil && *vol.ControllerBusNumber != diskCtrlID.BusNumber {
			continue
		}

		return true
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

// isSnapshotDiskVolumeAttachedInStatus checks whether the volume is already in vm.Status.Volumes
// with matching name, uuid, and slot, and attached=true.
func isSnapshotDiskVolumeAttachedInStatus(
	vm *vmopv1.VirtualMachine,
	vol vmopv1.VirtualMachineVolume,
	diskID string,
) bool {
	if vm == nil {
		return false
	}
	for i, volStatus := range vm.Status.Volumes {
		if volStatus.Name != vol.Name {
			continue
		}
		if !volStatus.Attached {
			return false
		}
		if !strings.EqualFold(volStatus.DiskUUID, diskID) {
			return false
		}
		if !isSnapshotDiskSlotMatch(vol, volStatus) {
			return false
		}
		if volStatus.Error != "" {
			vm.Status.Volumes[i].Error = ""
		}
		return true
	}
	return false
}

// isSnapshotDiskSlotMatch checks whether the slot (controller, bus, unit number) of vol matches volStatus.
// If the volume spec specifies a particular unit number, bus number, or controller type, the status must match it.
// If the spec does not specify them, any assigned slot in status is considered a match.
func isSnapshotDiskSlotMatch(vol vmopv1.VirtualMachineVolume, volStatus vmopv1.VirtualMachineVolumeStatus) bool {
	if vol.UnitNumber != nil {
		if volStatus.UnitNumber == nil || *volStatus.UnitNumber != *vol.UnitNumber {
			return false
		}
	}
	if vol.ControllerBusNumber != nil {
		if volStatus.ControllerBusNumber == nil || *volStatus.ControllerBusNumber != *vol.ControllerBusNumber {
			return false
		}
	}
	if vol.ControllerType != "" {
		if volStatus.ControllerType != "" && volStatus.ControllerType != vol.ControllerType {
			return false
		}
	}
	return true
}

// addSnapshotDiskInConfigSpec ensures that a snapshot disk corresponding to vol is attached to the VM.
// If the volume is already recorded as attached in vm.Status.Volumes (matching name, uuid, and slot),
// it considers the volume already attached and skips fetching snapshot hardware.
// If the volume is already attached in moVM.Config.Hardware.Device (matching uuid and slot),
// it marks the volume as attached in status and skips fetching snapshot hardware.
// Otherwise, it fetches the snapshot hardware, prepares a child delta backing, and appends an Add device change to configSpec.
func addSnapshotDiskInConfigSpec(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vol vmopv1.VirtualMachineVolume,
	newDeviceKey *int32,
	snapshotCache map[string]snapshotFetchResult,
	controllerMapFns ...controllerMapFn,
) (bool, error) {
	if vol.VirtualMachineSnapshotDisk == nil {
		return false, nil
	}

	snapName := vol.VirtualMachineSnapshotDisk.Name
	diskID := vol.VirtualMachineSnapshotDisk.DiskID

	controllerMapFn := resolveControllerMapFn(moVM, controllerMapFns)

	// Check whether the volume is already attached in moVM.Config.Hardware.Device.
	// Hardware is the single source of truth for disk attachment.
	if isSnapshotDiskAttachedInVMMo(moVM, vol, controllerMapFn) {
		if !isSnapshotDiskVolumeAttachedInStatus(vm, vol, diskID) {
			markSnapshotDiskAttached(ctx, vm, vol, diskID)
		}
		return false, nil
	}

	snapNamespace := vm.Namespace
	cacheKey := fmt.Sprintf("%s/%s", snapNamespace, snapName)

	fetchRes, ok := snapshotCache[cacheKey]
	if !ok {
		fetchRes = fetchSnapshotHardware(ctx, k8sClient, vimClient, snapNamespace, snapName)
		snapshotCache[cacheKey] = fetchRes
	}

	if fetchRes.err != nil {
		markSnapshotDiskFailed(ctx, vm, vol, fetchRes.err.Error())
		if fetchRes.isTransient {
			return false, fetchRes.err
		}
		return false, nil
	}

	targetDisk, backingInfo := findDiskInSnapshot(fetchRes.moSnap, diskID)
	if targetDisk == nil || backingInfo == nil {
		markSnapshotDiskFailed(ctx, vm, vol, fmt.Sprintf("disk %s not found in VirtualMachineSnapshot %s", diskID, snapName))
		return false, nil
	}

	newBacking := backingInfo.createBacking()
	if newBacking == nil {
		err := fmt.Errorf("failed to create child backing for disk %s", diskID)
		markSnapshotDiskFailed(ctx, vm, vol, err.Error())
		return false, err
	}

	controllerKey, err := findControllerKeyForSnapshotDisk(vm, moVM, vol, controllerMapFn)
	if err != nil {
		markSnapshotDiskFailed(ctx, vm, vol, err.Error())
		return false, err
	}

	newDisk := &vimtypes.VirtualDisk{
		CapacityInBytes: targetDisk.CapacityInBytes,
		VirtualDevice: vimtypes.VirtualDevice{
			Key:           *newDeviceKey,
			Backing:       newBacking,
			ControllerKey: controllerKey,
		},
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

	markSnapshotDiskAttached(ctx, vm, vol, diskID)
	return true, nil
}

// findControllerKeyForSnapshotDisk determines the controller device key for attaching a snapshot disk
// by mapping (vol.ControllerType, vol.ControllerBusNumber) via vm.Status.Hardware.Controllers,
// following the controller mapping pattern in controllers/virtualmachine/volumebatch/volumebatch_controller.go,
// with fallback to moVM.Config.Hardware.Device if status is not yet populated.
func findControllerKeyForSnapshotDisk(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	vol vmopv1.VirtualMachineVolume,
	controllerMapFns ...controllerMapFn,
) (int32, error) {
	ctrlType := vol.ControllerType
	if ctrlType == "" {
		ctrlType = vmopv1.VirtualControllerTypeSCSI
	}

	var busNumber int32
	if vol.ControllerBusNumber != nil {
		busNumber = *vol.ControllerBusNumber
	}

	targetID := pkgutil.ControllerID{
		ControllerType: ctrlType,
		BusNumber:      busNumber,
	}

	// 1. Map via vm.Status.Hardware.Controllers (pattern from volumebatch_controller.go)
	if vm != nil && vm.Status.Hardware != nil && len(vm.Status.Hardware.Controllers) > 0 {
		ctrlDevKeyMap := make(map[pkgutil.ControllerID]int32)
		for _, ctrlStatus := range vm.Status.Hardware.Controllers {
			ctrlDevKeyMap[pkgutil.ControllerID{
				ControllerType: ctrlStatus.Type,
				BusNumber:      ctrlStatus.BusNumber,
			}] = ctrlStatus.DeviceKey
		}

		if ctrlDevKey, ok := ctrlDevKeyMap[targetID]; ok {
			return ctrlDevKey, nil
		}
	}

	// 2. Fallback to moVM.Config.Hardware.Device if not found in status
	if moVM.Config != nil {
		controllerMapFn := resolveControllerMapFn(moVM, controllerMapFns)
		for devKey, ctrlID := range controllerMapFn() {
			if ctrlID == targetID {
				return devKey, nil
			}
		}
	}

	return 0, fmt.Errorf("waiting for device controller %s %d to be created for volume %s", targetID.ControllerType, targetID.BusNumber, vol.Name)
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

// isSnapshotDiskVolumeInSpec checks whether a status snapshot volume is still present in spec.volumes
// by matching volume name, diskuuid, and slot (UnitNumber and controller info).
func isSnapshotDiskVolumeInSpec(statusVol vmopv1.VirtualMachineVolumeStatus, specVolumes []vmopv1.VirtualMachineVolume) bool {
	for _, specVol := range specVolumes {
		if specVol.VirtualMachineSnapshotDisk == nil {
			continue
		}

		// 1. Match volume name
		if specVol.Name != statusVol.Name {
			continue
		}

		// 2. Match diskuuid
		if statusVol.DiskUUID != "" && !strings.EqualFold(specVol.VirtualMachineSnapshotDisk.DiskID, statusVol.DiskUUID) {
			continue
		}

		// 3. Match slot: only consider it a mismatch if both spec and status have slot defined and they differ.
		if specVol.UnitNumber != nil && statusVol.UnitNumber != nil && *specVol.UnitNumber != *statusVol.UnitNumber {
			continue
		}
		if specVol.ControllerBusNumber != nil && statusVol.ControllerBusNumber != nil && *specVol.ControllerBusNumber != *statusVol.ControllerBusNumber {
			continue
		}
		if specVol.ControllerType != "" && statusVol.ControllerType != "" && !strings.EqualFold(string(specVol.ControllerType), string(statusVol.ControllerType)) {
			continue
		}

		return true
	}
	return false
}

// findSnapshotDiskInHardware finds a disk in moVM.Config.Hardware.Device matching slot and diskuuid of statusVol.
func findSnapshotDiskInHardware(
	moVM mo.VirtualMachine,
	statusVol vmopv1.VirtualMachineVolumeStatus,
	controllerMap map[int32]pkgutil.ControllerID,
) *vimtypes.VirtualDisk {
	if moVM.Config == nil || statusVol.DiskUUID == "" {
		return nil
	}

	for _, dev := range moVM.Config.Hardware.Device {
		disk, ok := dev.(*vimtypes.VirtualDisk)
		if !ok {
			continue
		}

		// Check diskuuid
		diskUUID, err := getVirtualDiskUUID(disk)
		if err != nil || !strings.EqualFold(diskUUID, statusVol.DiskUUID) {
			continue
		}

		// Check slot: unit number
		if statusVol.UnitNumber != nil && !ptr.Equal(disk.UnitNumber, statusVol.UnitNumber) {
			continue
		}

		// Check slot: controller info
		diskCtrlID := getDiskControllerID(disk, controllerMap)
		if statusVol.ControllerType != "" && statusVol.ControllerType != diskCtrlID.ControllerType {
			continue
		}
		if statusVol.ControllerBusNumber != nil && *statusVol.ControllerBusNumber != diskCtrlID.BusNumber {
			continue
		}

		return disk
	}

	return nil
}

// detachSnapshotDisks iterates through all volumes with VolumeTypeVirtualMachineSnapshotDisk
// in status.volumes to check if they still exist in spec.volumes (by volume name, diskuuid, and slot).
// If a snapshot volume is in status.volumes but not in spec.volumes, it checks whether the volume is
// present in moVM.Config.Hardware.Device (by slot and diskuuid). If found, it appends a Remove
// device change to configSpec. In either case, the volume's Attached field in status is set to false.
func detachSnapshotDisks(
	moVM mo.VirtualMachine,
	vm *vmopv1.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	controllerMapFns ...controllerMapFn,
) {
	if vm == nil {
		return
	}

	controllerMapFn := resolveControllerMapFn(moVM, controllerMapFns)
	removedDeviceKeys := make(map[int32]struct{})

	for i := range vm.Status.Volumes {
		statusVol := &vm.Status.Volumes[i]
		if statusVol.Type != vmopv1.VolumeTypeVirtualMachineSnapshotDisk {
			continue
		}

		// Check whether the volume is in spec.volumes (by volume name, diskuuid, slot)
		if isSnapshotDiskVolumeInSpec(*statusVol, vm.Spec.Volumes) {
			continue
		}

		// The snapshot volume is in status.volumes but not in spec.volumes.
		// Check whether the volume is in moVM.Config.Hardware.Device (by checking slot and diskuuid).
		if disk := findSnapshotDiskInHardware(moVM, *statusVol, controllerMapFn()); disk != nil {
			if _, alreadyRemoved := removedDeviceKeys[disk.Key]; !alreadyRemoved {
				removedDeviceKeys[disk.Key] = struct{}{}
				configSpec.DeviceChange = append(configSpec.DeviceChange, &vimtypes.VirtualDeviceConfigSpec{
					Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					Device:    disk,
				})
			}
		}

		// Mark volume as detached in status
		statusVol.Attached = false
	}
}
