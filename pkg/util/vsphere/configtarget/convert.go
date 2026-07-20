// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package configtarget

import (
	"fmt"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
)

// bytesPerKB is used to convert vSphere's KB-denominated fields into the
// byte-denominated resource.Quantity fields used by ConfigTargetDevices.
const bytesPerKB = 1024

// populateConfigTargetDevices maps the non-SR-IOV device categories from the
// vSphere QueryConfigTarget result onto devices. SR-IOV is intentionally left
// unmapped here: ct.Sriov is untouched, and any *vimtypes.VirtualMachineSriovInfo
// entry found inside ct.PciPassthrough's union is skipped. Per-host SR-IOV
// enrichment (host attribution, DVX capabilities) is not supported yet;
// it is tracked separately (vmop-3926, T121).
func populateConfigTargetDevices(devices *vimv1.ConfigTargetDevices, ct *vimtypes.ConfigTarget) {
	devices.CDROM = convertCdroms(ct.CdRom)
	devices.Floppy = convertNames(ct.Floppy, func(v vimtypes.VirtualMachineFloppyInfo) string { return v.Name })
	devices.Serial = convertNames(ct.Serial, func(v vimtypes.VirtualMachineSerialInfo) string { return v.Name })
	devices.Parallel = convertNames(ct.Parallel, func(v vimtypes.VirtualMachineParallelInfo) string { return v.Name })
	devices.Sound = convertNames(ct.Sound, func(v vimtypes.VirtualMachineSoundInfo) string { return v.Name })
	devices.USB = convertUSBs(ct.Usb)

	devices.PCIPassthrough = convertPciPassthroughUnion(ct.PciPassthrough)
	devices.DynamicPassthroughDevices = convertDynamicPassthroughs(ct.DynamicPassthrough)

	devices.VGPUDevice = convertVgpuDevices(ct.VgpuDeviceInfo)
	devices.VGPUProfile = convertVgpuProfiles(ct.VgpuProfileInfo)
	devices.SharedGPUPassthroughTypes = convertSharedGPUPassthroughTypes(ct.SharedGpuPassthroughTypes)
	devices.SGXTargetInfo = convertSgxTargetInfo(ct.SgxTargetInfo)
	devices.PrecisionClockInfo = convertPrecisionClockInfos(ct.PrecisionClockInfo)
	devices.VendorDeviceGroupInfo = convertVendorDeviceGroupInfos(ct.VendorDeviceGroupInfo)
	devices.DVXClassInfo = convertDvxClassInfos(ct.DvxClassInfo)
	devices.IDEDisks = convertIdeDisks(ct.IdeDisk)
	devices.SCSIDisks = convertScsiDisks(ct.ScsiDisk)
	devices.SCSIPassthrough = convertScsiPassthroughs(ct.ScsiPassthrough)
	devices.VFlashModule = convertVFlashModules(ct.VFlashModule)
}

// convertNames converts a slice of any govmomi type embedding
// VirtualMachineTargetInfo into a slice of vimv1.VirtualMachineTargetInfo,
// using the caller-supplied accessor to read the promoted Name field.
func convertNames[T any](items []T, name func(T) string) []vimv1.VirtualMachineTargetInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineTargetInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineTargetInfo{Name: name(it)}
	}

	return out
}

func convertCdroms(items []vimtypes.VirtualMachineCdromInfo) []vimv1.VirtualMachineCdromInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineCdromInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineCdromInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			Description:              it.Description,
		}
	}

	return out
}

func convertUSBs(items []vimtypes.VirtualMachineUsbInfo) []vimv1.VirtualMachineUSBInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineUSBInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineUSBInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			Description:              it.Description,
			Vendor:                   it.Vendor,
			Product:                  it.Product,
			PhysicalPath:             it.PhysicalPath,
			Family:                   it.Family,
			Speed:                    it.Speed,
		}
	}

	return out
}

func convertHostPCIDevice(d vimtypes.HostPciDevice) vimv1.HostPCIDevice {
	return vimv1.HostPCIDevice{
		VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: d.DeviceName},
		Id:                       d.Id,
		ClassId:                  int32(d.ClassId),
		Bus:                      int32(d.Bus),
		Slot:                     int32(d.Slot),
		PhysicalSlot:             d.PhysicalSlot,
		SlotDescription:          d.SlotDescription,
		Function:                 int32(d.Function),
		VendorId:                 int32(d.VendorId),
		SubVendorId:              int32(d.SubVendorId),
		VendorName:               d.VendorName,
		DeviceId:                 int32(d.DeviceId),
		SubDeviceId:              int32(d.SubDeviceId),
		ParentBridge:             d.ParentBridge,
		DeviceName:               d.DeviceName,
		DeviceClassName:          d.DeviceClassName,
	}
}

func convertPCIPassthroughInfo(it vimtypes.VirtualMachinePciPassthroughInfo) vimv1.VirtualMachinePCIPassthroughInfo {
	return vimv1.VirtualMachinePCIPassthroughInfo{
		VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
		ID:                       fmt.Sprintf("%s:%s", it.SystemId, it.PciDevice.Id),
		PciDevice:                convertHostPCIDevice(it.PciDevice),
		SystemID:                 it.SystemId,
	}
}

// convertPciPassthroughUnion type-switches the BaseVirtualMachinePciPassthroughInfo
// union returned by QueryConfigTarget. VirtualMachineDynamicPassthroughInfo
// does not implement this interface (it does not embed
// VirtualMachinePciPassthroughInfo), so the only concrete types that can
// appear here are *VirtualMachinePciPassthroughInfo and
// *VirtualMachineSriovInfo (which embeds VirtualMachinePciPassthroughInfo).
// The latter is dropped -- SR-IOV is not supported yet; it is tracked
// separately (vmop-3926, T121) via a future per-host path.
func convertPciPassthroughUnion(
	items []vimtypes.BaseVirtualMachinePciPassthroughInfo) []vimv1.VirtualMachinePCIPassthroughInfo {
	var pciPassthrough []vimv1.VirtualMachinePCIPassthroughInfo

	for _, item := range items {
		switch v := item.(type) {
		case *vimtypes.VirtualMachineSriovInfo:
			// SR-IOV is not supported yet (vmop-3926, T121); skip.
		case *vimtypes.VirtualMachinePciPassthroughInfo:
			pciPassthrough = append(pciPassthrough, convertPCIPassthroughInfo(*v))
		}
	}

	return pciPassthrough
}

func convertDynamicPassthroughInfo(it vimtypes.VirtualMachineDynamicPassthroughInfo) vimv1.VirtualMachineDynamicPassthroughInfo {
	return vimv1.VirtualMachineDynamicPassthroughInfo{
		VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
		VendorName:               it.VendorName,
		DeviceName:               it.DeviceName,
		CustomLabel:              it.CustomLabel,
		VendorID:                 it.VendorId,
		DeviceID:                 it.DeviceId,
	}
}

func convertDynamicPassthroughs(items []vimtypes.VirtualMachineDynamicPassthroughInfo) []vimv1.VirtualMachineDynamicPassthroughInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineDynamicPassthroughInfo, len(items))
	for i, it := range items {
		out[i] = convertDynamicPassthroughInfo(it)
	}

	return out
}

func convertVgpuDevices(items []vimtypes.VirtualMachineVgpuDeviceInfo) []vimv1.VirtualMachineVgpuDeviceInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineVgpuDeviceInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineVgpuDeviceInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			DeviceName:               it.DeviceName,
			DeviceVendorID:           it.DeviceVendorId,
			MaxFbSizeInGib:           it.MaxFbSizeInGib,
			TimeSlicedCapable:        it.TimeSlicedCapable,
			MigCapable:               it.MigCapable,
			ComputeProfileCapable:    it.ComputeProfileCapable,
			QuadroProfileCapable:     it.QuadroProfileCapable,
		}
	}

	return out
}

func convertVMotionStunTimes(items []vimtypes.VirtualMachineVMotionStunTimeInfo) []vimv1.VirtualMachineVMotionStunTimeInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineVMotionStunTimeInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineVMotionStunTimeInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			MigrationBW:              it.MigrationBW,
			StunTime:                 it.StunTime,
		}
	}

	return out
}

func convertVgpuProfiles(items []vimtypes.VirtualMachineVgpuProfileInfo) []vimv1.VirtualMachineVgpuProfileInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineVgpuProfileInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineVgpuProfileInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			ProfileName:              it.ProfileName,
			DeviceVendorID:           it.DeviceVendorId,
			FbSizeInGib:              it.FbSizeInGib,
			ProfileSharing:           it.ProfileSharing,
			ProfileClass:             it.ProfileClass,
			StunTimeEstimates:        convertVMotionStunTimes(it.StunTimeEstimates),
		}
	}

	return out
}

func convertSharedGPUPassthroughTypes(items []vimtypes.VirtualMachinePciSharedGpuPassthroughInfo) []vimv1.VirtualMachinePciSharedGpuPassthroughInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachinePciSharedGpuPassthroughInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachinePciSharedGpuPassthroughInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			VGPU:                     it.Vgpu,
		}
	}

	return out
}

func convertSgxTargetInfo(it *vimtypes.VirtualMachineSgxTargetInfo) *vimv1.VirtualMachineSgxTargetInfo {
	if it == nil {
		return nil
	}

	return &vimv1.VirtualMachineSgxTargetInfo{
		VirtualMachineTargetInfo:    vimv1.VirtualMachineTargetInfo{Name: it.Name},
		MaxEpcSize:                  it.MaxEpcSize,
		FlcModes:                    it.FlcModes,
		LePubKeyHashes:              it.LePubKeyHashes,
		RequireAttestationSupported: it.RequireAttestationSupported != nil && *it.RequireAttestationSupported,
	}
}

func convertPrecisionClockInfos(items []vimtypes.VirtualMachinePrecisionClockInfo) []vimv1.VirtualMachinePrecisionClockInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachinePrecisionClockInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachinePrecisionClockInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			SystemClockProtocol:      vimv1.HostDateTimeInfoProtocol(it.SystemClockProtocol),
		}
	}

	return out
}

// convertVendorDeviceGroupInfos maps DeviceGroupName/DeviceGroupDescription
// and each ComponentDeviceInfo's Type/VendorName/DeviceName/IsConfigurable.
// ComponentDeviceInfo.Device (a VirtualDevice template) is intentionally left
// unset: nothing in vm-operator reads it today, so converting govmomi's
// polymorphic BaseVirtualDevice/backing-info union into vimv1.VirtualDevice
// is deferred until a real consumer needs it.
func convertVendorDeviceGroupInfos(items []vimtypes.VirtualMachineVendorDeviceGroupInfo) []vimv1.VirtualMachineVendorDeviceGroupInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineVendorDeviceGroupInfo, len(items))
	for i, it := range items {
		var components []vimv1.VirtualMachineVendorDeviceGroupInfoComponentDeviceInfo
		if len(it.ComponentDeviceInfo) > 0 {
			components = make([]vimv1.VirtualMachineVendorDeviceGroupInfoComponentDeviceInfo, len(it.ComponentDeviceInfo))
			for j, c := range it.ComponentDeviceInfo {
				components[j] = vimv1.VirtualMachineVendorDeviceGroupInfoComponentDeviceInfo{
					Type:           vimv1.VirtualMachineVendorDeviceGroupInfoComponentDeviceInfoComponentType(c.Type),
					VendorName:     c.VendorName,
					DeviceName:     c.DeviceName,
					IsConfigurable: c.IsConfigurable,
				}
			}
		}

		out[i] = vimv1.VirtualMachineVendorDeviceGroupInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			DeviceGroupName:          it.DeviceGroupName,
			DeviceGroupDescription:   it.DeviceGroupDescription,
			ComponentDeviceInfo:      components,
		}
	}

	return out
}

// convertElementDescription maps the common Key/Label/Summary fields shared
// by every vim.ElementDescription subtype. Subtype-specific data
// (ExtendedElementDescription, EVCMode, etc.) is not populated; DVX class
// descriptions and vFlash choice lists use nothing but a plain
// ElementDescription in practice.
func convertElementDescription(ed vimtypes.BaseElementDescription) vimv1.ElementDescription {
	if ed == nil {
		return vimv1.ElementDescription{}
	}

	d := ed.GetElementDescription()

	return vimv1.ElementDescription{
		Key:     d.Key,
		Label:   d.Label,
		Summary: d.Summary,
		Type:    vimv1.ElementDescriptionTypeBase,
	}
}

// convertDvxClassInfos maps DeviceClass/VendorName/SriovNic. ConfigParams
// (a list of vim.OptionDef, each wrapping a polymorphic OptionType) is
// intentionally left unmapped -- see convertVendorDeviceGroupInfos for the
// same rationale: nothing reads it today.
func convertDvxClassInfos(items []vimtypes.VirtualMachineDvxClassInfo) []vimv1.VirtualMachineDvxClassInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineDvxClassInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineDvxClassInfo{
			DeviceClass: convertElementDescription(it.DeviceClass),
			VendorName:  it.VendorName,
			SriovNic:    it.SriovNic,
		}
	}

	return out
}

func convertIdeDiskPartitions(items []vimtypes.VirtualMachineIdeDiskDevicePartitionInfo) []vimv1.VirtualMachineIdeDiskDevicePartitionInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineIdeDiskDevicePartitionInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineIdeDiskDevicePartitionInfo{
			ID:       it.Id,
			Capacity: quantityFromKB(int64(it.Capacity)),
		}
	}

	return out
}

func convertIdeDisks(items []vimtypes.VirtualMachineIdeDiskDeviceInfo) []vimv1.VirtualMachineIdeDiskDeviceInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineIdeDiskDeviceInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineIdeDiskDeviceInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			Capacity:                 quantityFromKB(it.Capacity),
			PartitionTable:           convertIdeDiskPartitions(it.PartitionTable),
		}
	}

	return out
}

func convertVsanDiskInfo(it *vimtypes.VsanHostVsanDiskInfo) *vimv1.VsanHostVsanDiskInfo {
	if it == nil {
		return nil
	}

	return &vimv1.VsanHostVsanDiskInfo{
		VsanUUID:      it.VsanUuid,
		FormatVersion: it.FormatVersion,
	}
}

func convertHostScsiDisk(it *vimtypes.HostScsiDisk) *vimv1.HostScsiDisk {
	if it == nil {
		return nil
	}

	return &vimv1.HostScsiDisk{
		DeviceName:    it.DeviceName,
		DeviceType:    it.DeviceType,
		Key:           it.Key,
		UUID:          it.Uuid,
		CanonicalName: it.CanonicalName,
		DisplayName:   it.DisplayName,
		LunType:       it.LunType,
		Vendor:        it.Vendor,
		Model:         it.Model,
		Capacity: vimv1.HostDiskDimensionsLba{
			Block:     it.Capacity.Block,
			BlockSize: it.Capacity.BlockSize,
		},
		DevicePath:            it.DevicePath,
		Ssd:                   it.Ssd != nil && *it.Ssd,
		LocalDisk:             it.LocalDisk != nil && *it.LocalDisk,
		ScsiDiskType:          vimv1.SCSIDiskType(it.ScsiDiskType),
		EmulatedDIXDIFEnabled: it.EmulatedDIXDIFEnabled != nil && *it.EmulatedDIXDIFEnabled,
		PhysicalLocation:      it.PhysicalLocation,
		UsedByMemoryTiering:   it.UsedByMemoryTiering != nil && *it.UsedByMemoryTiering,
		VsanDiskInfo:          convertVsanDiskInfo(it.VsanDiskInfo),
	}
}

func convertScsiDisks(items []vimtypes.VirtualMachineScsiDiskDeviceInfo) []vimv1.VirtualMachineScsiDiskDeviceInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineScsiDiskDeviceInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineScsiDiskDeviceInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			Capacity:                 quantityFromKB(it.Capacity),
			Disk:                     convertHostScsiDisk(it.Disk),
			TransportHint:            it.TransportHint,
			LunNumber:                it.LunNumber,
		}
	}

	return out
}

func convertScsiPassthroughs(items []vimtypes.VirtualMachineScsiPassthroughInfo) []vimv1.VirtualMachineScsiPassthroughInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineScsiPassthroughInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineScsiPassthroughInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			PhysicalUnitNumber:       it.PhysicalUnitNumber,
			SCSIClass:                it.ScsiClass,
			Vendor:                   it.Vendor,
		}
	}

	return out
}

func convertChoiceOption(co vimtypes.ChoiceOption) vimv1.ChoiceOption {
	var choices []vimv1.ElementDescription
	if len(co.ChoiceInfo) > 0 {
		choices = make([]vimv1.ElementDescription, len(co.ChoiceInfo))
		for i, ci := range co.ChoiceInfo {
			choices[i] = convertElementDescription(ci)
		}
	}

	return vimv1.ChoiceOption{
		BaseOptionType: vimv1.BaseOptionType{ValueIsReadonly: co.ValueIsReadonly},
		Choices:        choices,
		DefaultIndex:   co.DefaultIndex,
	}
}

func convertLongOptionToResourceQuantityOption(lo vimtypes.LongOption, unitBytes int64) vimv1.ResourceQuantityOption {
	return vimv1.ResourceQuantityOption{
		BaseOptionType: vimv1.BaseOptionType{ValueIsReadonly: lo.ValueIsReadonly},
		ResourceQuantityRange: vimv1.ResourceQuantityRange{
			Min: *resource.NewQuantity(lo.Min*unitBytes, resource.BinarySI),
			Max: *resource.NewQuantity(lo.Max*unitBytes, resource.BinarySI),
		},
		Default: *resource.NewQuantity(lo.DefaultValue*unitBytes, resource.BinarySI),
	}
}

func convertVFlashModuleConfigOption(
	o vimtypes.HostVFlashManagerVFlashCacheConfigInfoVFlashModuleConfigOption,
) vimv1.HostVFlashManagerVFlashCacheConfigInfoVFlashModuleConfigOption {
	return vimv1.HostVFlashManagerVFlashCacheConfigInfoVFlashModuleConfigOption{
		VFlashModule:              o.VFlashModule,
		VFlashModuleVersion:       o.VFlashModuleVersion,
		MinSupportedModuleVersion: o.MinSupportedModuleVersion,
		CacheMode:                 convertChoiceOption(o.CacheMode),
		CacheConsistencyType:      convertChoiceOption(o.CacheConsistencyType),
		BlockSizeOption:           convertLongOptionToResourceQuantityOption(o.BlockSizeInKBOption, bytesPerKB),
		ReservationOption:         convertLongOptionToResourceQuantityOption(o.ReservationInMBOption, bytesPerMB),
		MaxDiskSize:               *resource.NewQuantity(o.MaxDiskSizeInKB*bytesPerKB, resource.BinarySI),
	}
}

func convertVFlashModules(items []vimtypes.VirtualMachineVFlashModuleInfo) []vimv1.VirtualMachineVFlashModuleInfo {
	if len(items) == 0 {
		return nil
	}

	out := make([]vimv1.VirtualMachineVFlashModuleInfo, len(items))
	for i, it := range items {
		out[i] = vimv1.VirtualMachineVFlashModuleInfo{
			VirtualMachineTargetInfo: vimv1.VirtualMachineTargetInfo{Name: it.Name},
			VFlashModule:             convertVFlashModuleConfigOption(it.VFlashModule),
		}
	}

	return out
}

// quantityFromKB converts a vSphere legacy KB-denominated capacity field
// into a resource.Quantity, matching the byte-conversion convention already
// used by PopulateStatus for MB-denominated fields. Returns nil for a
// zero/absent capacity, consistent with the omitempty optional fields it
// feeds.
func quantityFromKB(capacityKB int64) *resource.Quantity {
	if capacityKB <= 0 {
		return nil
	}

	return resource.NewQuantity(capacityKB*bytesPerKB, resource.BinarySI)
}
