// Copyright (c) 2018-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	goctx "context"
	"fmt"
	"reflect"
	"time"

	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg"
	pkgconfig "github.com/vmware-tanzu/vm-operator/pkg/config"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	kubeutil "github.com/vmware-tanzu/vm-operator/pkg/util/kube"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/clustermodules"
	providercfg "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/virtualmachine"
)

type VMMetadata struct {
	Data      map[string]string
	Transport vmopv1.VirtualMachineMetadataTransport
}

// VMUpdateArgs contains the arguments needed to update a VM on VC.
type VMUpdateArgs struct {
	VMClass        *vmopv1.VirtualMachineClass
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	MinCPUFreq     uint64
	ExtraConfig    map[string]string
	VMMetadata     VMMetadata

	ConfigSpec      *vimTypes.VirtualMachineConfigSpec
	ClassConfigSpec *vimTypes.VirtualMachineConfigSpec

	NetIfList      network.InterfaceInfoList
	DNSServers     []string
	SearchSuffixes []string

	// hack. Remove after VMSVC-1261.
	// indicating if this VM image used is VM service v1alpha1 compatible.
	VirtualMachineImageV1Alpha1Compatible bool
}

func ethCardMatch(ctx goctx.Context, newBaseEthCard, curBaseEthCard vimTypes.BaseVirtualEthernetCard) bool {
	if pkgconfig.FromContext(ctx).Features.VMClassAsConfigDayNDate {
		if reflect.TypeOf(curBaseEthCard) != reflect.TypeOf(newBaseEthCard) {
			return false
		}
	}

	curEthCard := curBaseEthCard.GetVirtualEthernetCard()
	newEthCard := newBaseEthCard.GetVirtualEthernetCard()
	if newEthCard.AddressType == string(vimTypes.VirtualEthernetCardMacTypeManual) {
		// If the new card has an assigned MAC address, then it should match with
		// the current card. Note only NCP sets the MAC address.
		if newEthCard.MacAddress != curEthCard.MacAddress {
			return false
		}
	}

	if newEthCard.ExternalId != "" {
		// If the new card has a specific ExternalId, then it should match with the
		// current card. Note only NCP sets the ExternalId.
		if newEthCard.ExternalId != curEthCard.ExternalId {
			return false
		}
	}

	return true
}

func UpdateEthCardDeviceChanges(
	ctx goctx.Context,
	expectedEthCards object.VirtualDeviceList,
	currentEthCards object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	var deviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
	for _, expectedDev := range expectedEthCards {
		expectedNic := expectedDev.(vimTypes.BaseVirtualEthernetCard)
		expectedBacking := expectedNic.GetVirtualEthernetCard().Backing
		expectedBackingType := reflect.TypeOf(expectedBacking)

		var matchingIdx = -1

		// Try to match the expected NIC with an existing NIC but this isn't that great. We mostly
		// depend on the backing but we can improve that later on. When not generated, we could use
		// the MAC address. When we support something other than just vmxnet3 we should compare
		// those types too. And we should make this truly reconcile as well by comparing the full
		// state (support EDIT instead of only ADD/REMOVE operations).
		//
		// Another tack we could take is force the VM's device order to match the Spec order, but
		// that could lead to spurious removals. Or reorder the NetIfList to not be that of the
		// Spec, but in VM device order.
		for idx, curDev := range currentEthCards {
			nic := curDev.(vimTypes.BaseVirtualEthernetCard)

			// This assumes we don't have multiple NICs in the same backing network. This is kind of, sort
			// of enforced by the webhook, but we lack a guaranteed way to match up the NICs.

			if !ethCardMatch(ctx, expectedNic, nic) {
				continue
			}

			db := nic.GetVirtualEthernetCard().Backing
			if db == nil || reflect.TypeOf(db) != expectedBackingType {
				continue
			}

			var backingMatch bool

			// Cribbed from VirtualDeviceList.SelectByBackingInfo().
			switch a := db.(type) {
			case *vimTypes.VirtualEthernetCardNetworkBackingInfo:
				// This backing is only used in testing.
				b := expectedBacking.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				backingMatch = a.DeviceName == b.DeviceName
			case *vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo:
				b := expectedBacking.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				backingMatch = a.Port.SwitchUuid == b.Port.SwitchUuid && a.Port.PortgroupKey == b.Port.PortgroupKey
			case *vimTypes.VirtualEthernetCardOpaqueNetworkBackingInfo:
				b := expectedBacking.(*vimTypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				backingMatch = a.OpaqueNetworkId == b.OpaqueNetworkId
			}

			if backingMatch {
				matchingIdx = idx
				break
			}
		}

		if matchingIdx == -1 {
			// No matching backing found so add new card.
			deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
				Device:    expectedDev,
				Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
			})
		} else {
			// Matching backing found so keep this card (don't remove it below after this loop).
			currentEthCards = append(currentEthCards[:matchingIdx], currentEthCards[matchingIdx+1:]...)
		}
	}

	// Remove any unmatched existing interfaces.
	removeDeviceChanges := make([]vimTypes.BaseVirtualDeviceConfigSpec, 0, len(currentEthCards))
	for _, dev := range currentEthCards {
		removeDeviceChanges = append(removeDeviceChanges, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

// UpdatePCIDeviceChanges returns devices changes for PCI devices attached to a VM. There are 2 types of PCI devices
// processed here and in case of cloning a VM, devices listed in VMClass are considered as source of truth.
func UpdatePCIDeviceChanges(
	expectedPciDevices object.VirtualDeviceList,
	currentPciDevices object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	var deviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
	for _, expectedDev := range expectedPciDevices {
		expectedPci := expectedDev.(*vimTypes.VirtualPCIPassthrough)
		expectedBacking := expectedPci.Backing
		expectedBackingType := reflect.TypeOf(expectedBacking)

		var matchingIdx = -1
		for idx, curDev := range currentPciDevices {
			curBacking := curDev.GetVirtualDevice().Backing
			if curBacking == nil || reflect.TypeOf(curBacking) != expectedBackingType {
				continue
			}

			var backingMatch bool
			switch a := curBacking.(type) {
			case *vimTypes.VirtualPCIPassthroughVmiopBackingInfo:
				b := expectedBacking.(*vimTypes.VirtualPCIPassthroughVmiopBackingInfo)
				backingMatch = a.Vgpu == b.Vgpu

			case *vimTypes.VirtualPCIPassthroughDynamicBackingInfo:
				currAllowedDevs := a.AllowedDevice
				b := expectedBacking.(*vimTypes.VirtualPCIPassthroughDynamicBackingInfo)
				if a.CustomLabel == b.CustomLabel {
					// b.AllowedDevice has only one element because CreatePCIDevices() adds only one device based
					// on the devices listed in vmclass.spec.hardware.devices.dynamicDirectPathIODevices.
					expectedAllowedDev := b.AllowedDevice[0]
					for i := 0; i < len(currAllowedDevs) && !backingMatch; i++ {
						backingMatch = expectedAllowedDev.DeviceId == currAllowedDevs[i].DeviceId &&
							expectedAllowedDev.VendorId == currAllowedDevs[i].VendorId
					}
				}
			}

			if backingMatch {
				matchingIdx = idx
				break
			}
		}

		if matchingIdx == -1 {
			deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
				Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    expectedPci,
			})
		} else {
			// There could be multiple vGPUs with same BackingInfo. Remove current device if matching found.
			currentPciDevices = append(currentPciDevices[:matchingIdx], currentPciDevices[matchingIdx+1:]...)
		}
	}
	// Remove any unmatched existing devices.
	removeDeviceChanges := make([]vimTypes.BaseVirtualDeviceConfigSpec, 0, len(currentPciDevices))
	for _, dev := range currentPciDevices {
		removeDeviceChanges = append(removeDeviceChanges, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

func UpdateConfigSpecCPUAllocation(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec,
	minCPUFreq uint64) {

	cpuAllocation := config.CpuAllocation
	var cpuReservation *int64
	var cpuLimit *int64

	if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
		rsv := virtualmachine.CPUQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minCPUFreq)
		if cpuAllocation == nil || cpuAllocation.Reservation == nil || *cpuAllocation.Reservation != rsv {
			cpuReservation = &rsv
		}
	}

	if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
		lim := virtualmachine.CPUQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minCPUFreq)
		if cpuAllocation == nil || cpuAllocation.Limit == nil || *cpuAllocation.Limit != lim {
			cpuLimit = &lim
		}
	}

	if cpuReservation != nil || cpuLimit != nil {
		configSpec.CpuAllocation = &vimTypes.ResourceAllocationInfo{
			Reservation: cpuReservation,
			Limit:       cpuLimit,
		}
	}
}

func UpdateConfigSpecMemoryAllocation(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec) {

	memAllocation := config.MemoryAllocation
	var memoryReservation *int64
	var memoryLimit *int64

	if !vmClassSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := virtualmachine.MemoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
		if memAllocation == nil || memAllocation.Reservation == nil || *memAllocation.Reservation != rsv {
			memoryReservation = &rsv
		}
	}

	if !vmClassSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := virtualmachine.MemoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
		if memAllocation == nil || memAllocation.Limit == nil || *memAllocation.Limit != lim {
			memoryLimit = &lim
		}
	}

	if memoryReservation != nil || memoryLimit != nil {
		configSpec.MemoryAllocation = &vimTypes.ResourceAllocationInfo{
			Reservation: memoryReservation,
			Limit:       memoryLimit,
		}
	}
}

func UpdateConfigSpecExtraConfig(
	ctx goctx.Context,
	configInfo *vimTypes.VirtualMachineConfigInfo,
	configSpec,
	classConfigSpec *vimTypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec,
	vm *vmopv1.VirtualMachine,
	globalExtraConfig map[string]string,
	imageV1Alpha1Compatible bool) {

	extraConfig := make(map[string]string)
	for k, v := range globalExtraConfig {
		extraConfig[k] = v
	}

	virtualDevices := vmClassSpec.Hardware.Devices
	pciPassthruFromConfigSpec := util.SelectVirtualPCIPassthrough(util.DevicesFromConfigSpec(classConfigSpec))
	if len(virtualDevices.VGPUDevices) > 0 || len(virtualDevices.DynamicDirectPathIODevices) > 0 || len(pciPassthruFromConfigSpec) > 0 {
		setMMIOExtraConfig(vm, extraConfig)
	}

	if pkgconfig.FromContext(ctx).Features.VMClassAsConfigDayNDate {
		// Merge non intersecting keys from the desired config spec extra config with the class config spec extra config
		// (ie) class config spec extra config keys takes precedence over the desired config spec extra config keys
		ecFromClassConfigSpec := util.ExtraConfigToMap(classConfigSpec.ExtraConfig)
		mergedExtraConfig := classConfigSpec.ExtraConfig
		for k, v := range extraConfig {
			if _, exists := ecFromClassConfigSpec[k]; !exists {
				mergedExtraConfig = append(mergedExtraConfig, &vimTypes.OptionValue{Key: k, Value: v})
			}
		}
		extraConfig = util.ExtraConfigToMap(mergedExtraConfig)
	}

	// Ensure MMPowerOffVMExtraConfigKey is no longer part of ExtraConfig as
	// setting it to an empty value removes it.
	for i := range configInfo.ExtraConfig {
		if o := configInfo.ExtraConfig[i].GetOptionValue(); o != nil {
			if o.Key == constants.MMPowerOffVMExtraConfigKey && o.Value != "" {
				extraConfig[constants.MMPowerOffVMExtraConfigKey] = ""
			}
		}
	}

	configSpec.ExtraConfig = util.MergeExtraConfig(configInfo.ExtraConfig, extraConfig)

	// Enabling the defer-cloud-init extraConfig key for V1Alpha1Compatible images defers cloud-init from running on first boot
	// and disables networking configurations by cloud-init. Therefore, only set the extraConfig key to enabled
	// when the vmMetadata is nil or when the transport requested is not CloudInit.
	// VMSVC-1261: we may always set this extra config key to remove image from VM customization.
	// If a VM is deployed from an incompatible image,
	// it will do nothing and won't cause any issues, but can introduce confusion.
	if vm.Spec.VmMetadata == nil || vm.Spec.VmMetadata.Transport != vmopv1.VirtualMachineMetadataCloudInitTransport {
		ecMap := util.ExtraConfigToMap(configInfo.ExtraConfig)
		if ecMap[constants.VMOperatorV1Alpha1ExtraConfigKey] == constants.VMOperatorV1Alpha1ConfigReady &&
			imageV1Alpha1Compatible {
			// Set VMOperatorV1Alpha1ExtraConfigKey for v1alpha1 VirtualMachineImage compatibility.
			configSpec.ExtraConfig = append(configSpec.ExtraConfig,
				&vimTypes.OptionValue{Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigEnabled})
		}
	}

}

func setMMIOExtraConfig(vm *vmopv1.VirtualMachine, extraConfig map[string]string) {
	mmioSize := vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation]
	if mmioSize == "" {
		mmioSize = constants.PCIPassthruMMIOSizeDefault
	}
	if mmioSize != "0" {
		extraConfig[constants.PCIPassthruMMIOExtraConfigKey] = constants.ExtraConfigTrue
		extraConfig[constants.PCIPassthruMMIOSizeExtraConfigKey] = mmioSize
	}
}

func UpdateConfigSpecChangeBlockTracking(
	ctx goctx.Context,
	configInfo *vimTypes.VirtualMachineConfigInfo,
	configSpec, classConfigSpec *vimTypes.VirtualMachineConfigSpec,
	vmSpec vmopv1.VirtualMachineSpec) {
	// When VM_Class_as_Config_DaynDate is enabled, class config spec cbt if
	// set overrides the VM spec advanced options cbt.
	if pkgconfig.FromContext(ctx).Features.VMClassAsConfigDayNDate && classConfigSpec != nil {
		if classConfigSpec.ChangeTrackingEnabled != nil {
			if !apiEquality.Semantic.DeepEqual(configInfo.ChangeTrackingEnabled, classConfigSpec.ChangeTrackingEnabled) {
				configSpec.ChangeTrackingEnabled = classConfigSpec.ChangeTrackingEnabled
			}
			return
		}
	}

	if vmSpec.AdvancedOptions == nil || vmSpec.AdvancedOptions.ChangeBlockTracking == nil {
		// Treat this as we preserve whatever the current CBT status is. I think we'd need
		// to store somewhere what the original state was anyways.
		return
	}

	if !apiEquality.Semantic.DeepEqual(configInfo.ChangeTrackingEnabled, vmSpec.AdvancedOptions.ChangeBlockTracking) {
		configSpec.ChangeTrackingEnabled = vmSpec.AdvancedOptions.ChangeBlockTracking
	}
}

func UpdateHardwareConfigSpec(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec) {

	if nCPUs := int32(vmClassSpec.Hardware.Cpus); config.Hardware.NumCPU != nCPUs {
		configSpec.NumCPUs = nCPUs
	}
	if memMB := virtualmachine.MemoryQuantityToMb(vmClassSpec.Hardware.Memory); int64(config.Hardware.MemoryMB) != memMB {
		configSpec.MemoryMB = memMB
	}
}

func UpdateConfigSpecAnnotation(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec) {
	if config.Annotation != constants.VCVMAnnotation {
		configSpec.Annotation = constants.VCVMAnnotation
	}
}

func UpdateConfigSpecManagedBy(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec) {
	if config.ManagedBy == nil {
		configSpec.ManagedBy = &vimTypes.ManagedByInfo{
			ExtensionKey: vmopv1.ManagedByExtensionKey,
			Type:         vmopv1.ManagedByExtensionType,
		}
	}
}

func UpdateConfigSpecFirmware(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vm *vmopv1.VirtualMachine) {

	if val, ok := vm.Annotations[constants.FirmwareOverrideAnnotation]; ok {
		if (val == "efi" || val == "bios") && config.Firmware != val {
			configSpec.Firmware = val
		}
	}
}

// updateConfigSpec overlays the VM Class spec with the provided ConfigSpec to form a desired
// ConfigSpec that will be used to reconfigure the VM.
func updateConfigSpec(
	vmCtx context.VirtualMachineContext,
	configInfo *vimTypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) *vimTypes.VirtualMachineConfigSpec {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	vmClassSpec := updateArgs.VMClass.Spec

	// Before VM Class as Config, VMs were deployed from the OVA, and are then
	// reconfigured to match the desired CPU and memory reservation.  Maintain that
	// behavior.  With the FSS enabled, VMs will be _created_ with desired HW spec, and we
	// will not modify the hardware of the VM post creation.  So, don't populate the
	// Hardware config and CPU/Memory reservation.
	if !pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate {
		UpdateHardwareConfigSpec(configInfo, configSpec, &vmClassSpec)
		UpdateConfigSpecCPUAllocation(configInfo, configSpec, &vmClassSpec, updateArgs.MinCPUFreq)
		UpdateConfigSpecMemoryAllocation(configInfo, configSpec, &vmClassSpec)
	}

	UpdateConfigSpecAnnotation(configInfo, configSpec)
	UpdateConfigSpecManagedBy(configInfo, configSpec)
	UpdateConfigSpecExtraConfig(vmCtx, configInfo, configSpec, updateArgs.ConfigSpec, &vmClassSpec,
		vmCtx.VM, updateArgs.ExtraConfig, updateArgs.VirtualMachineImageV1Alpha1Compatible)
	UpdateConfigSpecChangeBlockTracking(vmCtx, configInfo, configSpec, updateArgs.ConfigSpec, vmCtx.VM.Spec)
	UpdateConfigSpecFirmware(configInfo, configSpec, vmCtx.VM)

	return configSpec
}

func (s *Session) prePowerOnVMConfigSpec(
	vmCtx context.VirtualMachineContext,
	configInfo *vimTypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimTypes.VirtualMachineConfigSpec, error) {

	configSpec := updateConfigSpec(vmCtx, configInfo, updateArgs)

	virtualDevices := object.VirtualDeviceList(configInfo.Hardware.Device)
	currentDisks := virtualDevices.SelectByType((*vimTypes.VirtualDisk)(nil))
	currentEthCards := virtualDevices.SelectByType((*vimTypes.VirtualEthernetCard)(nil))
	currentPciDevices := virtualDevices.SelectByType((*vimTypes.VirtualPCIPassthrough)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx, currentDisks)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, diskDeviceChanges...)

	expectedEthCards := updateArgs.NetIfList.GetVirtualDeviceList()
	ethCardDeviceChanges, err := UpdateEthCardDeviceChanges(vmCtx, expectedEthCards, currentEthCards)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, ethCardDeviceChanges...)

	var expectedPCIDevices []vimTypes.BaseVirtualDevice
	if pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate {
		if configSpecDevs := util.DevicesFromConfigSpec(updateArgs.ConfigSpec); len(configSpecDevs) > 0 {
			pciPassthruFromConfigSpec := util.SelectVirtualPCIPassthrough(configSpecDevs)
			expectedPCIDevices = virtualmachine.CreatePCIDevicesFromConfigSpec(pciPassthruFromConfigSpec)
		}
	} else {
		expectedPCIDevices = virtualmachine.CreatePCIDevicesFromVMClass(updateArgs.VMClass.Spec.Hardware.Devices)
	}

	pciDeviceChanges, err := UpdatePCIDeviceChanges(expectedPCIDevices, currentPciDevices)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, pciDeviceChanges...)

	return configSpec, nil
}

func (s *Session) prePowerOnVMReconfigure(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	configSpec, err := s.prePowerOnVMConfigSpec(vmCtx, config, updateArgs)
	if err != nil {
		return err
	}

	defaultConfigSpec := &vimTypes.VirtualMachineConfigSpec{}
	if !apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		vmCtx.Logger.Info("Pre PowerOn Reconfigure", "configSpec", configSpec)
		if err := resVM.Reconfigure(vmCtx, configSpec); err != nil {
			vmCtx.Logger.Error(err, "pre power on reconfigure failed")
			return err
		}
	}

	return nil
}

func (s *Session) ensureNetworkInterfaces(
	vmCtx context.VirtualMachineContext,
	configSpec *vimTypes.VirtualMachineConfigSpec) (network.InterfaceInfoList, error) {

	// This negative device key is the traditional range used for network interfaces.
	deviceKey := int32(-100)

	var networkDevices []vimTypes.BaseVirtualDevice
	if pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate && configSpec != nil {
		networkDevices = util.SelectDevicesByTypes(
			util.DevicesFromConfigSpec(configSpec),
			&vimTypes.VirtualE1000{},
			&vimTypes.VirtualE1000e{},
			&vimTypes.VirtualPCNet32{},
			&vimTypes.VirtualVmxnet2{},
			&vimTypes.VirtualVmxnet3{},
			&vimTypes.VirtualVmxnet3Vrdma{},
			&vimTypes.VirtualSriovEthernetCard{},
		)
	}

	// XXX: The following logic assumes that the order of network interfaces specified in the
	// VM spec matches one to one with the device changes in the ConfigSpec in VM class.
	// This is a safe assumption for now since VM service only supports one network interface.
	// TODO: Needs update when VM Service supports VMs with more then one network interface.
	var netIfList = make(network.InterfaceInfoList, len(vmCtx.VM.Spec.NetworkInterfaces))
	for i := range vmCtx.VM.Spec.NetworkInterfaces {
		vif := vmCtx.VM.Spec.NetworkInterfaces[i]

		info, err := s.NetworkProvider.EnsureNetworkInterface(vmCtx, &vif)
		if err != nil {
			return nil, err
		}

		if pkgconfig.FromContext(vmCtx).Features.VMClassAsConfigDayNDate {
			// If VM Class-as-a-Config is supported, we use the network device from the Class.
			// If the VM class doesn't specify enough number of network devices, we fall back to default behavior.
			if i < len(networkDevices) {
				ethCardFromNetProvider := info.Device.(vimTypes.BaseVirtualEthernetCard)

				if mac := ethCardFromNetProvider.GetVirtualEthernetCard().MacAddress; mac != "" {
					networkDevices[i].(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = mac
					networkDevices[i].(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimTypes.VirtualEthernetCardMacTypeManual)
				}

				networkDevices[i].(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId =
					ethCardFromNetProvider.GetVirtualEthernetCard().ExternalId
				// If the device from VM class has a DVX backing, this should still work if the backing as well
				// as the DVX backing are set. VPXD checks for DVX backing before checking for normal device backings.
				networkDevices[i].(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().Backing =
					ethCardFromNetProvider.GetVirtualEthernetCard().Backing

				info.Device = networkDevices[i]
			}
		}

		// govmomi assigns a random device key. Fix that up here.
		info.Device.GetVirtualDevice().Key = deviceKey
		netIfList[i] = *info

		deviceKey--
	}

	return netIfList, nil
}

func (s *Session) ensureCNSVolumes(vmCtx context.VirtualMachineContext) error {
	// If VM spec has a PVC, check if the volume is attached before powering on
	for _, volume := range vmCtx.VM.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// Don't process VsphereVolumes here. Note that we don't have Volume status
			// for Vsphere volumes.
			continue
		}

		found := false
		for _, volumeStatus := range vmCtx.VM.Status.Volumes {
			if volumeStatus.Name == volume.Name {
				found = true
				if !volumeStatus.Attached {
					return fmt.Errorf("persistent volume: %s not attached to VM", volume.Name)
				}
				break
			}
		}

		if !found {
			return fmt.Errorf("status update pending for persistent volume: %s on VM", volume.Name)
		}
	}

	return nil
}

func (s *Session) prepareVMForPowerOn(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	cfg *vimTypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	netIfList, err := s.ensureNetworkInterfaces(vmCtx, updateArgs.ConfigSpec)
	if err != nil {
		return err
	}

	updateArgs.NetIfList = netIfList

	if err := MutateUpdateArgsWithDNSInformation(
		vmCtx,
		vmCtx.VM.Labels,
		updateArgs,
		s.K8sClient); err != nil {

		// Due to an internal pipeline not properly creating the ConfigMap, this
		// error is swallowed if it's a 404.
		if apierrs.IsNotFound(err) {
			vmCtx.Logger.Error(err, "VM Op ConfigMap not found")
		} else {
			return err
		}
	}

	err = s.prePowerOnVMReconfigure(vmCtx, resVM, cfg, updateArgs)
	if err != nil {
		return err
	}

	err = s.customize(vmCtx, resVM, cfg, *updateArgs)
	if err != nil {
		return err
	}

	err = s.ensureCNSVolumes(vmCtx)
	if err != nil {
		return err
	}

	return nil
}

func MutateUpdateArgsWithDNSInformation(
	ctx goctx.Context,
	vmLabels map[string]string,
	updateArgs *VMUpdateArgs,
	client ctrl.Client) error {

	nameservers, searchSuffixes, err := providercfg.GetDNSInformationFromConfigMap(ctx, client)
	if err != nil {
		return err
	}

	updateArgs.DNSServers = nameservers

	// Propagate the DNS search suffixes from the config map if this is a TKGs
	// node. Please note that this will only be used by guests for VMs created
	// by TKGs that use Cloud-Init. Guest OS Customization (GOSC) has no means
	// to set the DNS search suffix.
	if !kubeutil.HasCAPILabels(vmLabels) {
		return nil
	}

	updateArgs.SearchSuffixes = searchSuffixes
	return nil
}

func (s *Session) poweredOnVMReconfigure(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo) error {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	UpdateConfigSpecChangeBlockTracking(vmCtx, config, configSpec, nil, vmCtx.VM.Spec)

	defaultConfigSpec := &vimTypes.VirtualMachineConfigSpec{}
	if !apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		vmCtx.Logger.Info("PoweredOn Reconfigure", "configSpec", configSpec)
		if err := resVM.Reconfigure(vmCtx, configSpec); err != nil {
			vmCtx.Logger.Error(err, "powered on reconfigure failed")
			return err
		}

		// Special case for CBT: in order for CBT change take effect for a powered on VM,
		// a checkpoint save/restore is needed.  tracks the implementation of
		// this FSR internally to vSphere.
		if configSpec.ChangeTrackingEnabled != nil {
			if err := s.invokeFsrVirtualMachine(vmCtx, resVM); err != nil {
				vmCtx.Logger.Error(err, "Failed to invoke FSR for CBT update")
				return err
			}
		}
	}

	return nil
}

func (s *Session) attachClusterModule(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {

	// The clusterModule is required be able to enforce the vm-vm anti-affinity policy.
	clusterModuleName := vmCtx.VM.Annotations[pkg.ClusterModuleNameKey]
	if clusterModuleName == "" {
		return nil
	}

	// Find ClusterModule UUID from the ResourcePolicy.
	_, moduleUUID := clustermodules.FindClusterModuleUUID(vmCtx, clusterModuleName, s.Cluster.Reference(), resourcePolicy)
	if moduleUUID == "" {
		return fmt.Errorf("ClusterModule %s not found", clusterModuleName)
	}

	return s.Client.ClusterModuleClient().AddMoRefToModule(vmCtx, moduleUUID, resVM.MoRef())
}

func (s *Session) UpdateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error)) (err error) {

	resVM := res.NewVMFromObject(vcVM)

	moVM, err := resVM.GetProperties(vmCtx, []string{"config", "runtime"})
	if err != nil {
		return err
	}

	defer func() {
		updateErr := s.updateVMStatus(vmCtx, resVM)
		if updateErr != nil {
			vmCtx.Logger.Error(updateErr, "Updating VM status failed")
			if err == nil {
				err = updateErr
			}
		}
	}()

	// Translate the VM's current power state into the VM Op power state value.
	var existingPowerState vmopv1.VirtualMachinePowerState
	switch moVM.Runtime.PowerState {
	case vimTypes.VirtualMachinePowerStatePoweredOn:
		existingPowerState = vmopv1.VirtualMachinePoweredOn
	case vimTypes.VirtualMachinePowerStatePoweredOff:
		existingPowerState = vmopv1.VirtualMachinePoweredOff
	case vimTypes.VirtualMachinePowerStateSuspended:
		existingPowerState = vmopv1.VirtualMachineSuspended
	}

	switch vmCtx.VM.Spec.PowerState {
	case vmopv1.VirtualMachinePoweredOff:
		var powerOff bool
		if existingPowerState == vmopv1.VirtualMachinePoweredOn {
			powerOff = true
		} else if existingPowerState == vmopv1.VirtualMachineSuspended {
			if vmCtx.VM.Spec.PowerOffMode == vmopv1.VirtualMachinePowerOpModeHard {
				powerOff = true
			}
		}
		if powerOff {
			return resVM.SetPowerState(
				logr.NewContext(vmCtx, vmCtx.Logger),
				existingPowerState,
				vmCtx.VM.Spec.PowerState,
				vmCtx.VM.Spec.PowerOffMode)
		}

		// BMV: We'll likely want to reconfigure a powered off VM too, but right now
		// we'll defer that until the pre power on (and until more people complain
		// that the UI appears wrong).

	case vmopv1.VirtualMachineSuspended:
		if existingPowerState == vmopv1.VirtualMachinePoweredOn {
			return resVM.SetPowerState(
				logr.NewContext(vmCtx, vmCtx.Logger),
				existingPowerState,
				vmCtx.VM.Spec.PowerState,
				vmCtx.VM.Spec.SuspendMode)
		}

	case vmopv1.VirtualMachinePoweredOn:
		config := moVM.Config

		// See GoVmomi's VirtualMachine::Device() explanation for this check.
		if config == nil {
			return fmt.Errorf("VM config is not available, connectionState=%s", moVM.Runtime.ConnectionState)
		}

		switch existingPowerState {
		case vmopv1.VirtualMachinePoweredOn:

			// Check to see if a possible restart is required.
			// Please note a VM may only be restarted if it is powered on.
			if vmCtx.VM.Spec.NextRestartTime != "" {

				// If non-empty, the value of spec.nextRestartTime is guaranteed
				// to be a valid RFC3339Nano timestamp due to the webhooks,
				// however, we still check for the error due to testing that may
				// not involve webhooks.
				nextRestartTime, err := time.Parse(
					time.RFC3339Nano, vmCtx.VM.Spec.NextRestartTime)
				if err != nil {
					return fmt.Errorf(
						"spec.nextRestartTime %q cannot be parsed with %q %w",
						vmCtx.VM.Spec.NextRestartTime,
						time.RFC3339Nano,
						err)
				}
				result, err := vmutil.RestartAndWait(
					logr.NewContext(vmCtx, vmCtx.Logger),
					vcVM.Client(),
					vmutil.ManagedObjectFromObject(vcVM),
					false,
					nextRestartTime,
					vmutil.ParsePowerOpMode(string(vmCtx.VM.Spec.RestartMode)))
				if err != nil {
					return err
				}
				if result.AnyChange() {
					lastRestartTime := metav1.NewTime(nextRestartTime)
					vmCtx.VM.Status.LastRestartTime = &lastRestartTime
				}
			}

			// Do not pass classConfigSpec to poweredOnVMReconfigure when VM is
			// already powered on since we do not have to get VM class at this
			// point.
			return s.poweredOnVMReconfigure(vmCtx, resVM, config)

		case vmopv1.VirtualMachineSuspended:
			// A suspended VM cannot be reconfigured.
			return resVM.SetPowerState(
				logr.NewContext(vmCtx, vmCtx.Logger),
				existingPowerState,
				vmCtx.VM.Spec.PowerState,
				vmopv1.VirtualMachinePowerOpModeHard)
		}

		updateArgs, err := getUpdateArgsFn()
		if err != nil {
			return err
		}

		// TODO: Find a better place for this?
		if err := s.attachClusterModule(vmCtx, resVM, updateArgs.ResourcePolicy); err != nil {
			return err
		}

		if err := s.prepareVMForPowerOn(vmCtx, resVM, config, updateArgs); err != nil {
			return err
		}

		if err := resVM.SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			existingPowerState,
			vmCtx.VM.Spec.PowerState,
			vmopv1.VirtualMachinePowerOpModeHard); err != nil {
			return err
		}

		if vmCtx.VM.Annotations == nil {
			vmCtx.VM.Annotations = map[string]string{}
		}
		vmCtx.VM.Annotations[vmopv1.FirstBootDoneAnnotation] = "true"
	}
	return nil
}
