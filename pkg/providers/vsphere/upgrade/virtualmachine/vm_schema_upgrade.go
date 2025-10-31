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

	// Build a map of backing file name to CD-ROM device.
	var cdromDevices []*vimtypes.VirtualCdrom

	for i := range moVM.Config.Hardware.Device {
		switch d := moVM.Config.Hardware.Device[i].(type) {

		case *vimtypes.VirtualDisk:
			reconcileVirtualDisk(ctx, vm, d)

		case *vimtypes.VirtualCdrom:
			cdromDevices = append(cdromDevices, d)
		}
	}

	reconcileVirtualCDROMs(ctx, k8sClient, vm, ctrlKeyToCtrlStatus, cdromDevices)
}

func reconcileVirtualDisk(
	ctx context.Context,
	_ *vmopv1.VirtualMachine,
	_ *vimtypes.VirtualDisk) {

	logger := pkglog.FromContextOrDefault(ctx)
	logger.V(4).Info("Reconciling schema upgrade for VM disk")

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

	bFileNameToCdromInfo := buildBackingFileNameToCdromInfoMap(
		cdromDevices)

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
// CD-ROM spec.
func reconcileCdromSpec(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	spec *vmopv1.VirtualMachineCdromSpec,
	bFileNameToCdromInfo map[string]pkgutil.VirtualCdromInfo,
	ctrlKeyToCtrlStatus map[int32]vmopv1.VirtualControllerStatus) {

	logger := pkglog.FromContextOrDefault(ctx)

	bFileName, err := virtualmachine.GetBackingFileNameByImageRef(
		ctx, k8sClient, spec.Image, vm.Namespace, false, nil)
	if err != nil {
		logger.Error(err,
			"Error getting backing file name", "spec", spec)
		return
	}

	info, ok := bFileNameToCdromInfo[bFileName]
	if !ok {
		return
	}

	ctrl, ok := ctrlKeyToCtrlStatus[info.ControllerKey]
	if !ok {
		return
	}

	if !needsCdromBackfill(spec) {
		return
	}

	if hasCdromPlacementMismatch(spec, &info, &ctrl) {
		logger.V(4).Info(
			"Skipping CD-ROM spec update due to mismatch"+
				"between vm cd-rom spec and vm cd-rom device state",
			"spec", spec,
			"unitNumber", info.UnitNumber,
			"controllerBusNumber", ctrl.BusNumber,
			"controllerType", ctrl.Type)
		return
	}

	// Perform atomic backfill of all fields
	spec.UnitNumber = info.UnitNumber
	spec.ControllerType = ctrl.Type
	spec.ControllerBusNumber = &ctrl.BusNumber

	logger.V(4).Info("Backfilled CD-ROM placement info",
		"unitNumber", *spec.UnitNumber,
		"controllerType", spec.ControllerType,
		"controllerBusNumber", *spec.ControllerBusNumber)
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
