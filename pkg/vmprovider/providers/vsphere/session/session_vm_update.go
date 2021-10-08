// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"
	"reflect"
	"strings"
	"text/template"

	vimTypes "github.com/vmware/govmomi/vim25/types"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/vmware/govmomi/object"

	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/instancevm"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/config"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

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
	expectedPciDevices := make([]vimTypes.BaseVirtualDevice, 0, len(pciDevices.VGPUDevices))

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
	vmClassSpec *v1alpha1.VirtualMachineClassSpec,
	minCPUFeq uint64) {

	cpuAllocation := config.CpuAllocation
	var cpuReservation *int64
	var cpuLimit *int64

	if !vmClassSpec.Policies.Resources.Requests.Cpu.IsZero() {
		rsv := CPUQuantityToMhz(vmClassSpec.Policies.Resources.Requests.Cpu, minCPUFeq)
		if cpuAllocation == nil || cpuAllocation.Reservation == nil || *cpuAllocation.Reservation != rsv {
			cpuReservation = &rsv
		}
	}

	if !vmClassSpec.Policies.Resources.Limits.Cpu.IsZero() {
		lim := CPUQuantityToMhz(vmClassSpec.Policies.Resources.Limits.Cpu, minCPUFeq)
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

	virtualDevices := vmClassSpec.Hardware.Devices
	if len(virtualDevices.VGPUDevices) > 0 || len(virtualDevices.DynamicDirectPathIODevices) > 0 {
		// Add "maintenance.vm.evacuation.poweroff" extraConfig key when GPU devices are present in the VMClass Spec.
		extraConfig[constants.MMPowerOffVMExtraConfigKey] = constants.ExtraConfigTrue
		setMMIOExtraConfig(vm, extraConfig)
	}

	// If VM has InstanceStorage configured, add "maintenance.vm.evacuation.poweroff" to extraConfig
	if instancevm.IsInstanceStorageConfigured(vm) {
		extraConfig[constants.MMPowerOffVMExtraConfigKey] = constants.ExtraConfigTrue
	}

	configSpec.ExtraConfig = MergeExtraConfig(config.ExtraConfig, extraConfig)

	if conditions.IsTrue(vmImage, v1alpha1.VirtualMachineImageV1Alpha1CompatibleCondition) {
		ecMap := ExtraConfigToMap(config.ExtraConfig)
		if ecMap[constants.VMOperatorV1Alpha1ExtraConfigKey] == constants.VMOperatorV1Alpha1ConfigReady {
			// Set VMOperatorV1Alpha1ExtraConfigKey for v1alpha1 VirtualMachineImage compatibility.
			configSpec.ExtraConfig = append(configSpec.ExtraConfig,
				&vimTypes.OptionValue{Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigEnabled})
		}
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
	vmCtx context.VirtualMachineContext,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs,
	globalExtraConfig map[string]string,
	minCPUFreq uint64) *vimTypes.VirtualMachineConfigSpec {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	vmImage := updateArgs.VMImage
	vmClassSpec := updateArgs.VMClass.Spec

	UpdateHardwareConfigSpec(config, configSpec, &vmClassSpec)
	UpdateConfigSpecCPUAllocation(config, configSpec, &vmClassSpec, minCPUFreq)
	UpdateConfigSpecMemoryAllocation(config, configSpec, &vmClassSpec)
	UpdateConfigSpecExtraConfig(config, configSpec, vmImage, &vmClassSpec, vmCtx.VM, globalExtraConfig)
	UpdateConfigSpecChangeBlockTracking(config, configSpec, vmCtx.VM.Spec)
	UpdateConfigSpecFirmware(config, configSpec, vmCtx.VM)

	return configSpec
}

func (s *Session) prePowerOnVMConfigSpec(
	vmCtx context.VirtualMachineContext,
	config *vimTypes.VirtualMachineConfigInfo,
	updateArgs VMUpdateArgs) (*vimTypes.VirtualMachineConfigSpec, error) {

	configSpec := updateConfigSpec(
		vmCtx,
		config,
		updateArgs,
		s.extraConfig,
		s.GetCPUMinMHzInCluster(),
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

	currentPciDevices := virtualDevices.SelectByType((*vimTypes.VirtualPCIPassthrough)(nil))
	expectedPciDevices := CreatePCIDevices(updateArgs.VMClass.Spec.Hardware.Devices)
	pciDeviceChanges, err := UpdatePCIDeviceChanges(expectedPciDevices, currentPciDevices)
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
	updateArgs VMUpdateArgs) error {

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

func (s *Session) ensureNetworkInterfaces(vmCtx context.VirtualMachineContext) (network.InterfaceInfoList, error) {
	// This negative device key is the traditional range used for network interfaces.
	deviceKey := int32(-100)

	var netIfList = make(network.InterfaceInfoList, len(vmCtx.VM.Spec.NetworkInterfaces))
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
	_ context.VirtualMachineContext,
	config *vimTypes.VirtualMachineConfigInfo) network.InterfaceInfoList {

	netIfList := make([]network.InterfaceInfo, 0)
	currentEthCards := object.VirtualDeviceList(config.Hardware.Device).SelectByType((*vimTypes.VirtualEthernetCard)(nil))

	for _, dev := range currentEthCards {
		card, ok := dev.(vimTypes.BaseVirtualEthernetCard)
		if !ok {
			continue
		}

		netIfList = append(netIfList, network.InterfaceInfo{
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

type VMUpdateArgs struct {
	vmprovider.VMConfigArgs
	NetIfList  network.InterfaceInfoList
	DNSServers []string
}

func (s *Session) prepareVMForPowerOn(
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	cfg *vimTypes.VirtualMachineConfigInfo,
	vmConfigArgs vmprovider.VMConfigArgs) error {

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

	updateArgs := VMUpdateArgs{
		VMConfigArgs: vmConfigArgs,
		NetIfList:    netIfList,
		DNSServers:   dnsServers,
	}

	err = s.prePowerOnVMReconfigure(vmCtx, resVM, cfg, updateArgs)
	if err != nil {
		return err
	}

	err = s.customize(vmCtx, resVM, cfg, updateArgs)
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
	vmCtx context.VirtualMachineContext,
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
	vmCtx context.VirtualMachineContext,
	resVM *res.VirtualMachine,
	resourcePolicy *v1alpha1.VirtualMachineSetResourcePolicy) error {

	clusterModuleName := vmCtx.VM.Annotations[pkg.ClusterModuleNameKey]
	providerTagsName := vmCtx.VM.Annotations[pkg.ProviderTagsAnnotationKey]

	// Both the clusterModule and tag are required be able to enforce the vm-vm anti-affinity policy.
	if clusterModuleName == "" || providerTagsName == "" {
		return nil
	}

	// Find ClusterModule UUID from the ResourcePolicy.
	_, moduleUUID := clustermodules.FindClusterModuleUUID(clusterModuleName, s.Cluster().Reference(), resourcePolicy)
	if moduleUUID == "" {
		return fmt.Errorf("ClusterModule %s not found", clusterModuleName)
	}

	vmRef := resVM.MoRef()

	err := s.Client.ClusterModuleClient().AddMoRefToModule(vmCtx, moduleUUID, vmRef)
	if err != nil {
		return err
	}

	// Lookup the real tag name from config and attach to the VM.
	tagName := s.tagInfo[providerTagsName]
	if tagName == "" {
		return fmt.Errorf("empty tagName, TagInfo %s not found", providerTagsName)
	}
	tagCategoryName := s.tagInfo[config.ProviderTagCategoryNameKey]
	return s.AttachTagToVM(vmCtx, tagName, tagCategoryName, vmRef)
}

func (s *Session) UpdateVirtualMachine(
	vmCtx context.VirtualMachineContext,
	vmConfigArgs vmprovider.VMConfigArgs) (err error) {

	resVM, err := s.GetVirtualMachine(vmCtx)
	if err != nil {
		return err
	}

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

	isOff := moVM.Runtime.PowerState == vimTypes.VirtualMachinePowerStatePoweredOff

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

	// TODO: Find a better place for this?
	return s.attachTagsAndModules(vmCtx, resVM, vmConfigArgs.ResourcePolicy)
}
