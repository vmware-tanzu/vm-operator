// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"
	"errors"
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

const (
	vmiKind  = "VirtualMachineImage"
	cvmiKind = "Cluster" + vmiKind
)

// UpdateCdromDeviceChanges reconciles the desired CD-ROM devices specified in
// VM.Spec.Cdrom with the current CD-ROM devices in the VM. It returns a list of
// device changes required to update the CD-ROM devices.
func UpdateCdromDeviceChanges(
	vmCtx pkgctx.VirtualMachineContext,
	restClient *rest.Client,
	k8sClient ctrlclient.Client,
	curDevices object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	if pkgcfg.FromContext(vmCtx).Features.VMSharedDisks {
		return updateCdromDeviceChangesWithSharedDisks(
			vmCtx, restClient, k8sClient, curDevices)
	}

	return updateCdromDeviceChangesLegacy(
		vmCtx, restClient, k8sClient, curDevices)
}

// updateCdromDeviceChangesWithSharedDisks implements CD-ROM reconciliation with
// placement change support (VMSharedDisks enabled).
func updateCdromDeviceChangesWithSharedDisks(
	vmCtx pkgctx.VirtualMachineContext,
	restClient *rest.Client,
	k8sClient ctrlclient.Client,
	curDevices object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	hw := vmCtx.VM.Spec.Hardware
	if hw == nil {
		return nil, nil
	}

	var (
		deviceChanges                  = make([]vimtypes.BaseVirtualDeviceConfigSpec, 0)
		curCdromBackingFileNameToSpec  = make(map[string]vmopv1.VirtualMachineCdromSpec)
		newCdromBackingFileNameToSpec  = make(map[string]vmopv1.VirtualMachineCdromSpec)
		expectedBackingFileNameToCdrom = make(map[string]vimtypes.BaseVirtualDevice, len(hw.Cdrom))
		libManager                     = library.NewManager(restClient)
		ctrlKeyToInfo                  = buildControllerKeyToInfoMap(curDevices)
	)

	for _, specCdrom := range hw.Cdrom {

		bFileName, cdrom, err := findCdromBySpec(
			vmCtx, k8sClient, specCdrom, curDevices, libManager)
		if err != nil {
			return nil, err
		}

		// Check if existing CD-ROM has placement changes.
		if cdrom != nil {
			placementChanged, err := hasControllerPlacementChanged(cdrom, specCdrom, ctrlKeyToInfo)
			if err != nil {
				return nil, err
			}

			if placementChanged {
				// Remove CD-ROM with outdated placement and treat as new.
				deviceChanges = append(deviceChanges,
					&vimtypes.VirtualDeviceConfigSpec{
						Device:    cdrom,
						Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
					})
			} else {
				// No placement change, update connection state later.
				curCdromBackingFileNameToSpec[bFileName] = specCdrom
				expectedBackingFileNameToCdrom[bFileName] = cdrom
				continue
			}
		}

		// Create new CD-ROM (doesn't exist or placement changed).
		var connected bool
		if specCdrom.Connected != nil {
			// A device can only be connected if the VM is powered on.
			connected = *specCdrom.Connected &&
				vmCtx.MoVM.Runtime.PowerState == vimtypes.VirtualMachinePowerStatePoweredOn
		}

		cdrom = createNewCdrom(specCdrom, bFileName, connected)
		deviceChanges = append(deviceChanges,
			&vimtypes.VirtualDeviceConfigSpec{
				Device:    cdrom,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			})
		newCdromBackingFileNameToSpec[bFileName] = specCdrom
		expectedBackingFileNameToCdrom[bFileName] = cdrom
	}

	// Remove obsolete CD-ROM devices and update the device list.
	newCurDevices, removeChanges := removeObsoleteCdroms(
		curDevices, expectedBackingFileNameToCdrom)
	deviceChanges = append(deviceChanges, removeChanges...)

	// Assign controllers with predefined placement information.
	if err := ensureAllCdromsHaveControllersAssigned(
		expectedBackingFileNameToCdrom, newCurDevices,
		newCdromBackingFileNameToSpec); err != nil {
		return nil, err
	}

	// Update connection state for existing CD-ROMs.
	curCdromChanges := updateCurCdromsConnectionState(
		curCdromBackingFileNameToSpec,
		expectedBackingFileNameToCdrom,
		vmCtx.MoVM.Runtime.PowerState,
	)
	deviceChanges = append(deviceChanges, curCdromChanges...)

	return deviceChanges, nil
}

// updateCdromDeviceChangesLegacy implements CD-ROM reconciliation with
// automatic controller assignment (VMSharedDisks disabled).
func updateCdromDeviceChangesLegacy(
	vmCtx pkgctx.VirtualMachineContext,
	restClient *rest.Client,
	k8sClient ctrlclient.Client,
	curDevices object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	hw := vmCtx.VM.Spec.Hardware
	if hw == nil {
		return nil, nil
	}

	var (
		deviceChanges                  = make([]vimtypes.BaseVirtualDeviceConfigSpec, 0)
		curCdromBackingFileNameToSpec  = make(map[string]vmopv1.VirtualMachineCdromSpec)
		expectedBackingFileNameToCdrom = make(map[string]vimtypes.BaseVirtualDevice, len(hw.Cdrom))
		libManager                     = library.NewManager(restClient)
	)

	for _, specCdrom := range hw.Cdrom {

		bFileName, cdrom, err := findCdromBySpec(
			vmCtx, k8sClient, specCdrom, curDevices, libManager)
		if err != nil {
			return nil, err
		}

		if cdrom != nil {
			// CD-ROM already exists, collect it to update its connection state
			// later with "Edit" operation.
			curCdromBackingFileNameToSpec[bFileName] = specCdrom
		} else {

			var connected bool
			if specCdrom.Connected != nil {
				// A device can only be connected if the VM is powered on. See
				// https://github.com/vmware/govmomi/blob/49947e5bbd7b83e91cf89dc2ba80daae72dddfff/simulator/virtual_machine.go#L1506-L1511
				connected = *specCdrom.Connected &&
					vmCtx.MoVM.Runtime.PowerState == vimtypes.VirtualMachinePowerStatePoweredOn
			}

			// CD-ROM does not exist, create a new one with desired backing and
			// connection, and update the device changes with "Add" operation.
			cdrom = createNewCdrom(specCdrom, bFileName, connected)

			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Device:    cdrom,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			})
		}

		expectedBackingFileNameToCdrom[bFileName] = cdrom
	}

	// Remove obsolete CD-ROM devices and update the device list.
	newCurDevices, removeChanges := removeObsoleteCdroms(
		curDevices, expectedBackingFileNameToCdrom)
	deviceChanges = append(deviceChanges, removeChanges...)

	// Auto-assign controllers, may add new controllers if needed.
	newControllers, err := ensureAllCdromsHaveControllers(
		expectedBackingFileNameToCdrom, newCurDevices)
	if err != nil {
		return nil, err
	}
	deviceChanges = append(deviceChanges, newControllers...)

	// Update connection state for existing CD-ROMs.
	curCdromChanges := updateCurCdromsConnectionState(
		curCdromBackingFileNameToSpec,
		expectedBackingFileNameToCdrom,
		vmCtx.MoVM.Runtime.PowerState,
	)
	deviceChanges = append(deviceChanges, curCdromChanges...)

	return deviceChanges, nil
}

// removeObsoleteCdroms removes CD-ROMs not in the expected list and returns
// the updated device list and remove operations.
func removeObsoleteCdroms(
	curDevices object.VirtualDeviceList,
	expectedBackingFileNameToCdrom map[string]vimtypes.BaseVirtualDevice,
) (object.VirtualDeviceList, []vimtypes.BaseVirtualDeviceConfigSpec) {

	newCurDevices := make(object.VirtualDeviceList, 0, len(curDevices))
	deviceChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0)

	for _, d := range curDevices {
		cdrom, ok := d.(*vimtypes.VirtualCdrom)
		if !ok {
			newCurDevices = append(newCurDevices, d)
			continue
		}

		if b, ok := cdrom.Backing.(*vimtypes.VirtualCdromIsoBackingInfo); ok &&
			expectedBackingFileNameToCdrom[b.FileName] != nil {
			newCurDevices = append(newCurDevices, cdrom)
			continue
		}

		deviceChanges = append(deviceChanges,
			&vimtypes.VirtualDeviceConfigSpec{
				Device:    cdrom,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
			})
	}

	return newCurDevices, deviceChanges
}

// UpdateConfigSpecCdromDeviceConnection updates the connection state of the
// VM's existing CD-ROM devices to match what specifies in VM.Spec.Cdrom list.
func UpdateConfigSpecCdromDeviceConnection(
	vmCtx pkgctx.VirtualMachineContext,
	restClient *rest.Client,
	k8sClient ctrlclient.Client,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	hw := vmCtx.VM.Spec.Hardware
	if hw == nil {
		return nil
	}

	var (
		cdromSpec                    = hw.Cdrom
		curDevices                   = object.VirtualDeviceList(config.Hardware.Device)
		backingFileNameToCdromSpec   = make(map[string]vmopv1.VirtualMachineCdromSpec, len(cdromSpec))
		backingFileNameToCdromDevice = make(map[string]vimtypes.BaseVirtualDevice, len(cdromSpec))
		libManager                   = library.NewManager(restClient)
	)

	for _, specCdrom := range cdromSpec {
		imageRef := specCdrom.Image
		// Sync the content library file if needed to connect the CD-ROM device.
		syncFile := ptr.Deref(specCdrom.Connected)
		bFileName, err := GetBackingFileNameByImageRef(vmCtx, k8sClient, imageRef, vmCtx.VM.Namespace, syncFile, libManager)
		if err != nil {
			return fmt.Errorf("error getting backing file name by image ref %s: %w", imageRef, err)
		}
		cdrom, err := getCdromByBackingFileName(bFileName, curDevices)
		if err != nil {
			return fmt.Errorf("error getting CD-ROM device by backing file name %s: %w", bFileName, err)
		}

		if cdrom == nil {
			// This could happen if the VM spec has a new CD-ROM device, or the
			// existing CD-ROM device's backing has been changed. The former
			// situation should be denied by the VM validating webhook.
			return fmt.Errorf("no CD-ROM is found for image ref %s", imageRef)
		}

		backingFileNameToCdromSpec[bFileName] = specCdrom
		backingFileNameToCdromDevice[bFileName] = cdrom
	}

	curCdromChanges := updateCurCdromsConnectionState(
		backingFileNameToCdromSpec,
		backingFileNameToCdromDevice,
		vmCtx.MoVM.Runtime.PowerState,
	)
	configSpec.DeviceChange = append(configSpec.DeviceChange, curCdromChanges...)

	return nil
}

// GetBackingFileNameByImageRef returns the ISO type content library file name
// based on the given VirtualMachineImageRef. It also syncs the content library
// if needed to ensure the file is available for CD-ROM connection.
func GetBackingFileNameByImageRef(
	ctx context.Context,
	client ctrlclient.Client,
	imageRef vmopv1.VirtualMachineImageRef,
	ns string,
	syncFile bool,
	libManager *library.Manager) (string, error) {

	var (
		libItemUUID string
		itemStatus  imgregv1a1.ContentLibraryItemStatus
		err         error
		logger      = pkglog.FromContextOrDefault(ctx)
	)

	switch imageRef.Kind {
	case vmiKind:
		libItemUUID, itemStatus, err = processImage(ctx, client, imageRef.Name, ns)
		if err != nil {
			return "", err
		}
	case cvmiKind:
		libItemUUID, itemStatus, err = processImage(ctx, client, imageRef.Name, "")
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported image kind: %q", imageRef.Kind)
	}

	if len(itemStatus.FileInfo) == 0 || itemStatus.FileInfo[0].StorageURI == "" {
		return "", fmt.Errorf("no storage URI found in the content library item status: %v", itemStatus)
	}

	// Subscribed content library item file may not always be stored in VC.
	// Sync the item to ensure the file is available for CD-ROM connection.
	if syncFile && (!itemStatus.Cached || itemStatus.SizeInBytes.IsZero()) {
		if libManager == nil {
			return "", fmt.Errorf("error syncing due to nil library manager")
		}
		logger.V(2).Info("Syncing content library item", "libItemUUID", libItemUUID)
		libItem, err := libManager.GetLibraryItem(ctx, libItemUUID)
		if err != nil {
			return "", fmt.Errorf("error getting library item %s to sync: %w", libItemUUID, err)
		}
		if err := libManager.SyncLibraryItem(ctx, libItem, true); err != nil {
			return "", fmt.Errorf("error syncing library item %s: %w", libItemUUID, err)
		}
	}

	return itemStatus.FileInfo[0].StorageURI, nil
}

// processImage validates if the image is ready and of type ISO, and returns the
// content library item UUID and status from its provider ref.
func processImage(
	ctx context.Context,
	client ctrlclient.Client,
	imageName string,
	namespace string) (string, imgregv1a1.ContentLibraryItemStatus, error) {

	var vmi vmopv1.VirtualMachineImage

	if namespace != "" {
		// Namespace scope image.
		if err := client.Get(ctx, ctrlclient.ObjectKey{Name: imageName, Namespace: namespace}, &vmi); err != nil {
			return "", imgregv1a1.ContentLibraryItemStatus{}, err
		}
	} else {
		// Cluster scope image.
		var cvmi vmopv1.ClusterVirtualMachineImage
		if err := client.Get(ctx, ctrlclient.ObjectKey{Name: imageName}, &cvmi); err != nil {
			return "", imgregv1a1.ContentLibraryItemStatus{}, err
		}
		vmi = vmopv1.VirtualMachineImage(cvmi)
	}

	// Verify image before retrieving content library item.
	if !conditions.IsTrue(&vmi, vmopv1.ReadyConditionType) {
		return "", imgregv1a1.ContentLibraryItemStatus{}, fmt.Errorf("image condition is not ready: %v", conditions.Get(&vmi, vmopv1.ReadyConditionType))
	}
	if vmi.Status.Type != string(imgregv1a1.ContentLibraryItemTypeIso) {
		return "", imgregv1a1.ContentLibraryItemStatus{}, fmt.Errorf("image type %q is not ISO", vmi.Status.Type)
	}
	if vmi.Spec.ProviderRef == nil || vmi.Spec.ProviderRef.Name == "" {
		return "", imgregv1a1.ContentLibraryItemStatus{}, errors.New("image provider ref is empty")
	}

	var (
		itemName = vmi.Spec.ProviderRef.Name
		clitem   imgregv1a1.ContentLibraryItem
	)

	if namespace != "" {
		// Namespace scope CL item.
		if err := client.Get(ctx, ctrlclient.ObjectKey{Name: itemName, Namespace: namespace}, &clitem); err != nil {
			return "", imgregv1a1.ContentLibraryItemStatus{}, err
		}
	} else {
		// Cluster scope CL item.
		var cclitem imgregv1a1.ClusterContentLibraryItem
		if err := client.Get(ctx, ctrlclient.ObjectKey{Name: itemName}, &cclitem); err != nil {
			return "", imgregv1a1.ContentLibraryItemStatus{}, err
		}
		clitem = imgregv1a1.ContentLibraryItem(cclitem)
	}

	return string(clitem.Spec.UUID), clitem.Status, nil
}

// getCdromByBackingFileName returns the CD-ROM device from the current devices
// by matching the given backing file name.
func getCdromByBackingFileName(
	fileName string,
	curDevices object.VirtualDeviceList) (vimtypes.BaseVirtualDevice, error) {

	backing := &vimtypes.VirtualCdromIsoBackingInfo{
		VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
			FileName: fileName,
		},
	}

	curCdroms := curDevices.SelectByBackingInfo(backing)
	switch len(curCdroms) {
	case 0:
		return nil, nil
	case 1:
		return curCdroms[0], nil
	default:
		return nil, fmt.Errorf("found multiple CD-ROMs with same backing file name: %s", fileName)
	}
}

// findCdromBySpec finds a CD-ROM device by resolving the image reference from
// the spec and looking it up in the current devices. It returns the backing
// file name and the CD-ROM device if found.
func findCdromBySpec(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	spec vmopv1.VirtualMachineCdromSpec,
	curDevices object.VirtualDeviceList,
	libManager *library.Manager) (string, vimtypes.BaseVirtualDevice, error) {

	// Sync the content library file if needed to connect the CD-ROM device.
	syncFile := ptr.Deref(spec.Connected)
	bFileName, err := GetBackingFileNameByImageRef(
		vmCtx, k8sClient, spec.Image, vmCtx.VM.Namespace, syncFile, libManager)
	if err != nil {
		return "", nil, fmt.Errorf(
			"error getting backing file name by image ref %s: %w", spec.Image, err)
	}

	cdrom, err := getCdromByBackingFileName(bFileName, curDevices)
	if err != nil {
		return "", nil, fmt.Errorf(
			"error getting CD-ROM device by backing file name %s: %w", bFileName, err)
	}

	return bFileName, cdrom, nil
}

// createNewCdrom creates a new CD-ROM device with the given backing file name
// and connection state as specified in the VirtualMachineCdromSpec.
func createNewCdrom(
	cdromSpec vmopv1.VirtualMachineCdromSpec,
	backingFileName string,
	connected bool) *vimtypes.VirtualCdrom {

	backing := &vimtypes.VirtualCdromIsoBackingInfo{
		VirtualDeviceFileBackingInfo: vimtypes.VirtualDeviceFileBackingInfo{
			FileName: backingFileName,
		},
	}

	return &vimtypes.VirtualCdrom{
		VirtualDevice: vimtypes.VirtualDevice{
			Backing: backing,
			Connectable: &vimtypes.VirtualDeviceConnectInfo{
				AllowGuestControl: ptr.Deref(cdromSpec.AllowGuestControl),
				StartConnected:    ptr.Deref(cdromSpec.Connected),
				Connected:         connected,
			},
		},
	}
}

// ensureAllCdromsHaveControllers ensures all CD-ROM device are assigned to an
// available controller on the VM. First connect to one of the VM's two IDE
// channels if one is free. If they are both in use, connect to the VM's SATA
// controller if one is present. Otherwise, add a new SATA (AHCI) controller to
// the VM and assign the CD-ROM to it.
// It returns a list of device changes if any new controllers are added.
func ensureAllCdromsHaveControllers(
	expectedCdromDevices map[string]vimtypes.BaseVirtualDevice,
	curDevices object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	var newControllerChanges []vimtypes.BaseVirtualDeviceConfigSpec

	for _, cdrom := range expectedCdromDevices {
		if cdrom.GetVirtualDevice().ControllerKey != 0 {
			// CD-ROM is already assigned to a controller.
			continue
		}

		if ide := curDevices.PickController((*vimtypes.VirtualIDEController)(nil)); ide != nil {
			// IDE controller is available.
			curDevices.AssignController(cdrom, ide)
		} else if sata := curDevices.PickController((*vimtypes.VirtualSATAController)(nil)); sata != nil {
			// SATA controller is available.
			curDevices.AssignController(cdrom, sata)
		} else {
			// No existing IDE or SATA controller is available, add a new one.
			sata, controllerChanges, updatedCurDevices, err := addNewSATAController(curDevices)
			if err != nil {
				return nil, fmt.Errorf("error adding a new SATA controller: %w", err)
			}
			curDevices.AssignController(cdrom, sata)
			newControllerChanges = append(newControllerChanges, controllerChanges...)
			curDevices = updatedCurDevices
		}

		// Update curDevices with assigned CD-ROM for correct slot (unit number)
		// allocation in the next CD-ROM assignment.
		curDevices = append(curDevices, cdrom)
	}

	return newControllerChanges, nil
}

// addNewSATAController adds a new SATA (AHCI) controller to the VM and returns:
// - the new SATA controller,
// - a list of device changes adding the new controller and PCI (if added)
// - updated current devices list with the new controller and PCI (if added).
func addNewSATAController(curDevices object.VirtualDeviceList) (
	vimtypes.BaseVirtualController,
	[]vimtypes.BaseVirtualDeviceConfigSpec,
	object.VirtualDeviceList,
	error) {

	var (
		pciController *vimtypes.VirtualPCIController
		deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	)

	// Get PCI controller key which is required to add a new SATA controller.
	if existingPCI := curDevices.PickController(
		(*vimtypes.VirtualPCIController)(nil)); existingPCI != nil {
		pciController = existingPCI.(*vimtypes.VirtualPCIController)
	} else {
		// PCI controller is not present, create a new one.
		pciController = &vimtypes.VirtualPCIController{
			VirtualController: vimtypes.VirtualController{
				VirtualDevice: vimtypes.VirtualDevice{
					Key: curDevices.NewKey(),
				},
			},
		}
		deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Device:    pciController,
			Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
		})
		curDevices = append(curDevices, pciController)
	}

	sata, err := curDevices.CreateSATAController()
	if err != nil {
		return nil, nil, curDevices, err
	}

	curDevices.AssignController(sata, pciController)
	deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
		Device:    sata,
		Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
	})
	curDevices = append(curDevices, sata)

	return sata.(vimtypes.BaseVirtualController).GetVirtualController(), deviceChanges, curDevices, nil
}

// updateCurCdromsConnectionState updates the connection state of the given
// CD-ROM devices to match the desired connection state in the given spec.
func updateCurCdromsConnectionState(
	backingFileNameToCdromSpec map[string]vmopv1.VirtualMachineCdromSpec,
	backingFileNameToCdrom map[string]vimtypes.BaseVirtualDevice,
	currentPowerState vimtypes.VirtualMachinePowerState) []vimtypes.BaseVirtualDeviceConfigSpec {

	if len(backingFileNameToCdromSpec) == 0 || len(backingFileNameToCdrom) == 0 {
		return nil
	}

	var (
		deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
		poweredOn     = currentPowerState == vimtypes.VirtualMachinePowerStatePoweredOn
	)

	for b, spec := range backingFileNameToCdromSpec {
		var (
			desiredConnected         bool
			desiredAllowGuestControl bool
			desiredStartConnected    bool
		)

		if spec.Connected != nil {
			// A device can only be connected if the VM is powered on. See
			// https://github.com/vmware/govmomi/blob/49947e5bbd7b83e91cf89dc2ba80daae72dddfff/simulator/virtual_machine.go#L1506-L1511
			desiredConnected = poweredOn && *spec.Connected
			desiredStartConnected = *spec.Connected
		}
		if spec.AllowGuestControl != nil {
			desiredAllowGuestControl = *spec.AllowGuestControl
		}

		if dev, ok := backingFileNameToCdrom[b]; ok {

			var (
				hasChange          bool
				cdrom              = dev.(*vimtypes.VirtualCdrom)
				connectable        = cdrom.Connectable
				isConnected        = connectable != nil && connectable.Connected
				allowsGuestControl = connectable != nil && connectable.AllowGuestControl
				startsConnected    = connectable != nil && connectable.StartConnected
			)

			if startsConnected != desiredStartConnected {
				if connectable == nil {
					connectable = &vimtypes.VirtualDeviceConnectInfo{}
				}
				hasChange = true
				connectable.StartConnected = desiredStartConnected
			}

			if isConnected != desiredConnected {
				if connectable == nil {
					connectable = &vimtypes.VirtualDeviceConnectInfo{}
				}
				hasChange = true
				connectable.Connected = desiredConnected
			}

			if allowsGuestControl != desiredAllowGuestControl {
				if connectable == nil {
					connectable = &vimtypes.VirtualDeviceConnectInfo{}
				}
				hasChange = true
				connectable.AllowGuestControl = desiredAllowGuestControl
			}

			cdrom.Connectable = connectable

			if hasChange {
				deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
					Device:    cdrom,
					Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
				})
			}
		}
	}

	return deviceChanges
}

// buildControllerMaps creates maps for fast controller lookup and unit number tracking.
// It returns two maps:
// - A map of ControllerID to BaseVirtualController for controller lookup by type and bus number.
// - A map of ControllerID to used unit numbers for conflict detection.
func buildControllerMaps(curDevices object.VirtualDeviceList) (
	map[pkgutil.ControllerID]vimtypes.BaseVirtualController,
	map[pkgutil.ControllerID]map[int32]struct{}) {

	ctrlMap := make(map[pkgutil.ControllerID]vimtypes.BaseVirtualController)
	ctrlUsedUnitNumMap := make(map[pkgutil.ControllerID]map[int32]struct{})
	ctrlKeyMap := buildControllerKeyToInfoMap(curDevices)

	// Build controller map and initialize unit number tracking.
	for _, device := range curDevices {
		switch controller := device.(type) {
		case *vimtypes.VirtualIDEController:
			key := pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      controller.BusNumber,
			}
			ctrlMap[key] = controller.GetVirtualController()
			ctrlUsedUnitNumMap[key] = make(map[int32]struct{})

		case vimtypes.BaseVirtualSATAController:
			sataCtrl := controller.GetVirtualSATAController()
			key := pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				BusNumber:      sataCtrl.BusNumber,
			}
			ctrlMap[key] = sataCtrl.GetVirtualController()
			ctrlUsedUnitNumMap[key] = make(map[int32]struct{})
		}
	}

	// Track used unit numbers.
	for _, device := range curDevices {
		ctrlKey := device.GetVirtualDevice().ControllerKey
		unitNum := device.GetVirtualDevice().UnitNumber

		if ctrlKey == 0 || unitNum == nil {
			continue
		}

		if ctrlID, ok := ctrlKeyMap[ctrlKey]; ok {
			ctrlUsedUnitNumMap[ctrlID][*unitNum] = struct{}{}
		}
	}

	return ctrlMap, ctrlUsedUnitNumMap
}

// ensureAllCdromsHaveControllersAssigned ensures all CD-ROM devices are
// assigned to controllers based on their predefined controller information when
// VMSharedDisks capability is enabled. Each CD-ROM spec must have controllerType,
// controllerBusNumber, and unitNumber defined. This is guaranteed by the schema
// upgrade to backfill the old CD-ROM specs and the CD-ROM mutating webhook.
func ensureAllCdromsHaveControllersAssigned(
	expectedCdromDevices map[string]vimtypes.BaseVirtualDevice,
	curDevices object.VirtualDeviceList,
	newCdromBackingFileNameToSpec map[string]vmopv1.VirtualMachineCdromSpec) error {

	ctrlMap, ctrlUsedUnitNumMap := buildControllerMaps(curDevices)

	for bFileName, cdrom := range expectedCdromDevices {
		if cdrom.GetVirtualDevice().ControllerKey != 0 {
			// CD-ROM is already assigned to a controller.
			continue
		}

		spec := newCdromBackingFileNameToSpec[bFileName]

		if spec.ControllerType == "" || spec.ControllerBusNumber == nil || spec.UnitNumber == nil {
			return fmt.Errorf("CD-ROM spec does not have all the controller information: %+v", spec)
		}

		ctrlKey := pkgutil.ControllerID{
			ControllerType: spec.ControllerType,
			BusNumber:      *spec.ControllerBusNumber,
		}
		ctrl, exists := ctrlMap[ctrlKey]
		if !exists {
			return fmt.Errorf("controller not found: type=%s, busNumber=%d",
				spec.ControllerType, *spec.ControllerBusNumber)
		}
		if _, used := ctrlUsedUnitNumMap[ctrlKey][*spec.UnitNumber]; used {
			return fmt.Errorf("found a CD-ROM spec with a used unitNumber %d", *spec.UnitNumber)
		}

		curDevices.AssignController(cdrom, ctrl)
		cdrom.GetVirtualDevice().UnitNumber = spec.UnitNumber
		curDevices = append(curDevices, cdrom)
		ctrlUsedUnitNumMap[ctrlKey][*spec.UnitNumber] = struct{}{}
	}

	return nil
}

// buildControllerKeyToInfoMap builds a map from controller key to
// controller information (type and bus number).
func buildControllerKeyToInfoMap(
	curDevices object.VirtualDeviceList) map[int32]pkgutil.ControllerID {

	ctrlKeyToInfo := make(map[int32]pkgutil.ControllerID)

	for _, d := range curDevices {
		switch ctrl := d.(type) {
		case *vimtypes.VirtualIDEController:
			ctrlKeyToInfo[ctrl.Key] = pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeIDE,
				BusNumber:      ctrl.BusNumber,
			}
		case vimtypes.BaseVirtualSATAController:
			sataCtrl := ctrl.GetVirtualSATAController()
			ctrlKeyToInfo[sataCtrl.Key] = pkgutil.ControllerID{
				ControllerType: vmopv1.VirtualControllerTypeSATA,
				BusNumber:      sataCtrl.BusNumber,
			}
		}
	}

	return ctrlKeyToInfo
}

// hasControllerPlacementChanged checks if the controller placement
// (type, bus number, or unit number) has changed for a CD-ROM device.
func hasControllerPlacementChanged(
	cdrom vimtypes.BaseVirtualDevice,
	spec vmopv1.VirtualMachineCdromSpec,
	ctrlKeyToInfo map[int32]pkgutil.ControllerID) (bool, error) {

	device := cdrom.GetVirtualDevice()

	// Lookup current controller info.
	currentCtrlInfo, exists := ctrlKeyToInfo[device.ControllerKey]
	if !exists {
		return false, fmt.Errorf(
			"controller with key %d not found for CD-ROM device",
			device.ControllerKey)
	}

	// Check if any placement attribute changed.
	if currentCtrlInfo.ControllerType != spec.ControllerType ||
		currentCtrlInfo.BusNumber != *spec.ControllerBusNumber ||
		!ptr.Equal(device.UnitNumber, spec.UnitNumber) {
		return true, nil
	}

	return false, nil
}
