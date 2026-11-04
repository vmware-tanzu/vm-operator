// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// Package configoptions maps vSphere config and guest options to discovery APIs.
package configoptions

import (
	"fmt"
	"slices"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"
	"k8s.io/apimachinery/pkg/api/resource"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// PopulateStatus copies the hardware limits and capabilities needed by classes.
func PopulateStatus(obj *vimv1.VirtualMachineConfigOptions, in *vimtypes.VirtualMachineConfigOption) {
	hw := convertVirtualHardwareOption(in.HardwareOptions)
	obj.Status.HardwareOptions = &hw
	capabilities := convertVirtualMachineConfigCapability(in.Capabilities)
	obj.Status.Capabilities = &capabilities
	storage := convertStorageClassOption(in.Datastore)
	obj.Status.StorageClassOptions = &storage
	obj.Status.PropertyRelations = nil
	for _, item := range in.PropertyRelations {
		obj.Status.PropertyRelations = append(obj.Status.PropertyRelations, convertVirtualMachinePropertyRelation(item))
	}
}

// GuestStatus maps all guest-specific hardware limits and feature support.
func GuestStatus(hardwareVersion string, in vimtypes.GuestOsDescriptor) vimv1.VirtualMachineGuestOptionsHardwareVersionStatus {
	out := convertVirtualMachineGuestOptionsHardwareVersionStatus(in)
	out.HardwareVersion = hardwareVersion
	return out
}
func deviceType(in string) string {
	switch strings.ToLower(in) {
	case "ide":
		return "VirtualIDEController"
	case "buslogic":
		return "VirtualBusLogicController"
	case "lsilogic":
		return "VirtualLsiLogicController"
	case "lsilogicsas":
		return "VirtualLsiLogicSASController"
	case "pvscsi":
		return "ParaVirtualSCSIController"
	case "sata", "ahci":
		return "VirtualAHCIController"
	case "nvme", "virtualnvmecontroller":
		return "VirtualNVMeController"
	case "uhci", "ehci":
		return "VirtualUSBController"
	case "xhci":
		return "VirtualUSBXHCIController"
	case "e1000":
		return "VirtualE1000"
	case "e1000e":
		return "VirtualE1000e"
	case "pcnet32":
		return "VirtualPCNet32"
	case "sriov":
		return "VirtualSriovEthernetCard"
	case "vmxnet":
		return "VirtualVmxnet"
	case "vmxnet2":
		return "VirtualVmxnet2"
	case "vmxnet3":
		return "VirtualVmxnet3"
	case "vmxnet3vrdma":
		return "VirtualVmxnet3Vrdma"
	}
	return in
}
func quantityOption(in vimtypes.LongOption, scale int64) vimv1.ResourceQuantityOption {
	return vimv1.ResourceQuantityOption{
		BaseOptionType:        vimv1.BaseOptionType{ValueIsReadonly: in.ValueIsReadonly},
		ResourceQuantityRange: vimv1.ResourceQuantityRange{Min: *resource.NewQuantity(in.Min*scale, resource.BinarySI), Max: *resource.NewQuantity(in.Max*scale, resource.BinarySI)},
		Default:               *resource.NewQuantity(in.DefaultValue*scale, resource.BinarySI),
	}
}
func convertVirtualHardwareOption(in vimtypes.VirtualHardwareOption) vimv1.VirtualHardwareOption {
	var out vimv1.VirtualHardwareOption
	out.HardwareVersion = fmt.Sprintf("vmx-%d", in.HwVersion)
	out.DeviceListReadonly = in.DeviceListReadonly
	out.NumCPU = slices.Clone(in.NumCPU)
	out.NumCoresPerSocket = ptr.To(convertIntOption(in.NumCoresPerSocket))
	out.AutoCoresPerSocket = mapPointer(in.AutoCoresPerSocket, convertBoolOption)
	out.NumCpuReadonly = in.NumCpuReadonly
	out.NumCPUSimultaneousThreads = mapPointer(in.NumCpuSimultaneousThreads, convertIntOption)
	out.Memory = func(v vimtypes.LongOption) *vimv1.ResourceQuantityOption {
		out := quantityOption(v, 1024*1024)
		return &out
	}(in.MemoryMB)
	out.NumNumaNodes = mapPointer(in.NumNumaNodes, convertIntOption)
	out.NumPCIControllers = ptr.To(convertIntOption(in.NumPCIControllers))
	out.NumIDEControllers = ptr.To(convertIntOption(in.NumIDEControllers))
	out.NumUSBControllers = ptr.To(convertIntOption(in.NumUSBControllers))
	out.NumUSBXHCIControllers = ptr.To(convertIntOption(in.NumUSBXHCIControllers))
	out.NumSIOControllers = ptr.To(convertIntOption(in.NumSIOControllers))
	out.NumPS2Controllers = ptr.To(convertIntOption(in.NumPS2Controllers))
	out.NumSupportedWwnPorts = mapPointer(in.NumSupportedWwnPorts, convertIntOption)
	out.NumSupportedWwnNodes = mapPointer(in.NumSupportedWwnNodes, convertIntOption)
	out.NumTPMDevices = mapPointer(in.NumTPMDevices, convertIntOption)
	out.NumNVDIMMControllers = mapPointer(in.NumNVDIMMControllers, convertIntOption)
	out.NumPrecisionClockDevices = mapPointer(in.NumPrecisionClockDevices, convertIntOption)
	out.NumWDTDevices = mapPointer(in.NumWDTDevices, convertIntOption)
	out.NumDeviceGroups = mapPointer(in.NumDeviceGroups, convertIntOption)
	out.EpcMemory = func(v *vimtypes.LongOption) *vimv1.ResourceQuantityOption {
		if v == nil {
			return nil
		}
		out := quantityOption(*v, 1024*1024)
		return &out
	}(in.EpcMemoryMB)
	out.AcpiHostBridgesFirmware = func(in []string) []vimv1.Firmware {
		if in == nil {
			return nil
		}
		out := make([]vimv1.Firmware, len(in))
		for i, item := range in {
			out[i] = vimv1.Firmware(strings.ToUpper(item))
		}
		return out
	}(in.AcpiHostBridgesFirmware)
	out.DeviceGroupTypes = slices.Clone(in.DeviceGroupTypes)
	out.LicensingLimit = slices.Clone(in.LicensingLimit)
	return out
}

func convertIntOption(in vimtypes.IntOption) vimv1.IntOption {
	var out vimv1.IntOption
	out.BaseOptionType = convertBaseOptionType(in.OptionType)
	out.IntRange = vimv1.IntRange{Min: in.Min, Max: in.Max}
	out.Default = in.DefaultValue
	return out
}

func convertBaseOptionType(in vimtypes.OptionType) vimv1.BaseOptionType {
	var out vimv1.BaseOptionType
	out.ValueIsReadonly = clonePointer(in.ValueIsReadonly)
	return out
}

func convertBoolOption(in vimtypes.BoolOption) vimv1.BoolOption {
	var out vimv1.BoolOption
	out.BaseOptionType = convertBaseOptionType(in.OptionType)
	out.Default = in.DefaultValue
	out.Supported = in.Supported
	return out
}

func convertVirtualMachineConfigCapability(in vimtypes.VirtualMachineCapability) vimv1.VirtualMachineConfigCapability {
	var out vimv1.VirtualMachineConfigCapability
	out.SnapshotOperationsSupported = in.SnapshotOperationsSupported
	out.MultipleSnapshotsSupported = in.MultipleSnapshotsSupported
	out.SnapshotConfigSupported = in.SnapshotConfigSupported
	out.PoweredOffSnapshotsSupported = in.PoweredOffSnapshotsSupported
	out.MemorySnapshotsSupported = in.MemorySnapshotsSupported
	out.RevertToSnapshotSupported = in.RevertToSnapshotSupported
	out.QuiescedSnapshotsSupported = in.QuiescedSnapshotsSupported
	out.LockSnapshotsSupported = in.LockSnapshotsSupported
	out.DiskOnlySnapshotOnSuspendedVMSupported = clonePointer(in.DiskOnlySnapshotOnSuspendedVMSupported)
	out.ConsolePreferencesSupported = in.ConsolePreferencesSupported
	out.CpuFeatureMaskSupported = in.CpuFeatureMaskSupported
	out.S1AcpiManagementSupported = in.S1AcpiManagementSupported
	out.SettingScreenResolutionSupported = in.SettingScreenResolutionSupported
	out.SettingDisplayTopologySupported = in.SettingDisplayTopologySupported
	out.SettingVideoRamSizeSupported = in.SettingVideoRamSizeSupported
	out.ToolsAutoUpdateSupported = in.ToolsAutoUpdateSupported
	out.ToolsSyncTimeSupported = in.ToolsSyncTimeSupported
	out.ToolsSyncTimeAllowSupported = clonePointer(in.ToolsSyncTimeAllowSupported)
	out.VmNpivWwnSupported = in.VmNpivWwnSupported
	out.NpivWwnOnNonRdmVmSupported = in.NpivWwnOnNonRdmVmSupported
	out.VmNpivWwnDisableSupported = in.VmNpivWwnDisableSupported
	out.VmNpivWwnUpdateSupported = in.VmNpivWwnUpdateSupported
	out.SwapPlacementSupported = in.SwapPlacementSupported
	out.VirtualMmuUsageSupported = in.VirtualMmuUsageSupported
	out.VirtualMmuUsageIgnored = clonePointer(in.VirtualMmuUsageIgnored)
	out.VirtualExecUsageIgnored = clonePointer(in.VirtualExecUsageIgnored)
	out.DiskSharesSupported = in.DiskSharesSupported
	out.BootOptionsSupported = in.BootOptionsSupported
	out.BootRetryOptionsSupported = in.BootRetryOptionsSupported
	out.ChangeTrackingSupported = in.ChangeTrackingSupported
	out.HostBasedReplicationSupported = in.HostBasedReplicationSupported
	out.GuestAutoLockSupported = in.GuestAutoLockSupported
	out.MemoryReservationLockSupported = in.MemoryReservationLockSupported
	out.FeatureRequirementSupported = in.FeatureRequirementSupported
	out.PoweredOnMonitorTypeChangeSupported = in.PoweredOnMonitorTypeChangeSupported
	out.SeSparseDiskSupported = in.SeSparseDiskSupported
	out.MultipleCoresPerSocketSupported = in.MultipleCoresPerSocketSupported
	out.NestedHVSupported = in.NestedHVSupported
	out.VPMCSupported = in.VPMCSupported
	out.PerVmEvcSupported = clonePointer(in.PerVmEvcSupported)
	out.SecureBootSupported = clonePointer(in.SecureBootSupported)
	out.SuspendToMemorySupported = clonePointer(in.SuspendToMemorySupported)
	out.RequireSgxAttestationSupported = clonePointer(in.RequireSgxAttestationSupported)
	out.ChangeModeDisksSupported = clonePointer(in.ChangeModeDisksSupported)
	out.VendorDeviceGroupSupported = clonePointer(in.VendorDeviceGroupSupported)
	out.SEVSupported = clonePointer(in.SevSupported)
	out.SEVSNPSupported = clonePointer(in.SevSnpSupported)
	out.TDXSupported = clonePointer(in.TdxSupported)
	out.DisableSnapshotsSupported = func(v bool) *bool { out := v; return &out }(in.DisableSnapshotsSupported)
	out.PMEMFailoverSupported = clonePointer(in.PmemFailoverSupported)
	out.RecordReplaySupported = func(v bool) *bool { out := v; return &out }(in.RecordReplaySupported)
	return out
}

func convertStorageClassOption(in vimtypes.DatastoreOption) vimv1.StorageClassOption {
	var out vimv1.StorageClassOption
	out.UnsupportedVolumes = func(in []vimtypes.VirtualMachineDatastoreVolumeOption) []vimv1.VirtualMachineStorageClassVolumeOption {
		if in == nil {
			return nil
		}
		out := make([]vimv1.VirtualMachineStorageClassVolumeOption, len(in))
		for i, item := range in {
			out[i] = convertVirtualMachineStorageClassVolumeOption(item)
		}
		return out
	}(in.UnsupportedVolumes)
	return out
}

func convertVirtualMachineStorageClassVolumeOption(in vimtypes.VirtualMachineDatastoreVolumeOption) vimv1.VirtualMachineStorageClassVolumeOption {
	var out vimv1.VirtualMachineStorageClassVolumeOption
	out.FileSystemType = in.FileSystemType
	out.MajorVersion = in.MajorVersion
	return out
}

func convertVirtualMachinePropertyRelation(in vimtypes.VirtualMachinePropertyRelation) vimv1.VirtualMachinePropertyRelation {
	var out vimv1.VirtualMachinePropertyRelation
	out.Key = convertVirtualMachineProperty(in.Key)
	out.Relations = func(in []vimtypes.DynamicProperty) []vimv1.VirtualMachineProperty {
		if in == nil {
			return nil
		}
		out := make([]vimv1.VirtualMachineProperty, len(in))
		for i, item := range in {
			out[i] = convertVirtualMachineProperty(item)
		}
		return out
	}(in.Relations)
	return out
}

func convertVirtualMachineProperty(in vimtypes.DynamicProperty) vimv1.VirtualMachineProperty {
	var out vimv1.VirtualMachineProperty
	out.Name = in.Name
	if in.Val != nil {
		out.Value = fmt.Sprint(in.Val)
	}
	return out
}

func convertVirtualMachineGuestOptionsHardwareVersionStatus(in vimtypes.GuestOsDescriptor) vimv1.VirtualMachineGuestOptionsHardwareVersionStatus {
	var out vimv1.VirtualMachineGuestOptionsHardwareVersionStatus
	out.SupportedMaxCPUs = in.SupportedMaxCPUs
	out.NumSupportedPhysicalSockets = in.NumSupportedPhysicalSockets
	out.NumSupportedCoresPerSocket = in.NumSupportedCoresPerSocket
	out.SupportedMaxMem = optionalMB(int64(in.SupportedMaxMemMB))
	out.SupportedMinMem = optionalMB(int64(in.SupportedMinMemMB))
	out.RecommendedMem = optionalMB(int64(in.RecommendedMemMB))
	out.RecommendedColorDepth = in.RecommendedColorDepth
	out.RecommendedDiskSize = optionalMB(int64(in.RecommendedDiskSizeMB))
	out.SupportedNumDisks = in.SupportedNumDisks
	out.RecommendedDiskController = vimv1.VirtualControllerType(deviceType(in.RecommendedDiskController))
	out.RecommendedSCSIController = vimv1.VirtualControllerType(deviceType(in.RecommendedSCSIController))
	out.RecommendedCdromController = vimv1.VirtualControllerType(deviceType(in.RecommendedCdromController))
	out.SupportedDiskControllers = func(in []string) []vimv1.VirtualControllerType {
		if in == nil {
			return nil
		}
		out := make([]vimv1.VirtualControllerType, len(in))
		for i, item := range in {
			out[i] = vimv1.VirtualControllerType(deviceType(item))
		}
		return out
	}(in.SupportedDiskControllerList)
	out.RecommendedEthernetCard = vimv1.EthernetCardType(deviceType(in.RecommendedEthernetCard))
	out.SupportedEthernetCards = func(in []string) []vimv1.EthernetCardType {
		if in == nil {
			return nil
		}
		out := make([]vimv1.EthernetCardType, len(in))
		for i, item := range in {
			out[i] = vimv1.EthernetCardType(deviceType(item))
		}
		return out
	}(in.SupportedEthernetCard)
	out.SupportedUSBControllerList = func(in []string) []vimv1.VirtualUSBControllerType {
		if in == nil {
			return nil
		}
		out := make([]vimv1.VirtualUSBControllerType, len(in))
		for i, item := range in {
			out[i] = vimv1.VirtualUSBControllerType(deviceType(item))
		}
		return out
	}(in.SupportedUSBControllerList)
	out.RecommendedUSBController = vimv1.VirtualUSBControllerType(deviceType(in.RecommendedUSBController))
	out.WakeOnLanEthernetCard = func(in []string) []vimv1.EthernetCardType {
		if in == nil {
			return nil
		}
		out := make([]vimv1.EthernetCardType, len(in))
		for i, item := range in {
			out[i] = vimv1.EthernetCardType(deviceType(item))
		}
		return out
	}(in.WakeOnLanEthernetCard)
	out.RecommendedFirmware = vimv1.Firmware(strings.ToUpper(in.RecommendedFirmware))
	out.SupportedFirmware = func(in []string) []vimv1.Firmware {
		if in == nil {
			return nil
		}
		out := make([]vimv1.Firmware, len(in))
		for i, item := range in {
			out[i] = vimv1.Firmware(strings.ToUpper(item))
		}
		return out
	}(in.SupportedFirmware)
	out.VRAMSize = func(v vimtypes.IntOption) *vimv1.ResourceQuantityOption {
		out := quantityOption(vimtypes.LongOption{OptionType: v.OptionType, Min: int64(v.Min), Max: int64(v.Max), DefaultValue: int64(v.DefaultValue)}, 1024)
		return &out
	}(in.VRAMSizeInKB)
	out.NumSupportedFloppyDevices = in.NumSupportedFloppyDevices
	out.NumRecommendedPhysicalSockets = in.NumRecommendedPhysicalSockets
	out.NumRecommendedCoresPerSocket = in.NumRecommendedCoresPerSocket
	out.SupportsSlaveDisk = clonePointer(in.SupportsSlaveDisk)
	out.SmcRequired = in.SmcRequired
	out.SmcRecommended = in.SmcRecommended
	out.Ich7mRecommended = in.Ich7mRecommended
	out.UsbRecommended = in.UsbRecommended
	out.SupportsWakeOnLan = in.SupportsWakeOnLan
	out.SupportsVMI = in.SupportsVMI
	out.Supports3D = in.Supports3D
	out.Recommended3D = in.Recommended3D
	out.SupportsSecureBoot = clonePointer(in.SupportsSecureBoot)
	out.CPUFeatureMask = convertCPUFeatureMask(in.CpuFeatureMask)
	out.DefaultSecureBoot = clonePointer(in.DefaultSecureBoot)
	out.SupportsCpuHotAdd = in.SupportsCpuHotAdd
	out.SupportsCpuHotRemove = in.SupportsCpuHotRemove
	out.SupportsMemoryHotAdd = in.SupportsMemoryHotAdd
	out.SupportsPvscsiControllerForBoot = in.SupportsPvscsiControllerForBoot
	out.DiskUuidEnabled = in.DiskUuidEnabled
	out.SupportsHotPlugPCI = in.SupportsHotPlugPCI
	out.SupportsTPM20 = clonePointer(in.SupportsTPM20)
	out.RecommendedTPM20 = clonePointer(in.RecommendedTPM20)
	out.VvtdSupported = mapPointer(in.VvtdSupported, convertBoolOption)
	out.VbsSupported = mapPointer(in.VbsSupported, convertBoolOption)
	out.VsgxSupported = mapPointer(in.VsgxSupported, convertBoolOption)
	out.VsgxRemoteAttestationSupported = clonePointer(in.VsgxRemoteAttestationSupported)
	out.VwdtSupported = clonePointer(in.VwdtSupported)
	out.PersistentMemorySupported = clonePointer(in.PersistentMemorySupported)
	out.SupportedMinPersistentMemory = optionalMB(in.SupportedMinPersistentMemoryMB)
	out.SupportedMaxPersistentMemory = optionalMB(in.SupportedMaxPersistentMemoryMB)
	out.RecommendedPersistentMemory = optionalMB(in.RecommendedPersistentMemoryMB)
	out.PersistentMemoryHotAddSupported = clonePointer(in.PersistentMemoryHotAddSupported)
	out.PersistentMemoryHotRemoveSupported = clonePointer(in.PersistentMemoryHotRemoveSupported)
	out.PersistentMemoryColdGrowthSupported = clonePointer(in.PersistentMemoryColdGrowthSupported)
	out.PersistentMemoryColdGrowthGranularity = func(v int64) *vimv1.ResourceQuantityOption {
		if v == 0 {
			return nil
		}
		out := quantityOption(vimtypes.LongOption{Min: v, Max: v, DefaultValue: v}, 1024*1024)
		return &out
	}(in.PersistentMemoryColdGrowthGranularityMB)
	out.PersistentMemoryHotGrowthSupported = clonePointer(in.PersistentMemoryHotGrowthSupported)
	out.PersistentMemoryHotGrowthGranularity = func(v int64) *vimv1.ResourceQuantityOption {
		if v == 0 {
			return nil
		}
		out := quantityOption(vimtypes.LongOption{Min: v, Max: v, DefaultValue: v}, 1024*1024)
		return &out
	}(in.PersistentMemoryHotGrowthGranularityMB)
	out.SupportedForCreate = in.SupportedForCreate
	out.SupportLevel = func(v string) vimv1.SupportLevel { var out vimv1.SupportLevel; out.FromVimType(v); return out }(in.SupportLevel)
	return out
}

//nolint:staticcheck // Preserve the existing API field without changing its type.
func convertHostCPUIDInfo(in vimtypes.HostCpuIdInfo) vimv1.HostCpuIdInfo {
	var out vimv1.HostCpuIdInfo
	out.Level = in.Level
	out.Vendor = in.Vendor
	out.Eax = in.Eax
	out.Ebx = in.Ebx
	out.Ecx = in.Ecx
	out.Edx = in.Edx
	return out
}

func clonePointer[T any](in *T) *T {
	if in == nil {
		return nil
	}
	return ptr.To(*in)
}

func mapPointer[A, B any](in *A, convert func(A) B) *B {
	if in == nil {
		return nil
	}
	return ptr.To(convert(*in))
}

func optionalMB(value int64) *resource.Quantity {
	if value <= 0 {
		return nil
	}
	return resource.NewQuantity(value*1024*1024, resource.BinarySI)
}

//nolint:staticcheck // The discovery API retains the guest descriptor's legacy CPUID masks.
func convertCPUFeatureMask(in []vimtypes.HostCpuIdInfo) []vimv1.HostCpuIdInfo {
	if in == nil {
		return nil
	}
	out := make([]vimv1.HostCpuIdInfo, len(in))
	for i, item := range in {
		out[i] = convertHostCPUIDInfo(item)
	}
	return out
}
