// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vm

import (
	"fmt"

	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vapi/library"
	"github.com/vmware/govmomi/vapi/rest"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	imgregv1a1 "github.com/vmware-tanzu/image-registry-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
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

	var (
		deviceChanges                  = make([]vimtypes.BaseVirtualDeviceConfigSpec, 0)
		curCdromBackingFileNameToSpec  = make(map[string]vmopv1.VirtualMachineCdromSpec)
		expectedBackingFileNameToCdrom = make(map[string]vimtypes.BaseVirtualDevice, len(vmCtx.VM.Spec.Cdrom))
		libManager                     = library.NewManager(restClient)
	)

	for _, specCdrom := range vmCtx.VM.Spec.Cdrom {
		imageRef := specCdrom.Image
		// Sync the content library file if needed to connect the CD-ROM device.
		syncFile := ptr.Deref(specCdrom.Connected)
		bFileName, err := getBackingFileNameByImageRef(vmCtx, libManager, k8sClient, syncFile, imageRef)
		if err != nil {
			return nil, fmt.Errorf("error getting backing file name by image ref %s: %w", imageRef, err)
		}
		cdrom, err := getCdromByBackingFileName(bFileName, curDevices)
		if err != nil {
			return nil, fmt.Errorf("error getting CD-ROM device by backing file name %s: %w", bFileName, err)
		}

		if cdrom != nil {
			// CD-ROM already exists, collect it to update its connection state
			// later with "Edit" operation.
			curCdromBackingFileNameToSpec[bFileName] = specCdrom
		} else {
			// CD-ROM does not exist, create a new one with desired backing and
			// connection, and update the device changes with "Add" operation.
			cdrom = createNewCdrom(specCdrom, bFileName)
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Device:    cdrom,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			})
		}

		expectedBackingFileNameToCdrom[bFileName] = cdrom
	}

	// Ensure all CD-ROM devices are assigned to controllers with proper keys.
	// This may result new controllers to be added if none is available.
	newControllers := ensureAllCdromsHaveControllers(expectedBackingFileNameToCdrom, curDevices)
	deviceChanges = append(deviceChanges, newControllers...)

	// Check and update existing CD-ROM devices' connection state if needed.
	// Add them to the device changes with "Edit" operation.
	curCdromChanges := updateCurCdromsConnectionState(
		curCdromBackingFileNameToSpec,
		expectedBackingFileNameToCdrom,
	)
	deviceChanges = append(deviceChanges, curCdromChanges...)

	// Remove any existing CD-ROM devices that are not in the expected list.
	// Add them to the device changes with "Remove" operation.
	curCdroms := util.SelectDevicesByType[*vimtypes.VirtualCdrom](curDevices)
	for _, cdrom := range curCdroms {
		if b, ok := cdrom.Backing.(*vimtypes.VirtualCdromIsoBackingInfo); ok {
			if _, ok := expectedBackingFileNameToCdrom[b.FileName]; ok {
				continue
			}
		}

		deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Device:    cdrom,
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	return deviceChanges, nil
}

// UpdateConfigSpecCdromDeviceConnection updates the connection state of the
// VM's existing CD-ROM devices to match what specifies in VM.Spec.Cdrom list.
func UpdateConfigSpecCdromDeviceConnection(
	vmCtx pkgctx.VirtualMachineContext,
	restClient *rest.Client,
	k8sClient ctrlclient.Client,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	var (
		cdromSpec                    = vmCtx.VM.Spec.Cdrom
		curDevices                   = object.VirtualDeviceList(config.Hardware.Device)
		backingFileNameToCdromSpec   = make(map[string]vmopv1.VirtualMachineCdromSpec, len(cdromSpec))
		backingFileNameToCdromDevice = make(map[string]vimtypes.BaseVirtualDevice, len(cdromSpec))
		libManager                   = library.NewManager(restClient)
	)

	for _, specCdrom := range cdromSpec {
		imageRef := specCdrom.Image
		// Sync the content library file if needed to connect the CD-ROM device.
		syncFile := ptr.Deref(specCdrom.Connected)
		bFileName, err := getBackingFileNameByImageRef(vmCtx, libManager, k8sClient, syncFile, imageRef)
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
	)
	configSpec.DeviceChange = append(configSpec.DeviceChange, curCdromChanges...)

	return nil
}

// getBackingFileNameByImageRef syncs and returns the ISO type content library
// file name based on the given VirtualMachineImageRef.
// If a namespace scope VirtualMachineImage is provided, it checks the
// ContentLibraryItem status, otherwise, a ClusterVirtualMachineImage status,
// and returns the backing file name from the StorageURI field.
func getBackingFileNameByImageRef(
	vmCtx pkgctx.VirtualMachineContext,
	libManager *library.Manager,
	client ctrlclient.Client,
	syncFile bool,
	imageRef vmopv1.VirtualMachineImageRef) (string, error) {

	var (
		libItemUUID string
		itemStatus  imgregv1a1.ContentLibraryItemStatus
	)

	switch imageRef.Kind {
	case vmiKind:
		ns := vmCtx.VM.Namespace
		var vmi vmopv1.VirtualMachineImage
		if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: imageRef.Name, Namespace: ns}, &vmi); err != nil {
			return "", err
		}
		if vmi.Spec.ProviderRef == nil {
			return "", fmt.Errorf("provider ref is nil for VirtualMachineImage: %q", vmi.Name)
		}
		var clitem imgregv1a1.ContentLibraryItem
		if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: vmi.Spec.ProviderRef.Name, Namespace: ns}, &clitem); err != nil {
			return "", err
		}
		libItemUUID = string(clitem.Spec.UUID)
		itemStatus = clitem.Status
	case cvmiKind:
		var cvmi vmopv1.ClusterVirtualMachineImage
		if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: imageRef.Name}, &cvmi); err != nil {
			return "", err
		}
		if cvmi.Spec.ProviderRef == nil {
			return "", fmt.Errorf("provider ref is nil for ClusterVirtualMachineImage: %q", cvmi.Name)
		}
		var cclitem imgregv1a1.ClusterContentLibraryItem
		if err := client.Get(vmCtx, ctrlclient.ObjectKey{Name: cvmi.Spec.ProviderRef.Name}, &cclitem); err != nil {
			return "", err
		}
		libItemUUID = string(cclitem.Spec.UUID)
		itemStatus = cclitem.Status

	default:
		return "", fmt.Errorf("unsupported image kind: %q", imageRef.Kind)
	}

	if itemStatus.Type != imgregv1a1.ContentLibraryItemTypeIso {
		return "", fmt.Errorf("expected ISO type image, got %s", itemStatus.Type)
	}
	if len(itemStatus.FileInfo) == 0 || itemStatus.FileInfo[0].StorageURI == "" {
		return "", fmt.Errorf("no storage URI found in the content library item status: %v", itemStatus)
	}

	// Subscribed content library item file may not always be stored in VC.
	// Sync the item to ensure the file is available for CD-ROM connection.
	if syncFile && (!itemStatus.Cached || itemStatus.SizeInBytes.Size() == 0) {
		vmCtx.Logger.Info("Syncing content library item", "libItemUUID", libItemUUID)
		libItem, err := libManager.GetLibraryItem(vmCtx, libItemUUID)
		if err != nil {
			return "", fmt.Errorf("error getting library item %s to sync: %w", libItemUUID, err)
		}
		if err := libManager.SyncLibraryItem(vmCtx, libItem, true); err != nil {
			return "", fmt.Errorf("error syncing library item %s: %w", libItemUUID, err)
		}
	}

	return itemStatus.FileInfo[0].StorageURI, nil
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

// createNewCdrom creates a new CD-ROM device with the given backing file name
// and connection state as specified in the VirtualMachineCdromSpec.
func createNewCdrom(
	cdromSpec vmopv1.VirtualMachineCdromSpec,
	backingFileName string) *vimtypes.VirtualCdrom {
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
				Connected:         ptr.Deref(cdromSpec.Connected),
			},
		},
	}
}

// ensureAllCdromsHaveControllers ensures all CD-ROM device are assigned to an
// available controller on the VM. First connect to one of the VM's two IDE
// channels if one is free. If they are both in use, connect to the VM's SATA
// controller if one is present. Otherwise, add a new AHCI (SATA) controller to
// the VM and assign the CD-ROM to it.
// It returns a list of device changes if a new controller is added.
func ensureAllCdromsHaveControllers(
	expectedCdromDevices map[string]vimtypes.BaseVirtualDevice,
	curDevices object.VirtualDeviceList) []vimtypes.BaseVirtualDeviceConfigSpec {

	var newControllerChanges []vimtypes.BaseVirtualDeviceConfigSpec

	for _, cdrom := range expectedCdromDevices {
		if cdrom.GetVirtualDevice().ControllerKey != 0 {
			// CD-ROM is already assigned to a controller.
			continue
		}

		if ide := curDevices.PickController((*vimtypes.VirtualIDEController)(nil)); ide != nil {
			curDevices.AssignController(cdrom, ide)
		} else if sata := curDevices.PickController((*vimtypes.VirtualSATAController)(nil)); sata != nil {
			curDevices.AssignController(cdrom, sata)
		} else {
			ahci, controllerChanges, updatedCurDevices := addNewAHCIController(curDevices)
			// Update curDevices to include new controllers so that next CD-ROM
			// can be assigned to them without adding new controllers.
			curDevices = updatedCurDevices
			curDevices.AssignController(cdrom, ahci)
			newControllerChanges = append(newControllerChanges, controllerChanges...)
		}

		// Update curDevices so that next CD-ROM device can be assigned to a
		// controller in correct slot (unit number).
		curDevices = append(curDevices, cdrom)
	}

	return newControllerChanges
}

// addNewAHCIController adds a new AHCI (SATA) controller to the VM and returns
// the AHCI controller and the other device changes required to add it to VM.
func addNewAHCIController(curDevices object.VirtualDeviceList) (
	*vimtypes.VirtualAHCIController,
	[]vimtypes.BaseVirtualDeviceConfigSpec,
	object.VirtualDeviceList) {

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

	ahciController := &vimtypes.VirtualAHCIController{
		VirtualSATAController: vimtypes.VirtualSATAController{
			VirtualController: vimtypes.VirtualController{
				VirtualDevice: vimtypes.VirtualDevice{
					ControllerKey: pciController.Key,
					Key:           curDevices.NewKey(),
				},
			},
		},
	}
	deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
		Device:    ahciController,
		Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
	})
	curDevices = append(curDevices, ahciController)

	return ahciController, deviceChanges, curDevices
}

// updateCurCdromsConnectionState updates the connection state of the given
// CD-ROM devices to match the desired connection state in the given spec.
func updateCurCdromsConnectionState(
	backingFileNameToCdromSpec map[string]vmopv1.VirtualMachineCdromSpec,
	backingFileNameToCdrom map[string]vimtypes.BaseVirtualDevice) []vimtypes.BaseVirtualDeviceConfigSpec {

	if len(backingFileNameToCdromSpec) == 0 || len(backingFileNameToCdrom) == 0 {
		return nil
	}

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

	for b, spec := range backingFileNameToCdromSpec {
		if cdrom, ok := backingFileNameToCdrom[b]; ok {
			if c := cdrom.GetVirtualDevice().Connectable; c != nil &&
				(c.Connected != ptr.Deref(spec.Connected) || c.AllowGuestControl != ptr.Deref(spec.AllowGuestControl)) {
				c.StartConnected = ptr.Deref(spec.Connected)
				c.Connected = ptr.Deref(spec.Connected)
				c.AllowGuestControl = ptr.Deref(spec.AllowGuestControl)

				deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
					Device:    cdrom,
					Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
				})
			}
		}
	}

	return deviceChanges
}
