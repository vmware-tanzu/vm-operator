// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"context"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	cnsv1alpha1 "github.com/vmware-tanzu/vm-operator/external/vsphere-csi-driver/api/v1alpha1"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var ErrUpgradeSchema = pkgerr.NoRequeueNoErr("upgraded vm schema")

// ReconcileSchemaUpgrade ensures the VM's spec is upgraded to match the current
// expectations for the data that should be present on a VirtualMachine object.
// This may include back-filling data from the underlying vSphere VM into the
// API object's spec.
//
// Please note, each time VM Operator is upgraded, patch/update operations
// against VirtualMachine objects by unprivileged users will be denied until
// ReconcileSchemaUpgrade is executed. This ensures the objects are not changing
// while the object is being back-filled.
func ReconcileSchemaUpgrade(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) error {

	if ctx == nil {
		panic("context is nil")
	}
	if k8sClient == nil {
		panic("k8sClient is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if moVM.Config == nil {
		panic("moVM.config is nil")
	}

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM")

	var (
		curBuildVersion  = pkgcfg.FromContext(ctx).BuildVersion
		curSchemaVersion = vmopv1.GroupVersion.Version

		vmBuildVersion  = vm.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey]
		vmSchemaVersion = vm.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey]
	)

	if vmBuildVersion == curBuildVersion &&
		vmSchemaVersion == curSchemaVersion {

		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM" +
			" that is already upgraded")
		return nil
	}

	reconcileBIOSUUID(ctx, vm, moVM)
	reconcileInstanceUUID(ctx, vm, moVM)
	reconcileCloudInitInstanceUUID(ctx, vm, moVM)
	reconcileControllers(ctx, vm, moVM)
	reconcileDevices(ctx, vm, moVM, k8sClient)

	// Indicate the VM has been upgraded.
	if vm.Annotations == nil {
		vm.Annotations = map[string]string{}
	}
	vm.Annotations[pkgconst.UpgradedToBuildVersionAnnotationKey] = curBuildVersion
	vm.Annotations[pkgconst.UpgradedToSchemaVersionAnnotationKey] = curSchemaVersion

	logger.V(4).Info("Upgraded VM schema version",
		"buildVersion", curBuildVersion,
		"schemaVersion", curSchemaVersion)

	return ErrUpgradeSchema
}

func reconcileBIOSUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM BIOS UUID")

	if vm.Spec.BiosUUID != "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"instance BIOS that is already upgraded")
		return
	}

	if moVM.Config.Uuid == "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"BIOS UUID with empty config.uuid")
		return
	}

	vm.Spec.BiosUUID = moVM.Config.Uuid
	logger.V(4).Info("Reconciled schema upgrade for VM BIOS UUID")
}

func reconcileInstanceUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM instance UUID")

	if vm.Spec.InstanceUUID != "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"instance UUID that is already upgraded")
		return
	}

	if moVM.Config.InstanceUuid == "" {
		logger.V(4).Info("Skipping reconciliation of schema upgrade for VM " +
			"instance UUID with empty config.instanceUuid")
		return
	}

	vm.Spec.InstanceUUID = moVM.Config.InstanceUuid
	logger.V(4).Info("Reconciled schema upgrade for VM instance UUID")
}

func reconcileCloudInitInstanceUUID(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	_ mo.VirtualMachine) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info(
		"Reconciling schema upgrade for VM cloud-init instance UUID")

	if vm.Spec.Bootstrap == nil {
		logger.V(4).Info(
			"Skipping reconciliation of schema upgrade for VM cloud-init " +
				"instance UUID with nil spec.bootstrap")
		return
	}

	if vm.Spec.Bootstrap.CloudInit == nil {
		logger.V(4).Info(
			"Skipping reconciliation of schema upgrade for VM cloud-init " +
				"instance UUID with nil spec.bootstrap.cloudInit")
		return
	}

	if vm.Spec.Bootstrap.CloudInit.InstanceID != "" {
		logger.V(4).Info(
			"Skipping reconciliation of schema upgrade for VM cloud-init" +
				"instance UUID that is already upgraded")
		return
	}

	_ = vmlifecycle.BootStrapCloudInitInstanceID(
		vm,
		vm.Spec.Bootstrap.CloudInit)

	logger.V(4).Info(
		"Reconciled schema upgrade for VM cloud-init instance UUID")
}

func reconcileControllers(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM controllers")

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		logger.V(4).Info("Skipping controllers reconciliation due to" +
			"disabled VMSharedDisks capability")
		return
	}

	for i := range moVM.Config.Hardware.Device {
		switch d := moVM.Config.Hardware.Device[i].(type) {

		// IDE
		case *vimtypes.VirtualIDEController:
			reconcileIDEController(ctx, vm, d)

		// NVME
		case *vimtypes.VirtualNVMEController:
			reconcileNVMEController(ctx, vm, d)

		// SATA
		case *vimtypes.VirtualAHCIController:
			reconcileSATAController(ctx, vm, d)

		// SCSI
		case vimtypes.BaseVirtualSCSIController:
			reconcileSCSIController(ctx, vm, d)
		}
	}
}

func initSpecHardware(vm *vmopv1.VirtualMachine) {
	if vm.Spec.Hardware == nil {
		vm.Spec.Hardware = &vmopv1.VirtualMachineHardwareSpec{}
	}
}

func getPCISlot(i vimtypes.BaseVirtualDeviceBusSlotInfo) *int32 {
	if pi, ok := i.(*vimtypes.VirtualDevicePciBusSlotInfo); ok {
		return ptr.To(pi.PciSlotNumber)
	}
	return nil
}

func reconcileIDEController(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	dev *vimtypes.VirtualIDEController) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM IDE controller")

	if vm.Spec.Hardware != nil {
		for _, c := range vm.Spec.Hardware.IDEControllers {
			if c.BusNumber == dev.BusNumber {
				logger.V(4).Info(
					"Skipping reconciliation of schema upgrade for VM " +
						"IDE controller that already exists")
				return
			}
		}
	}

	initSpecHardware(vm)

	newController := vmopv1.IDEControllerSpec{
		BusNumber: dev.BusNumber,
	}
	vm.Spec.Hardware.IDEControllers = append(
		vm.Spec.Hardware.IDEControllers,
		newController,
	)

	logger.V(4).Info("Added new IDE controller",
		"busNumber", newController.BusNumber)
}

func reconcileNVMEController(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	dev *vimtypes.VirtualNVMEController) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM NVME controller")

	if vm.Spec.Hardware != nil {
		for _, c := range vm.Spec.Hardware.NVMEControllers {
			if c.BusNumber == dev.BusNumber {
				logger.V(4).Info(
					"Skipping reconciliation of schema upgrade for VM " +
						"NVME controller that already exists")
				return
			}
		}
	}

	initSpecHardware(vm)

	newController := vmopv1.NVMEControllerSpec{
		BusNumber: dev.BusNumber,
	}
	switch dev.SharedBus {
	case string(vimtypes.VirtualNVMEControllerSharingNoSharing):
		newController.SharingMode = vmopv1.VirtualControllerSharingModeNone
	case string(vimtypes.VirtualNVMEControllerSharingPhysicalSharing):
		newController.SharingMode = vmopv1.VirtualControllerSharingModePhysical
	}
	if pi, ok := dev.SlotInfo.(*vimtypes.VirtualDevicePciBusSlotInfo); ok {
		newController.PCISlotNumber = ptr.To(pi.PciSlotNumber)
	}
	newController.PCISlotNumber = getPCISlot(dev.SlotInfo)

	vm.Spec.Hardware.NVMEControllers = append(
		vm.Spec.Hardware.NVMEControllers,
		newController,
	)

	logger.V(4).Info("Added new NVME controller",
		"busNumber", newController.BusNumber)

}

func reconcileSATAController(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	dev *vimtypes.VirtualAHCIController) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM SATA controller")

	if vm.Spec.Hardware != nil {
		for _, c := range vm.Spec.Hardware.SATAControllers {
			if c.BusNumber == dev.BusNumber {
				logger.V(4).Info(
					"Skipping reconciliation of schema upgrade for VM " +
						"SATA controller that already exists")
				return
			}
		}
	}

	initSpecHardware(vm)

	newController := vmopv1.SATAControllerSpec{
		BusNumber: dev.BusNumber,
	}
	if pi, ok := dev.SlotInfo.(*vimtypes.VirtualDevicePciBusSlotInfo); ok {
		newController.PCISlotNumber = ptr.To(pi.PciSlotNumber)
	}
	newController.PCISlotNumber = getPCISlot(dev.SlotInfo)

	vm.Spec.Hardware.SATAControllers = append(
		vm.Spec.Hardware.SATAControllers,
		newController,
	)

	logger.V(4).Info("Added new SATA controller",
		"busNumber", newController.BusNumber)

}

func reconcileSCSIController(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	dev vimtypes.BaseVirtualSCSIController) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM SCSI controller")

	baseController := dev.GetVirtualSCSIController()

	if vm.Spec.Hardware != nil {
		for _, c := range vm.Spec.Hardware.SCSIControllers {
			if c.BusNumber == baseController.BusNumber {
				logger.V(4).Info(
					"Skipping reconciliation of schema upgrade for VM " +
						"SCSI controller that already exists")
				return
			}
		}
	}

	initSpecHardware(vm)

	newController := vmopv1.SCSIControllerSpec{
		BusNumber:     baseController.BusNumber,
		PCISlotNumber: getPCISlot(baseController.SlotInfo),
	}

	switch baseController.SharedBus {
	case vimtypes.VirtualSCSISharingNoSharing:
		newController.SharingMode = vmopv1.VirtualControllerSharingModeNone
	case vimtypes.VirtualSCSISharingPhysicalSharing:
		newController.SharingMode = vmopv1.VirtualControllerSharingModePhysical
	case vimtypes.VirtualSCSISharingVirtualSharing:
		newController.SharingMode = vmopv1.VirtualControllerSharingModeVirtual
	}

	switch dev.(type) {
	case *vimtypes.ParaVirtualSCSIController:
		newController.Type = vmopv1.SCSIControllerTypeParaVirtualSCSI
	case *vimtypes.VirtualBusLogicController:
		newController.Type = vmopv1.SCSIControllerTypeBusLogic
	case *vimtypes.VirtualLsiLogicController:
		newController.Type = vmopv1.SCSIControllerTypeLsiLogic
	case *vimtypes.VirtualLsiLogicSASController:
		newController.Type = vmopv1.SCSIControllerTypeLsiLogicSAS
	}

	vm.Spec.Hardware.SCSIControllers = append(
		vm.Spec.Hardware.SCSIControllers,
		newController,
	)

	logger.V(4).Info("Added new SCSI controller",
		"busNumber", newController.BusNumber,
		"type", newController.Type)
}

func reconcileDevices(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	k8sClient ctrlclient.Client) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM devices")

	if !pkgcfg.FromContext(ctx).Features.VMSharedDisks {
		logger.V(4).Info("Skipping devices reconciliation due to" +
			"disabled VMSharedDisks capability")
		return
	}

	// Build a map of controller key to controller info from VM status.
	// Note: vm.status is always reconciled before calling this function,
	// and DeviceKey is a required field.
	ctrlKeyToCtrlStatus := make(map[int32]vmopv1.VirtualControllerStatus)
	if vm.Status.Hardware != nil {
		for _, ctrl := range vm.Status.Hardware.Controllers {
			ctrlKeyToCtrlStatus[ctrl.DeviceKey] = ctrl
		}
	}

	var (
		cdromDevices []*vimtypes.VirtualCdrom
		diskDevices  []*vimtypes.VirtualDisk
	)

	for i := range moVM.Config.Hardware.Device {
		switch d := moVM.Config.Hardware.Device[i].(type) {

		case *vimtypes.VirtualDisk:
			diskDevices = append(diskDevices, d)

		case *vimtypes.VirtualCdrom:
			cdromDevices = append(cdromDevices, d)
		}
	}

	reconcileVirtualCDROMs(ctx, k8sClient, vm, ctrlKeyToCtrlStatus, cdromDevices)
	reconcileVirtualDisks(ctx, k8sClient, vm, ctrlKeyToCtrlStatus, diskDevices)
}

// reconcileVirtualDisks reconciles the VM's disk devices during
// schema upgrade. It maps disk devices from the vSphere VM
// configuration to the VirtualMachine spec volumes, updating the
// unit number, controller type, and controller bus number for each
// PVC volume based on the disk UUID and controller information.
// Once VMSharedDisks is enabled, a new VM's PVC is never needed to be
// backfilled because it is expected that the PVCs to already have all the
// placement info. The backfilling is only needed for existing VMs.
func reconcileVirtualDisks(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	ctrlKeyToCtrlStatus map[int32]vmopv1.VirtualControllerStatus,
	diskDevices []*vimtypes.VirtualDisk) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM disks")

	diskUUIDToDiskInfo := buildDiskUUIDToDiskInfoMap(diskDevices)
	if len(diskUUIDToDiskInfo) == 0 {
		logger.V(4).Info("Skipping disks reconciliation due to no virtual disks attached")
		return
	}

	pvcNameToDiskUUID := buildPVCNameToDiskUUIDMap(ctx, k8sClient, vm)
	if len(pvcNameToDiskUUID) == 0 {
		logger.V(4).Info("Skipping disks reconciliation due to no node attachments found")
		return
	}

	for i := range vm.Spec.Volumes {
		vol := &vm.Spec.Volumes[i]

		if vol.PersistentVolumeClaim == nil ||
			vol.PersistentVolumeClaim.UnmanagedVolumeClaim != nil {
			continue
		}

		reconcilePVCVolumePlacement(
			ctx,
			vol,
			pvcNameToDiskUUID,
			diskUUIDToDiskInfo,
			ctrlKeyToCtrlStatus)
	}
}

// buildDiskUUIDToDiskInfoMap builds a map of disk UUID to disk
// device info by extracting from vSphere VM hardware.
func buildDiskUUIDToDiskInfoMap(
	diskDevices []*vimtypes.VirtualDisk) map[string]pkgutil.VirtualDiskInfo {

	diskUUIDToDiskInfo := make(map[string]pkgutil.VirtualDiskInfo)
	for _, disk := range diskDevices {
		vdi := pkgutil.GetVirtualDiskInfo(disk)
		if vdi.UUID != "" {
			diskUUIDToDiskInfo[vdi.UUID] = vdi
		}
	}
	return diskUUIDToDiskInfo
}

// buildPVCNameToDiskUUIDMap builds a map of PVC claim name to
// disk UUID by querying CnsNodeVmAttachment objects.
func buildPVCNameToDiskUUIDMap(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine) map[string]string {

	attachments, err := pkgutil.GetCnsNodeVMAttachmentsForVM(ctx, k8sClient, vm)
	if err != nil {
		pkglog.FromContextOrDefault(ctx).V(4).Error(
			err,
			"Error getting CnsNodeVmAttachments")
		return nil
	}

	pvcNameToDiskUUID := make(map[string]string)
	for _, attm := range attachments {
		diskUUID := attm.Status.AttachmentMetadata[cnsv1alpha1.AttributeFirstClassDiskUUID]
		pvcName := attm.Spec.VolumeName
		// Ignore Attached status since we are checking directly against
		// the attached virtual devices.
		if diskUUID != "" && pvcName != "" {
			pvcNameToDiskUUID[pvcName] = diskUUID
		}
	}
	return pvcNameToDiskUUID
}

// reconcilePVCVolumePlacement reconciles the placement info for
// a single PVC volume. It performs an atomic backfill of all placement
// info fields if needed.
func reconcilePVCVolumePlacement(
	ctx context.Context,
	vol *vmopv1.VirtualMachineVolume,
	pvcNameToDiskUUID map[string]string,
	diskUUIDToDiskInfo map[string]pkgutil.VirtualDiskInfo,
	ctrlKeyToCtrlStatus map[int32]vmopv1.VirtualControllerStatus) {

	pvc := vol.PersistentVolumeClaim
	logger := pkglog.FromContextOrDefault(ctx).WithValues(
		"name", vol.Name,
		"pvc", pvc.ClaimName)

	if !needsPlacementBackfill(vol) {
		logger.V(4).Info("Skipping volume due to no placement fields to backfill")
		return
	}

	diskUUID, ok := pvcNameToDiskUUID[pvc.ClaimName]
	if !ok {
		logger.V(4).Info("Skipping volume due to no matching disk UUID for PVC",
			"diskUUID", diskUUID)
		return
	}
	info, ok := diskUUIDToDiskInfo[diskUUID]
	if !ok {
		logger.V(4).Info("Skipping volume due file backing not found",
			"diskUUID", diskUUID)
		return
	}
	ctrl, ok := ctrlKeyToCtrlStatus[info.ControllerKey]
	if !ok {
		logger.V(4).Info("Skipping volume due to no controller key found",
			"controllerKey", info.ControllerKey)
		return
	}

	if hasPlacementMismatch(vol, &info, &ctrl) {
		logger.V(4).Info(
			"Skipping volume due to spec/state mismatch",
			"unitNumber", info.UnitNumber,
			"controllerBusNumber", ctrl.BusNumber,
			"controllerType", ctrl.Type)
		return
	}

	vol.UnitNumber = info.UnitNumber
	vol.ControllerType = ctrl.Type
	vol.ControllerBusNumber = &ctrl.BusNumber
}

// needsPlacementBackfill checks if any placement fields are missing.
func needsPlacementBackfill(
	vol *vmopv1.VirtualMachineVolume) bool {

	return vol.UnitNumber == nil ||
		vol.ControllerBusNumber == nil ||
		vol.ControllerType == ""
}

// hasPlacementMismatch checks if any existing placement fields
// conflict with the actual VM hardware configuration.
func hasPlacementMismatch(
	vol *vmopv1.VirtualMachineVolume,
	info *pkgutil.VirtualDiskInfo,
	ctrl *vmopv1.VirtualControllerStatus) bool {

	return (vol.UnitNumber != nil &&
		!ptr.Equal(vol.UnitNumber, info.UnitNumber)) ||
		(vol.ControllerBusNumber != nil &&
			!ptr.Equal(vol.ControllerBusNumber, &ctrl.BusNumber)) ||
		(vol.ControllerType != "" && ctrl.Type != vol.ControllerType)
}

// reconcileVirtualCDROMs reconciles the VM's CD-ROM devices during
// schema upgrade. It maps CD-ROM devices from the vSphere VM
// configuration to the VirtualMachine spec, updating the unit
// number, controller type, and controller bus number for each
// CD-ROM device based on the backing file name and controller
// information.
func reconcileVirtualCDROMs(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	ctrlKeyToCtrlStatus map[int32]vmopv1.VirtualControllerStatus,
	cdromDevices []*vimtypes.VirtualCdrom) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM CD-ROM")

	if vm.Spec.Hardware == nil {
		return
	}

	bFileNameToCdromInfo := buildBackingFileNameToCdromInfoMap(cdromDevices)
	if len(bFileNameToCdromInfo) == 0 {
		logger.V(4).Info("Skipping CD-ROM reconciliation due to no virtual " +
			"CD-ROM devices with backing files attached")
		return
	}

	for i := range vm.Spec.Hardware.Cdrom {
		spec := &vm.Spec.Hardware.Cdrom[i]
		reconcileCdromSpec(
			ctx,
			k8sClient,
			vm,
			spec,
			bFileNameToCdromInfo,
			ctrlKeyToCtrlStatus)
	}
}

// buildBackingFileNameToCdromInfoMap builds a map of backing file
// name to CD-ROM device info. Devices pointing to the same ISO
// image are blocked by the VM validating webhook and CD-ROM
// reconciler.
func buildBackingFileNameToCdromInfoMap(
	cdromDevices []*vimtypes.VirtualCdrom) map[string]pkgutil.VirtualCdromInfo {

	bFileNameToCdromInfo := make(map[string]pkgutil.VirtualCdromInfo)
	for _, cdrom := range cdromDevices {
		cdi := pkgutil.GetVirtualCdromInfo(cdrom)
		if cdi.FileName != "" {
			bFileNameToCdromInfo[cdi.FileName] = cdi
		}
	}
	return bFileNameToCdromInfo
}

// reconcileCdromSpec reconciles the placement info for a single
// CD-ROM spec. It performs an atomic backfill of all placement
// info fields if needed.
func reconcileCdromSpec(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	spec *vmopv1.VirtualMachineCdromSpec,
	bFileNameToCdromInfo map[string]pkgutil.VirtualCdromInfo,
	ctrlKeyToCtrlStatus map[int32]vmopv1.VirtualControllerStatus) {

	logger := pkglog.FromContextOrDefault(ctx).WithValues("name", spec.Name)

	if !needsCdromBackfill(spec) {
		logger.V(4).Info("Skipping CD-ROM due to no placement fields to backfill")
		return
	}

	bFileName, err := virtualmachine.GetBackingFileNameByImageRef(
		ctx, k8sClient, spec.Image, vm.Namespace, false, nil)
	if err != nil {
		logger.Error(err, "Error getting CD-ROM backing file name")
		return
	}

	info, ok := bFileNameToCdromInfo[bFileName]
	if !ok {
		logger.V(4).Info("Skipping CD-ROM due to no matching backing file name",
			"backingFileName", bFileName)
		return
	}

	ctrl, ok := ctrlKeyToCtrlStatus[info.ControllerKey]
	if !ok {
		logger.V(4).Info("Skipping CD-ROM due to no matching controller key",
			"controllerKey", info.ControllerKey)
		return
	}

	if hasCdromPlacementMismatch(spec, &info, &ctrl) {
		logger.V(4).Info(
			"Skipping CD-ROM due to spec/state mismatch",
			"unitNumber", info.UnitNumber,
			"controllerBusNumber", ctrl.BusNumber,
			"controllerType", ctrl.Type)
		return
	}

	spec.UnitNumber = info.UnitNumber
	spec.ControllerType = ctrl.Type
	spec.ControllerBusNumber = &ctrl.BusNumber
}

// needsCdromBackfill checks if any CD-ROM placement fields are
// missing.
func needsCdromBackfill(
	spec *vmopv1.VirtualMachineCdromSpec) bool {

	return spec.UnitNumber == nil ||
		spec.ControllerBusNumber == nil ||
		spec.ControllerType == ""
}

// hasCdromPlacementMismatch checks if any existing CD-ROM
// placement fields conflict with the actual VM hardware
// configuration.
func hasCdromPlacementMismatch(
	spec *vmopv1.VirtualMachineCdromSpec,
	info *pkgutil.VirtualCdromInfo,
	ctrl *vmopv1.VirtualControllerStatus) bool {

	return (spec.UnitNumber != nil &&
		!ptr.Equal(spec.UnitNumber, info.UnitNumber)) ||
		(spec.ControllerBusNumber != nil &&
			!ptr.Equal(spec.ControllerBusNumber, &ctrl.BusNumber)) ||
		(spec.ControllerType != "" && ctrl.Type != spec.ControllerType)
}
