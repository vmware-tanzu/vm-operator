// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bytes"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/task"
	vimTypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/lib"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/context"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/internal"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func IsCustomizationPendingExtraConfig(extraConfig []vimTypes.BaseOptionValue) bool {
	for _, opt := range extraConfig {
		if optValue := opt.GetOptionValue(); optValue != nil {
			if optValue.Key == constants.GOSCPendingExtraConfigKey {
				return optValue.Value.(string) != ""
			}
		}
	}
	return false
}

func isCustomizationPendingError(err error) bool {
	if te, ok := err.(task.Error); ok {
		if _, ok := te.Fault().(*vimTypes.CustomizationPending); ok {
			return true
		}
	}
	return false
}

func GetLinuxCustomizationSpec(vmName string, updateArgs VmUpdateArgs) *vimTypes.CustomizationSpec {
	return &vimTypes.CustomizationSpec{
		Identity: &vimTypes.CustomizationLinuxPrep{
			HostName: &vimTypes.CustomizationFixedName{
				Name: vmName,
			},
			HwClockUTC: vimTypes.NewBool(true),
		},
		GlobalIPSettings: vimTypes.CustomizationGlobalIPSettings{
			DnsServerList: updateArgs.DNSServers,
		},
		NicSettingMap: updateArgs.NetIfList.GetInterfaceCustomizations(),
	}
}

type Netplan struct {
	Version   int                                `yaml:"version,omitempty"`
	Ethernets map[string]network.NetplanEthernet `yaml:"ethernets,omitempty"`
}

// CloudInitMetadata is used to set metadata field in
// CloudInitPrep customization spec.
type CloudInitMetadata struct {
	InstanceId    string  `yaml:"instance-id,omitempty"`
	LocalHostname string  `yaml:"local-hostname,omitempty"`
	Hostname      string  `yaml:"hostname,omitempty"`
	Network       Netplan `yaml:"network,omitempty"`
}

func GetCloudInitPrepCustomizationSpec(vmName string, currentEthCards object.VirtualDeviceList, updateArgs VmUpdateArgs) (*vimTypes.CustomizationSpec, error) {
	netplanEthernets, err := updateArgs.NetIfList.GetNetplanEthernets(currentEthCards, updateArgs.DNSServers)
	if err != nil {
		return nil, fmt.Errorf("error getting netplanEthernets %v", err)
	}

	metadataObj := &CloudInitMetadata{
		InstanceId:    vmName,
		LocalHostname: vmName,
		Hostname:      vmName,
		Network: Netplan{
			Version:   constants.NetPlanVersion,
			Ethernets: netplanEthernets,
		},
	}

	metadataBytes, err := yaml.Marshal(metadataObj)
	if err != nil {
		return nil, fmt.Errorf("yaml marshalling of CloudinitPrep metadata failed %v", err)
	}

	return &vimTypes.CustomizationSpec{
		Identity: &internal.CustomizationCloudinitPrep{
			Metadata: string(metadataBytes),
			Userdata: updateArgs.VmMetadata.Data["user-data"],
		},
	}, nil
}

func (s *Session) customizeVM(
	vmCtx context.VMContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VmUpdateArgs) error {

	if val := vmCtx.VM.Annotations[constants.VSphereCustomizationBypassKey]; val == constants.VSphereCustomizationBypassDisable {
		vmCtx.Logger.Info("Skipping vsphere customization because of vsphere-customization bypass annotation")
		return nil
	}

	if IsCustomizationPendingExtraConfig(config.ExtraConfig) {
		vmCtx.Logger.Info("Skipping customization because it is already pending")
		// TODO: We should really determine if the pending customization is stale, clear it
		// if so, and then re-customize. Otherwise, the Customize call could perpetually fail
		// preventing power on.
		return nil
	}

	var customizationSpec *vimTypes.CustomizationSpec
	if updateArgs.VmMetadata != nil && updateArgs.VmMetadata.Transport == v1alpha1.VirtualMachineMetadataCloudInitTransport {
		currentEthCards, err := resVM.GetNetworkDevices(vmCtx)
		if err != nil {
			return err
		}

		customizationSpec, err = GetCloudInitPrepCustomizationSpec(vmCtx.VM.Name, currentEthCards, updateArgs)
		if err != nil {
			return err
		}
	} else {
		// TODO: Don't assume Linux; support Windows.
		customizationSpec = GetLinuxCustomizationSpec(vmCtx.VM.Name, updateArgs)
	}

	vmCtx.Logger.Info("Customizing VM", "customizationSpec", *customizationSpec)
	if err := resVM.Customize(vmCtx, *customizationSpec); err != nil {
		// isCustomizationPendingExtraConfig() above is supposed to prevent this error, but
		// handle it explicitly here just in case so VM reconciliation can proceed.
		if !isCustomizationPendingError(err) {
			return err
		}
	}

	return nil
}

func ethCardMatch(newEthCard, curEthCard *vimTypes.VirtualEthernetCard) bool {
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

	// TODO: Compare other attributes, like the card type (e1000 vs vmxnet3).

	return true
}

func UpdateEthCardDeviceChanges(
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

			if !ethCardMatch(expectedNic.GetVirtualEthernetCard(), nic.GetVirtualEthernetCard()) {
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
	var removeDeviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
	for _, dev := range currentEthCards {
		removeDeviceChanges = append(removeDeviceChanges, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

func CreatePCIPassThroughDevice(deviceKey int32, backingInfo vimTypes.BaseVirtualDeviceBackingInfo) vimTypes.BaseVirtualDevice {
	device := &vimTypes.VirtualPCIPassthrough{
		VirtualDevice: vimTypes.VirtualDevice{
			Key:     deviceKey,
			Backing: backingInfo,
		},
	}
	return device
}

func CreatePCIDevices(pciDevices v1alpha1.VirtualDevices) []vimTypes.BaseVirtualDevice {

	var expectedPciDevices []vimTypes.BaseVirtualDevice

	// A negative device range is used for pciDevices here.
	deviceKey := int32(-200)

	for _, vGPU := range pciDevices.VGPUDevices {
		backingInfo := &vimTypes.VirtualPCIPassthroughVmiopBackingInfo{
			Vgpu: vGPU.ProfileName,
		}
		vGPUDevice := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		expectedPciDevices = append(expectedPciDevices, vGPUDevice)
		deviceKey--
	}

	for _, dynamicDirectPath := range pciDevices.DynamicDirectPathIODevices {
		allowedDev := vimTypes.VirtualPCIPassthroughAllowedDevice{
			VendorId: int32(dynamicDirectPath.VendorID),
			DeviceId: int32(dynamicDirectPath.DeviceID),
		}
		backingInfo := &vimTypes.VirtualPCIPassthroughDynamicBackingInfo{
			AllowedDevice: []vimTypes.VirtualPCIPassthroughAllowedDevice{allowedDev},
			CustomLabel:   dynamicDirectPath.CustomLabel,
		}
		dynamicDirectPathDevice := CreatePCIPassThroughDevice(deviceKey, backingInfo)
		expectedPciDevices = append(expectedPciDevices, dynamicDirectPathDevice)
		deviceKey--
	}
	return expectedPciDevices
}

// UpdatePCIDeviceChanges returns devices changes for PCI devices attached to a VM. There are 2 types of PCI devices processed
// here and in case of cloning a VM, devices listed in VMClass are considered as source of truth.
func UpdatePCIDeviceChanges(expectedPciDevices object.VirtualDeviceList,
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
					// b.AllowedDevice has only one element because createPCIDevices() adds only one device based on the
					// devices listed in vmclass.spec.hardware.devices.dynamicDirectPathIODevices.
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
			// There could be multiple vgpus with same backinginfo. Remove current device if matching found.
			currentPciDevices = append(currentPciDevices[:matchingIdx], currentPciDevices[matchingIdx+1:]...)
		}
	}
	// Remove any unmatched existing devices.
	var removeDeviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
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
	vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	minCPUFeq uint64) {

	cpuAllocation := config.CpuAllocation
	var cpuReservation *int64
	var cpuLimit *int64

	if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
		rsv := CpuQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minCPUFeq)
		if cpuAllocation == nil || cpuAllocation.Reservation == nil || *cpuAllocation.Reservation != rsv {
			cpuReservation = &rsv
		}
	}

	if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
		lim := CpuQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minCPUFeq)
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
	vmClassSpec *v1alpha1.VirtualMachineClassSpec) {

	memAllocation := config.MemoryAllocation
	var memoryReservation *int64
	var memoryLimit *int64

	if !vmClassSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := MemoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
		if memAllocation == nil || memAllocation.Reservation == nil || *memAllocation.Reservation != rsv {
			memoryReservation = &rsv
		}
	}

	if !vmClassSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := MemoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
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
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmImage *v1alpha1.VirtualMachineImage,
	vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	vm *v1alpha1.VirtualMachine,
	vmMetadata *vmprovider.VmMetadata,
	globalExtraConfig map[string]string) {

	// The only use of this is for the global JSON_EXTRA_CONFIG to set the image name.
	renderTemplateFn := func(name, text string) string {
		t, err := template.New(name).Parse(text)
		if err != nil {
			return text
		}
		b := strings.Builder{}
		if err := t.Execute(&b, vmImage.Status); err != nil {
			return text
		}
		return b.String()
	}

	extraConfig := make(map[string]string)
	for k, v := range globalExtraConfig {
		extraConfig[k] = renderTemplateFn(k, v)
	}

	if vmMetadata != nil && vmMetadata.Transport == v1alpha1.VirtualMachineMetadataExtraConfigTransport {
		for k, v := range vmMetadata.Data {
			if strings.HasPrefix(k, constants.ExtraConfigGuestInfoPrefix) {
				extraConfig[k] = v
			}
		}
	}

	if lib.IsThunderPciDevicesFSSEnabled() {
		virtualDevices := vmClassSpec.Hardware.Devices
		if len(virtualDevices.VGPUDevices) > 0 || len(virtualDevices.DynamicDirectPathIODevices) > 0 {
			// Add "maintenance.vm.evacuation.poweroff" extraConfig key when GPU devices are present in the VMClass Spec.
			extraConfig[constants.MMPowerOffVMExtraConfigKey] = constants.ExtraConfigTrue
			setMMIOExtraConfig(vm, extraConfig)
		}
	}

	currentExtraConfig := make(map[string]string)
	for _, opt := range config.ExtraConfig {
		if optValue := opt.GetOptionValue(); optValue != nil {
			// BMV: Is this cast to string always safe?
			currentExtraConfig[optValue.Key] = optValue.Value.(string)
		}
	}

	for k, v := range extraConfig {
		// Only add the key/value to the ExtraConfig if the key is not present, to let to the value be
		// changed by the VM. The existing usage of ExtraConfig is hard to fit in the reconciliation model.
		if _, exists := currentExtraConfig[k]; !exists {
			configSpec.ExtraConfig = append(configSpec.ExtraConfig, &vimTypes.OptionValue{Key: k, Value: v})
		}
	}

	if conditions.IsTrue(vmImage, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition) &&
		currentExtraConfig[constants.VMOperatorV1Alpha1ExtraConfigKey] == constants.VMOperatorV1Alpha1ConfigReady {
		// Set VMOperatorV1Alpha1ExtraConfigKey for v1alpha1 VirtualMachineImage compatibility.
		configSpec.ExtraConfig = append(configSpec.ExtraConfig,
			&vimTypes.OptionValue{Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigEnabled})
	}
}

func setMMIOExtraConfig(vm *v1alpha1.VirtualMachine, extraConfig map[string]string) {
	mmioSize := vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation]
	if mmioSize == "" {
		mmioSize = constants.PCIPassthruMMIOSizeDefault
	}
	if mmioSize != "0" {
		extraConfig[constants.PCIPassthruMMIOExtraConfigKey] = constants.ExtraConfigTrue
		extraConfig[constants.PCIPassthruMMIOSizeExtraConfigKey] = mmioSize
	}
}

func UpdateConfigSpecVAppConfig(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmMetadata *vmprovider.VmMetadata) {

	if config.VAppConfig == nil || vmMetadata == nil || vmMetadata.Transport != v1alpha1.VirtualMachineMetadataOvfEnvTransport {
		return
	}

	vAppConfigInfo := config.VAppConfig.GetVmConfigInfo()
	if vAppConfigInfo == nil {
		return
	}

	vmConfigSpec := GetMergedvAppConfigSpec(vmMetadata.Data, vAppConfigInfo.Property)
	if vmConfigSpec != nil {
		configSpec.VAppConfig = vmConfigSpec
	}
}

func UpdateConfigSpecChangeBlockTracking(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmSpec v1alpha1.VirtualMachineSpec) {

	if vmSpec.AdvancedOptions == nil || vmSpec.AdvancedOptions.ChangeBlockTracking == nil {
		// Treat this as we preserve whatever the current CBT status is. I think we'd need
		// to store somewhere what the original state was anyways.
		return
	}

	if !apiEquality.Semantic.DeepEqual(config.ChangeTrackingEnabled, vmSpec.AdvancedOptions.ChangeBlockTracking) {
		configSpec.ChangeTrackingEnabled = vmSpec.AdvancedOptions.ChangeBlockTracking
	}
}

func UpdateHardwareConfigSpec(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmClassSpec *v1alpha1.VirtualMachineClassSpec) {

	// TODO: Looks like a different default annotation gets set by VC.
	if config.Annotation != constants.VCVMAnnotation {
		configSpec.Annotation = constants.VCVMAnnotation
	}
	if nCPUs := int32(vmClassSpec.Hardware.Cpus); config.Hardware.NumCPU != nCPUs {
		configSpec.NumCPUs = nCPUs
	}
	if memMB := MemoryQuantityToMb(vmClassSpec.Hardware.Memory); int64(config.Hardware.MemoryMB) != memMB {
		configSpec.MemoryMB = memMB
	}
	if config.ManagedBy == nil {
		configSpec.ManagedBy = &vimTypes.ManagedByInfo{
			ExtensionKey: "com.vmware.vcenter.wcp",
			Type:         "VirtualMachine",
		}
	}
}

func UpdateConfigSpecFirmware(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vm *v1alpha1.VirtualMachine) {

	if val, ok := vm.Annotations[constants.FirmwareOverrideAnnotation]; ok {
		if (val == "efi" || val == "bios") && config.Firmware != val {
			configSpec.Firmware = val
		}
	}
}

// TODO: Fix parameter explosion.
func updateConfigSpec(
	vmCtx context.VMContext,
	config *vimTypes.VirtualMachineConfigInfo,
	vmImage *v1alpha1.VirtualMachineImage,
	vmClassSpec v1alpha1.VirtualMachineClassSpec,
	vmMetadata *vmprovider.VmMetadata,
	globalExtraConfig map[string]string,
	minCPUFreq uint64) *vimTypes.VirtualMachineConfigSpec {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}

	UpdateHardwareConfigSpec(config, configSpec, &vmClassSpec)
	UpdateConfigSpecCPUAllocation(config, configSpec, &vmClassSpec, minCPUFreq)
	UpdateConfigSpecMemoryAllocation(config, configSpec, &vmClassSpec)
	UpdateConfigSpecExtraConfig(config, configSpec, vmImage, &vmClassSpec, vmCtx.VM, vmMetadata, globalExtraConfig)
	UpdateConfigSpecVAppConfig(config, configSpec, vmMetadata)
	UpdateConfigSpecChangeBlockTracking(config, configSpec, vmCtx.VM.Spec)
	UpdateConfigSpecFirmware(config, configSpec, vmCtx.VM)

	return configSpec
}

func (s *Session) prePowerOnVMConfigSpec(
	vmCtx context.VMContext,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VmUpdateArgs) (*vimTypes.VirtualMachineConfigSpec, error) {

	configSpec := updateConfigSpec(
		vmCtx,
		config,
		updateArgs.VmImage,
		updateArgs.VmClass.Spec,
		updateArgs.VmMetadata,
		s.extraConfig,
		s.GetCpuMinMHzInCluster(),
	)

	virtualDevices := object.VirtualDeviceList(config.Hardware.Device)
	currentDisks := virtualDevices.SelectByType((*vimTypes.VirtualDisk)(nil))
	currentEthCards := virtualDevices.SelectByType((*vimTypes.VirtualEthernetCard)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx, currentDisks)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, diskDeviceChanges...)

	expectedEthCards := updateArgs.NetIfList.GetVirtualDeviceList()
	ethCardDeviceChanges, err := UpdateEthCardDeviceChanges(expectedEthCards, currentEthCards)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, ethCardDeviceChanges...)

	// With FSS_THUNDERPCIDEVICES = true, we allow a VM to get attached to PCI devices.
	if lib.IsThunderPciDevicesFSSEnabled() {
		currentPciDevices := virtualDevices.SelectByType((*vimTypes.VirtualPCIPassthrough)(nil))
		expectedPciDevices := CreatePCIDevices(updateArgs.VmClass.Spec.Hardware.Devices)
		pciDeviceChanges, err := UpdatePCIDeviceChanges(expectedPciDevices, currentPciDevices)
		if err != nil {
			return nil, err
		}
		configSpec.DeviceChange = append(configSpec.DeviceChange, pciDeviceChanges...)
	}

	return configSpec, nil
}

func (s *Session) prePowerOnVMReconfigure(
	vmCtx context.VMContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VmUpdateArgs) error {

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

func (s *Session) ensureNetworkInterfaces(vmCtx context.VMContext) (network.NetworkInterfaceInfoList, error) {
	// This negative device key is the traditional range used for network interfaces.
	deviceKey := int32(-100)

	var netIfList = make(network.NetworkInterfaceInfoList, len(vmCtx.VM.Spec.NetworkInterfaces))
	for i := range vmCtx.VM.Spec.NetworkInterfaces {
		vif := vmCtx.VM.Spec.NetworkInterfaces[i]

		info, err := s.networkProvider.EnsureNetworkInterface(vmCtx, &vif)
		if err != nil {
			return nil, err
		}

		// govmomi assigns a random device key. Fix that up here.
		info.Device.GetVirtualDevice().Key = deviceKey
		netIfList[i] = *info

		deviceKey--
	}

	return netIfList, nil
}

func (s *Session) fakeUpClonedNetIfList(
	_ context.VMContext,
	config *vimTypes.VirtualMachineConfigInfo) network.NetworkInterfaceInfoList {

	var netIfList []network.NetworkInterfaceInfo
	currentEthCards := object.VirtualDeviceList(config.Hardware.Device).SelectByType((*vimTypes.VirtualEthernetCard)(nil))

	for _, dev := range currentEthCards {
		card, ok := dev.(vimTypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		netIfList = append(netIfList, network.NetworkInterfaceInfo{
			Device: dev,
			Customization: &vimTypes.CustomizationAdapterMapping{
				MacAddress: card.GetVirtualEthernetCard().MacAddress,
				Adapter: vimTypes.CustomizationIPSettings{
					Ip: &vimTypes.CustomizationDhcpIpGenerator{},
				},
			},
		})
	}

	return netIfList
}

// TemplateData is used to specify templating values
// for guest customization data. Users will be able
// to specify fields from this struct as values
// for customization. E.g.: {{ (index .NetworkInterfaces 0).Gateway }}.
type TemplateData struct {
	NetworkInterfaces []network.IPConfig
	NameServers       []string
}

func UpdateVmConfigArgsTemplates(vmCtx context.VMContext, updateArgs VmUpdateArgs) {
	templateData := TemplateData{}
	templateData.NetworkInterfaces = updateArgs.NetIfList.GetIPConfigs()
	templateData.NameServers = updateArgs.DNSServers

	renderTemplate := func(name, templateStr string) string {
		templ, err := template.New(name).Parse(templateStr)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to parse template", "templateStr", templateStr)
			// TODO: emit related events
			return templateStr
		}
		var doc bytes.Buffer
		err = templ.Execute(&doc, &templateData)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to execute template", "templateStr", templateStr)
			// TODO: emit related events
			return templateStr
		}
		return doc.String()
	}

	if updateArgs.VmMetadata != nil {
		data := updateArgs.VmMetadata.Data
		for key, val := range data {
			data[key] = renderTemplate(key, val)
		}
	}
}

func (s *Session) ensureCNSVolumes(vmCtx context.VMContext) error {
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

type VmUpdateArgs struct {
	vmprovider.VmConfigArgs
	NetIfList  network.NetworkInterfaceInfoList
	DNSServers []string
}

func (s *Session) prepareVMForPowerOn(
	vmCtx context.VMContext,
	resVM *res.VirtualMachine,
	cfg *vimTypes.VirtualMachineConfigInfo,
	vmConfigArgs vmprovider.VmConfigArgs) error {

	netIfList, err := s.ensureNetworkInterfaces(vmCtx)
	if err != nil {
		return err
	}

	if len(netIfList) == 0 {
		// Assume this is the special condition in cloneVMNicDeviceChanges(), instead of actually
		// wanting to remove all the interfaces. Create fake list that matches the current interfaces.
		// The special clone condition is a hack that we should address later.
		netIfList = s.fakeUpClonedNetIfList(vmCtx, cfg)
	}

	dnsServers, err := config.GetNameserversFromConfigMap(s.k8sClient)
	if err != nil {
		vmCtx.Logger.Error(err, "Unable to get DNS server list from ConfigMap")
		// Prior code only logged?!?
	}

	updateArgs := VmUpdateArgs{
		VmConfigArgs: vmConfigArgs,
		NetIfList:    netIfList,
		DNSServers:   dnsServers,
	}

	if lib.IsVMServiceV1Alpha2FSSEnabled() {
		// For templating errors, only logged the error instead of failing completely.
		// Maybe emit a warning event to VM?
		UpdateVmConfigArgsTemplates(vmCtx, updateArgs)
	}

	err = s.prePowerOnVMReconfigure(vmCtx, resVM, cfg, updateArgs)
	if err != nil {
		return err
	}

	err = s.customizeVM(vmCtx, resVM, cfg, updateArgs)
	if err != nil {
		return err
	}

	err = s.ensureCNSVolumes(vmCtx)
	if err != nil {
		return err
	}

	return nil
}

func (s *Session) poweredOnVMReconfigure(
	vmCtx context.VMContext,
	resVM *res.VirtualMachine,
	config *vimTypes.VirtualMachineConfigInfo) error {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	UpdateConfigSpecChangeBlockTracking(config, configSpec, vmCtx.VM.Spec)

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

func (s *Session) attachTagsAndModules(
	vmCtx context.VMContext,
	resVM *res.VirtualMachine,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	clusterModuleName := vmCtx.VM.Annotations[pkg.ClusterModuleNameKey]
	providerTagsName := vmCtx.VM.Annotations[pkg.ProviderTagsAnnotationKey]

	// Both the clusterModule and tag are required be able to enforce the vm-vm anti-affinity policy.
	if clusterModuleName == "" || providerTagsName == "" {
		return nil
	}

	// Find ClusterModule UUID from the ResourcePolicy.
	var moduleUuid string
	for _, clusterModule := range resourcePolicy.Status.ClusterModules {
		// TODO Needs to compare cluster
		if clusterModule.GroupName == clusterModuleName {
			moduleUuid = clusterModule.ModuleUuid
			break
		}
	}
	if moduleUuid == "" {
		return fmt.Errorf("ClusterModule %s not found", clusterModuleName)
	}

	vmRef := resVM.MoRef()

	err := s.Client.ClusterModuleClient().AddMoRefToModule(vmCtx, moduleUuid, vmRef)
	if err != nil {
		return err
	}

	// Lookup the real tag name from config and attach to the VM.
	tagName := s.tagInfo[providerTagsName]
	if tagName == "" {
		return fmt.Errorf("empty tagName, TagInfo %s not found", providerTagsName)
	}
	tagCategoryName := s.tagInfo[config.ProviderTagCategoryNameKey]
	if err := s.AttachTagToVm(vmCtx, tagName, tagCategoryName, vmRef); err != nil {
		return err
	}

	return nil
}

func ipCIDRNotation(ipAddress string, prefix int32) string {
	return ipAddress + "/" + strconv.Itoa(int(prefix))
}

func NicInfoToNetworkIfStatus(nicInfo vimTypes.GuestNicInfo) v1alpha1.NetworkInterfaceStatus {
	var IpAddresses []string
	for _, ipAddress := range nicInfo.IpConfig.IpAddress {
		IpAddresses = append(IpAddresses, ipCIDRNotation(ipAddress.IpAddress, ipAddress.PrefixLength))
	}

	return v1alpha1.NetworkInterfaceStatus{
		Connected:   nicInfo.Connected,
		MacAddress:  nicInfo.MacAddress,
		IpAddresses: IpAddresses,
	}
}

func MarkCustomizationInfoCondition(vm *v1alpha1.VirtualMachine, guestInfo *vimTypes.GuestInfo) {
	if guestInfo == nil || guestInfo.CustomizationInfo == nil {
		conditions.MarkUnknown(vm, v1alpha1.GuestCustomizationCondition, "", "")
		return
	}

	switch guestInfo.CustomizationInfo.CustomizationStatus {
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_IDLE), "":
		conditions.MarkTrue(vm, v1alpha1.GuestCustomizationCondition)
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_PENDING):
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, v1alpha1.GuestCustomizationPendingReason, v1alpha1.ConditionSeverityInfo, "")
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_RUNNING):
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, v1alpha1.GuestCustomizationRunningReason, v1alpha1.ConditionSeverityInfo, "")
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_SUCCEEDED):
		conditions.MarkTrue(vm, v1alpha1.GuestCustomizationCondition)
	case string(vimTypes.GuestInfoCustomizationStatusTOOLSDEPLOYPKG_FAILED):
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "vSphere VM Customization failed due to an unknown error."
		}
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, v1alpha1.GuestCustomizationFailedReason, v1alpha1.ConditionSeverityError, errorMsg)
	default:
		errorMsg := guestInfo.CustomizationInfo.ErrorMsg
		if errorMsg == "" {
			errorMsg = "Unexpected VM Customization status"
		}
		conditions.MarkFalse(vm, v1alpha1.GuestCustomizationCondition, "", v1alpha1.ConditionSeverityError, errorMsg)
	}
}

func MarkVMToolsRunningStatusCondition(vm *v1alpha1.VirtualMachine, guestInfo *vimTypes.GuestInfo) {
	if guestInfo == nil || guestInfo.ToolsRunningStatus == "" {
		conditions.MarkUnknown(vm, v1alpha1.VirtualMachineToolsCondition, "", "")
		return
	}

	switch guestInfo.ToolsRunningStatus {
	case string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsNotRunning):
		msg := "VMware Tools is not running"
		conditions.MarkFalse(vm, v1alpha1.VirtualMachineToolsCondition, v1alpha1.VirtualMachineToolsNotRunningReason,
			v1alpha1.ConditionSeverityError, msg)
	case string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsRunning), string(vimTypes.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts):
		conditions.MarkTrue(vm, v1alpha1.VirtualMachineToolsCondition)
	default:
		msg := "Unexpected VMware Tools running status"
		conditions.MarkUnknown(vm, v1alpha1.VirtualMachineToolsCondition, "", msg)
	}
}

func (s *Session) updateVMStatus(
	vmCtx context.VMContext,
	resVM *res.VirtualMachine) error {

	// TODO: We could be smarter about not re-fetching the config: if we didn't do a
	// reconfigure or power change, the prior config is still entirely valid.
	moVM, err := resVM.GetProperties(vmCtx, []string{"config.changeTrackingEnabled", "guest", "summary"})
	if err != nil {
		// Leave the current Status unchanged.
		return err
	}

	var errs []error
	vm := vmCtx.VM
	summary := moVM.Summary

	vm.Status.Phase = v1alpha1.Created
	vm.Status.PowerState = v1alpha1.VirtualMachinePowerState(summary.Runtime.PowerState)
	vm.Status.UniqueID = resVM.MoRef().Value
	vm.Status.BiosUUID = summary.Config.Uuid
	vm.Status.InstanceUUID = summary.Config.InstanceUuid

	if host := summary.Runtime.Host; host != nil {
		hostSystem := object.NewHostSystem(s.Client.VimClient(), *host)
		if hostName, err := hostSystem.ObjectName(vmCtx); err != nil {
			// Leave existing vm.Status.Host value.
			errs = append(errs, err)
		} else {
			vm.Status.Host = hostName
		}
	} else {
		vm.Status.Host = ""
	}

	guestInfo := moVM.Guest

	if guestInfo != nil {
		vm.Status.VmIp = guestInfo.IpAddress
		var networkIfStatuses []v1alpha1.NetworkInterfaceStatus
		for _, nicInfo := range guestInfo.Net {
			networkIfStatuses = append(networkIfStatuses, NicInfoToNetworkIfStatus(nicInfo))
		}
		vm.Status.NetworkInterfaces = networkIfStatuses
	} else {
		vm.Status.VmIp = ""
		vm.Status.NetworkInterfaces = nil
	}

	MarkCustomizationInfoCondition(vm, guestInfo)
	MarkVMToolsRunningStatusCondition(vm, guestInfo)

	if config := moVM.Config; config != nil {
		vm.Status.ChangeBlockTracking = config.ChangeTrackingEnabled
	} else {
		vm.Status.ChangeBlockTracking = nil
	}

	return k8serrors.NewAggregate(errs)
}

func (s *Session) UpdateVirtualMachine(
	vmCtx context.VMContext,
	vmConfigArgs vmprovider.VmConfigArgs) error {

	resVM, err := s.GetVirtualMachine(vmCtx)
	if err != nil {
		return err
	}

	moVM, err := resVM.GetProperties(vmCtx, []string{"config", "runtime"})
	if err != nil {
		return err
	}

	isOff := moVM.Runtime.PowerState == vimTypes.VirtualMachinePowerStatePoweredOff

	// Update VMStatus with BiosUUID to unblock volume controller
	vmCtx.VM.Status.BiosUUID = moVM.Config.Uuid

	switch vmCtx.VM.Spec.PowerState {
	case v1alpha1.VirtualMachinePoweredOff:
		if !isOff {
			err := resVM.SetPowerState(vmCtx, v1alpha1.VirtualMachinePoweredOff)
			if err != nil {
				return err
			}
		}

		// BMV: We'll likely want to reconfigure a powered off VM too, but right now
		// we'll defer that until the pre power on (and until more people complain
		// that the UI appears wrong).

	case v1alpha1.VirtualMachinePoweredOn:
		config := moVM.Config

		// See govmomi VirtualMachine::Device() explanation for this check.
		if config == nil {
			return fmt.Errorf("VM config is not available, connectionState=%s", moVM.Runtime.ConnectionState)
		}

		if isOff {
			err := s.prepareVMForPowerOn(vmCtx, resVM, config, vmConfigArgs)
			if err != nil {
				return err
			}

			err = resVM.SetPowerState(vmCtx, v1alpha1.VirtualMachinePoweredOn)
			if err != nil {
				return err
			}
		} else {
			err := s.poweredOnVMReconfigure(vmCtx, resVM, config)
			if err != nil {
				return err
			}
		}
	}

	if err := s.updateVMStatus(vmCtx, resVM); err != nil {
		return err
	}

	// TODO: Find a better place for this?
	if err := s.attachTagsAndModules(vmCtx, resVM, vmConfigArgs.ResourcePolicy); err != nil {
		return err
	}

	return nil
}
