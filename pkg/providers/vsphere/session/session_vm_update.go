// Copyright (c) 2018-2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha3/common"
	"github.com/vmware-tanzu/vm-operator/pkg"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	network2 "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/annotations"
	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
)

// VMUpdateArgs contains the arguments needed to update a VM on VC.
type VMUpdateArgs struct {
	VMClass        vmopv1.VirtualMachineClass
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	MinCPUFreq     uint64
	ExtraConfig    map[string]string
	BootstrapData  vmlifecycle.BootstrapData
	ConfigSpec     vimtypes.VirtualMachineConfigSpec
	NetworkResults network2.NetworkInterfaceResults
}

// VMResizeArgs contains the arguments needed to resize a VM on VC.
type VMResizeArgs struct {
	VMClass *vmopv1.VirtualMachineClass
	// ConfigSpec derived from the class and VM Spec.
	ConfigSpec vimtypes.VirtualMachineConfigSpec
}

func ethCardMatch(newBaseEthCard, curBaseEthCard vimtypes.BaseVirtualEthernetCard) bool {
	if reflect.TypeOf(curBaseEthCard) != reflect.TypeOf(newBaseEthCard) {
		return false
	}

	curEthCard := curBaseEthCard.GetVirtualEthernetCard()
	newEthCard := newBaseEthCard.GetVirtualEthernetCard()
	if newEthCard.AddressType == string(vimtypes.VirtualEthernetCardMacTypeManual) {
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
	ctx context.Context,
	expectedEthCards object.VirtualDeviceList,
	currentEthCards object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, expectedDev := range expectedEthCards {
		expectedNic := expectedDev.(vimtypes.BaseVirtualEthernetCard)
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
			nic := curDev.(vimtypes.BaseVirtualEthernetCard)

			// This assumes we don't have multiple NICs in the same backing network. This is kind of, sort
			// of enforced by the webhook, but we lack a guaranteed way to match up the NICs.

			if !ethCardMatch(expectedNic, nic) {
				continue
			}

			db := nic.GetVirtualEthernetCard().Backing
			if db == nil || reflect.TypeOf(db) != expectedBackingType {
				continue
			}

			var backingMatch bool

			// Cribbed from VirtualDeviceList.SelectByBackingInfo().
			switch a := db.(type) {
			case *vimtypes.VirtualEthernetCardNetworkBackingInfo:
				// This backing is only used in testing.
				b := expectedBacking.(*vimtypes.VirtualEthernetCardNetworkBackingInfo)
				backingMatch = a.DeviceName == b.DeviceName
			case *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo:
				b := expectedBacking.(*vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				backingMatch = a.Port.SwitchUuid == b.Port.SwitchUuid && a.Port.PortgroupKey == b.Port.PortgroupKey
			case *vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo:
				b := expectedBacking.(*vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				backingMatch = a.OpaqueNetworkId == b.OpaqueNetworkId
			}

			if backingMatch {
				matchingIdx = idx
				break
			}
		}

		if matchingIdx == -1 {
			// No matching backing found so add new card.
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Device:    expectedDev,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			})
		} else {
			// Matching backing found so keep this card (don't remove it below after this loop).
			currentEthCards = append(currentEthCards[:matchingIdx], currentEthCards[matchingIdx+1:]...)
		}
	}

	// Remove any unmatched existing interfaces.
	removeDeviceChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(currentEthCards))
	for _, dev := range currentEthCards {
		removeDeviceChanges = append(removeDeviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

// UpdatePCIDeviceChanges returns devices changes for PCI devices attached to a VM. There are 2 types of PCI devices
// processed here and in case of cloning a VM, devices listed in VMClass are considered as source of truth.
func UpdatePCIDeviceChanges(
	expectedPciDevices object.VirtualDeviceList,
	currentPciDevices object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec
	for _, expectedDev := range expectedPciDevices {
		expectedPci := expectedDev.(*vimtypes.VirtualPCIPassthrough)
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
			case *vimtypes.VirtualPCIPassthroughVmiopBackingInfo:
				b := expectedBacking.(*vimtypes.VirtualPCIPassthroughVmiopBackingInfo)
				backingMatch = a.Vgpu == b.Vgpu

			case *vimtypes.VirtualPCIPassthroughDynamicBackingInfo:
				currAllowedDevs := a.AllowedDevice
				b := expectedBacking.(*vimtypes.VirtualPCIPassthroughDynamicBackingInfo)
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
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
				Device:    expectedPci,
			})
		} else {
			// There could be multiple vGPUs with same BackingInfo. Remove current device if matching found.
			currentPciDevices = append(currentPciDevices[:matchingIdx], currentPciDevices[matchingIdx+1:]...)
		}
	}
	// Remove any unmatched existing devices.
	removeDeviceChanges := make([]vimtypes.BaseVirtualDeviceConfigSpec, 0, len(currentPciDevices))
	for _, dev := range currentPciDevices {
		removeDeviceChanges = append(removeDeviceChanges, &vimtypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimtypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

// UpdateConfigSpecExtraConfig updates the ExtraConfig of the given ConfigSpec.
// At a minimum, config and configSpec must be non-nil, in which case it will
// just ensure MMPowerOffVMExtraConfigKey is no longer part of ExtraConfig.
func UpdateConfigSpecExtraConfig(
	ctx context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec, classConfigSpec *vimtypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec,
	vm *vmopv1.VirtualMachine,
	globalExtraConfig map[string]string) {

	// Either initialize extraConfig to an empty map or from a copy of
	// globalExtraConfig.
	var extraConfig map[string]string
	if globalExtraConfig == nil {
		extraConfig = map[string]string{}
	} else {
		extraConfig = maps.Clone(globalExtraConfig)
	}

	// Ensure a VM with vGPUs or dynamic direct path I/O devices has the
	// correct flags in ExtraConfig for memory mapped I/O. Please see
	// https://kb.vmware.com/s/article/2142307 for more information about these
	// flags.
	if hasvGPUOrDDPIODevices(config, classConfigSpec, vmClassSpec) {
		setMemoryMappedIOFlagsInExtraConfig(vm, extraConfig)
	}

	if classConfigSpec != nil {
		// Merge non-intersecting keys from the desired config spec extra config
		// with the class config spec extra config (ie) class config spec extra
		// config keys takes precedence over the desired config spec extra
		// config keys.
		extraConfig = pkgutil.OptionValues(classConfigSpec.ExtraConfig).
			Append(pkgutil.OptionValuesFromMap(extraConfig)...).
			StringMap()
	}

	// Note if the VM uses both LinuxPrep and vAppConfig. This is used in the
	// loop below.
	linuxPrepAndVAppConfig := isLinuxPrepAndVAppConfig(vm)

	for i := range config.ExtraConfig {
		if o := config.ExtraConfig[i].GetOptionValue(); o != nil {

			switch o.Key {

			// Ensure MMPowerOffVMExtraConfigKey is no longer part of ExtraConfig as
			// setting it to an empty value removes it.
			case constants.MMPowerOffVMExtraConfigKey:
				if o.Value != "" {
					extraConfig[o.Key] = ""
				}

			// For the special V1Alpha1Compatible images, set the
			// VMOperatorV1Alpha1ExtraConfigKey to "Ready" to fix configuration
			// races between cloud-init, vApp, and GOSC. This is addressed by
			// by deferring cloud-init to run on second boot and preventing
			// cloud-init from configuring the network. This only matters for
			// two, legacy marketplace images. The check below is what the v1a1
			// OvfEnv transport converts to in v1a2 bootstrap. The v1a1
			// ExtraConfig transport is deprecated.
			case constants.VMOperatorV1Alpha1ExtraConfigKey:
				if linuxPrepAndVAppConfig {
					if o.Value == constants.VMOperatorV1Alpha1ConfigReady {
						extraConfig[o.Key] = constants.VMOperatorV1Alpha1ConfigEnabled
					}
				}
			}
		}
	}

	// Update the ConfigSpec's ExtraConfig property with the results from
	// above. Please note this *may* include keys with empty values. This
	// indicates to vSphere that a key/value pair should be removed.
	configSpec.ExtraConfig = pkgutil.OptionValues(config.ExtraConfig).
		Diff(pkgutil.OptionValuesFromMap(extraConfig)...)
}

func isLinuxPrepAndVAppConfig(vm *vmopv1.VirtualMachine) bool {
	if vm == nil {
		return false
	}
	if vm.Spec.Bootstrap == nil {
		return false
	}
	return vm.Spec.Bootstrap.LinuxPrep != nil && vm.Spec.Bootstrap.VAppConfig != nil
}

func hasvGPUOrDDPIODevices(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec) bool {

	return hasvGPUOrDDPIODevicesInVM(config) ||
		hasvGPUOrDDPIODevicesInVMClass(configSpec, vmClassSpec)
}

func hasvGPUOrDDPIODevicesInVM(config *vimtypes.VirtualMachineConfigInfo) bool {
	if config == nil {
		return false
	}
	if len(pkgutil.SelectNvidiaVgpu(config.Hardware.Device)) > 0 {
		return true
	}
	if len(pkgutil.SelectDynamicDirectPathIO(config.Hardware.Device)) > 0 {
		return true
	}
	return false
}

func hasvGPUOrDDPIODevicesInVMClass(
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec) bool {

	if vmClassSpec != nil {
		if len(vmClassSpec.Hardware.Devices.VGPUDevices) > 0 {
			return true
		}
		if len(vmClassSpec.Hardware.Devices.DynamicDirectPathIODevices) > 0 {
			return true
		}
	}

	if configSpec != nil {
		return pkgutil.HasVirtualPCIPassthroughDeviceChange(configSpec.DeviceChange)
	}

	return false
}

// setMemoryMappedIOFlagsInExtraConfig sets flags in ExtraConfig that can
// improve the performance of VMs with vGPUs and dynamic direct path I/O
// devices. Please see https://kb.vmware.com/s/article/2142307 for more
// information about these flags.
func setMemoryMappedIOFlagsInExtraConfig(
	vm *vmopv1.VirtualMachine, extraConfig map[string]string) {
	if vm == nil {
		return
	}

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
	ctx context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec, classConfigSpec *vimtypes.VirtualMachineConfigSpec,
	vmSpec vmopv1.VirtualMachineSpec) {

	if adv := vmSpec.Advanced; adv != nil && adv.ChangeBlockTracking != nil {
		if !apiEquality.Semantic.DeepEqual(config.ChangeTrackingEnabled, adv.ChangeBlockTracking) {
			configSpec.ChangeTrackingEnabled = adv.ChangeBlockTracking
		}
		return
	}

	if classConfigSpec != nil && classConfigSpec.ChangeTrackingEnabled != nil {
		if !apiEquality.Semantic.DeepEqual(config.ChangeTrackingEnabled, classConfigSpec.ChangeTrackingEnabled) {
			configSpec.ChangeTrackingEnabled = classConfigSpec.ChangeTrackingEnabled
		}
	}
}

func UpdateHardwareConfigSpec(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec) {

	if nCPUs := int32(vmClassSpec.Hardware.Cpus); config.Hardware.NumCPU != nCPUs {
		configSpec.NumCPUs = nCPUs
	}
	if memMB := virtualmachine.MemoryQuantityToMb(vmClassSpec.Hardware.Memory); int64(config.Hardware.MemoryMB) != memMB {
		configSpec.MemoryMB = memMB
	}
}

func UpdateConfigSpecAnnotation(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) {
	if config.Annotation == "" {
		configSpec.Annotation = constants.VCVMAnnotation
	}
}

// UpdateConfigSpecGuestID sets the given vmSpecGuestID in the ConfigSpec if it
// is not empty and different from the current GuestID in the VM's ConfigInfo.
func UpdateConfigSpecGuestID(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vmSpecGuestID string) {
	if vmSpecGuestID != "" && config.GuestId != vmSpecGuestID {
		configSpec.GuestId = vmSpecGuestID
	}
}

func UpdateConfigSpecFirmware(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vm *vmopv1.VirtualMachine) {

	if val, ok := vm.Annotations[constants.FirmwareOverrideAnnotation]; ok {
		if (val == "efi" || val == "bios") && config.Firmware != val {
			configSpec.Firmware = val
		}
	}
}

// updateConfigSpec overlays the VM Class spec with the provided ConfigSpec to
// form a desired ConfigSpec that will be used to reconfigure the VM.
func updateConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimtypes.VirtualMachineConfigSpec, error) {

	configSpec := &vimtypes.VirtualMachineConfigSpec{}
	vmClassSpec := updateArgs.VMClass.Spec

	if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		*config,
		configSpec); err != nil {

		return nil, err
	}

	UpdateConfigSpecExtraConfig(
		vmCtx, config, configSpec, &updateArgs.ConfigSpec,
		&vmClassSpec, vmCtx.VM, updateArgs.ExtraConfig)
	UpdateConfigSpecAnnotation(config, configSpec)
	UpdateConfigSpecChangeBlockTracking(
		vmCtx, config, configSpec, &updateArgs.ConfigSpec, vmCtx.VM.Spec)
	UpdateConfigSpecFirmware(config, configSpec, vmCtx.VM)
	UpdateConfigSpecGuestID(config, configSpec, vmCtx.VM.Spec.GuestID)

	if pkgcfg.FromContext(vmCtx).Features.VMResizeCPUMemory {
		UpdateHardwareConfigSpec(config, configSpec, &vmClassSpec)
		resize.CompareCPUAllocation(*config, updateArgs.ConfigSpec, configSpec)
		resize.CompareMemoryAllocation(*config, updateArgs.ConfigSpec, configSpec)
	}

	return configSpec, nil
}

func (s *Session) prePowerOnVMConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimtypes.VirtualMachineConfigSpec, error) {

	configSpec, err := updateConfigSpec(vmCtx, config, updateArgs)
	if err != nil {
		return nil, err
	}

	virtualDevices := object.VirtualDeviceList(config.Hardware.Device)
	currentDisks := virtualDevices.SelectByType((*vimtypes.VirtualDisk)(nil))
	currentEthCards := virtualDevices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))
	currentPciDevices := virtualDevices.SelectByType((*vimtypes.VirtualPCIPassthrough)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx, currentDisks)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, diskDeviceChanges...)

	var expectedEthCards object.VirtualDeviceList
	for idx := range updateArgs.NetworkResults.Results {
		expectedEthCards = append(expectedEthCards, updateArgs.NetworkResults.Results[idx].Device)
	}

	ethCardDeviceChanges, err := UpdateEthCardDeviceChanges(vmCtx, expectedEthCards, currentEthCards)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, ethCardDeviceChanges...)

	var expectedPCIDevices []vimtypes.BaseVirtualDevice
	if configSpecDevs := pkgutil.DevicesFromConfigSpec(&updateArgs.ConfigSpec); len(configSpecDevs) > 0 {
		pciPassthruFromConfigSpec := pkgutil.SelectVirtualPCIPassthrough(configSpecDevs)
		expectedPCIDevices = virtualmachine.CreatePCIDevicesFromConfigSpec(pciPassthruFromConfigSpec)
	}

	pciDeviceChanges, err := UpdatePCIDeviceChanges(expectedPCIDevices, currentPciDevices)
	if err != nil {
		return nil, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, pciDeviceChanges...)

	if pkgcfg.FromContext(vmCtx).Features.IsoSupport {
		cdromDeviceChanges, err := vmutil.UpdateCdromDeviceChanges(vmCtx, s.Client.RestClient(), s.K8sClient, virtualDevices)
		if err != nil {
			return nil, fmt.Errorf("update CD-ROM device changes error: %w", err)
		}
		configSpec.DeviceChange = append(configSpec.DeviceChange, cdromDeviceChanges...)
	}

	return configSpec, nil
}

func (s *Session) prePowerOnVMReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	var configSpec *vimtypes.VirtualMachineConfigSpec
	var err error

	features := pkgcfg.FromContext(vmCtx).Features
	if features.VMResize {
		configSpec, err = s.prePowerOnVMResizeConfigSpec(vmCtx, config, updateArgs)
	} else {
		configSpec, err = s.prePowerOnVMConfigSpec(vmCtx, config, updateArgs)
	}
	if err != nil {
		return err
	}

	if _, err := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("prePowerOnVMReconfigure"),
		),
		vmCtx.VM,
		resVM.VcVM(),
		*configSpec); err != nil {

		return err
	}

	if features.VMResize || features.VMResizeCPUMemory {
		vmopv1util.MustSetLastResizedAnnotation(vmCtx.VM, updateArgs.VMClass)

		vmCtx.VM.Status.Class = &vmopv1common.LocalObjectRef{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       "VirtualMachineClass",
			Name:       updateArgs.VMClass.Name,
		}
	}

	return nil
}

func (s *Session) ensureNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext,
	configSpec *vimtypes.VirtualMachineConfigSpec) (network2.NetworkInterfaceResults, error) {

	networkSpec := vmCtx.VM.Spec.Network
	if networkSpec == nil || networkSpec.Disabled {
		return network2.NetworkInterfaceResults{}, nil
	}

	// This negative device key is the traditional range used for network interfaces.
	deviceKey := int32(-100)

	var networkDevices []vimtypes.BaseVirtualDevice
	if configSpec != nil {
		networkDevices = pkgutil.SelectDevices[vimtypes.BaseVirtualDevice](
			pkgutil.DevicesFromConfigSpec(configSpec),
			pkgutil.IsEthernetCard,
		)
	}

	clusterMoRef := s.Cluster.Reference()
	results, err := network2.CreateAndWaitForNetworkInterfaces(
		vmCtx,
		s.K8sClient,
		s.Client.VimClient(),
		s.Finder,
		&clusterMoRef,
		networkSpec.Interfaces)
	if err != nil {
		return network2.NetworkInterfaceResults{}, err
	}

	// XXX: The following logic assumes that the order of network interfaces specified in the
	// VM spec matches one to one with the device changes in the ConfigSpec in VM class.
	// This is a safe assumption for now since VM service only supports one network interface.
	// TODO: Needs update when VM Service supports VMs with more then one network interface.
	for idx := range results.Results {
		result := &results.Results[idx]

		dev, err := network2.CreateDefaultEthCard(vmCtx, result)
		if err != nil {
			return network2.NetworkInterfaceResults{}, err
		}

		// Use network devices from the class.
		if idx < len(networkDevices) {
			ethCardFromNetProvider := dev.(vimtypes.BaseVirtualEthernetCard)

			if mac := ethCardFromNetProvider.GetVirtualEthernetCard().MacAddress; mac != "" {
				networkDevices[idx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().MacAddress = mac
				networkDevices[idx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().AddressType = string(vimtypes.VirtualEthernetCardMacTypeManual)
			}

			networkDevices[idx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().ExternalId =
				ethCardFromNetProvider.GetVirtualEthernetCard().ExternalId
			// If the device from VM class has a DVX backing, this should still work if the backing as well
			// as the DVX backing are set. VPXD checks for DVX backing before checking for normal device backings.
			networkDevices[idx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard().Backing =
				ethCardFromNetProvider.GetVirtualEthernetCard().Backing

			dev = networkDevices[idx]
		}

		// govmomi assigns a random device key. Fix that up here.
		dev.GetVirtualDevice().Key = deviceKey
		deviceKey--

		result.Device = dev
	}

	return results, nil
}

func (s *Session) ensureCNSVolumes(vmCtx pkgctx.VirtualMachineContext) error {
	// If VM spec has a PVC, check if the volume is attached before powering on
	for _, volume := range vmCtx.VM.Spec.Volumes {
		if volume.PersistentVolumeClaim == nil {
			// Only handle PVC volumes here. In v1a1 we had non-PVC ("vsphereVolumes") but those are gone.
			continue
		}

		// BMV: We should not use the Status as the SoT here. What a mess.
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

func (s *Session) fixupMacAddresses(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	updateArgs *VMUpdateArgs) error {

	missingMAC := false
	for i := range updateArgs.NetworkResults.Results {
		if updateArgs.NetworkResults.Results[i].MacAddress == "" {
			missingMAC = true
			break
		}
	}
	if !missingMAC {
		// Expected path in NSX-T since it always provides the MAC address.
		return nil
	}

	networkDevices, err := resVM.GetNetworkDevices(vmCtx)
	if err != nil {
		return err
	}

	// Just zip these together until we can do interface identification.
	for i := 0; i < min(len(networkDevices), len(updateArgs.NetworkResults.Results)); i++ {
		result := &updateArgs.NetworkResults.Results[i]

		if result.MacAddress == "" {
			ethCard := networkDevices[i].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			result.MacAddress = ethCard.MacAddress
		}
	}

	return nil
}

func (s *Session) customize(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	cfg *vimtypes.VirtualMachineConfigInfo,
	bootstrapArgs vmlifecycle.BootstrapArgs) error {

	return vmlifecycle.DoBootstrap(vmCtx, resVM.VcVM(), cfg, bootstrapArgs)
}

func (s *Session) prepareVMForPowerOn(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	cfg *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	netIfList, err := s.ensureNetworkInterfaces(vmCtx, &updateArgs.ConfigSpec)
	if err != nil {
		return err
	}
	updateArgs.NetworkResults = netIfList

	if err := s.prePowerOnVMReconfigure(vmCtx, resVM, cfg, updateArgs); err != nil {
		return err
	}

	if err := s.fixupMacAddresses(vmCtx, resVM, updateArgs); err != nil {
		return err
	}

	// Get the information required to bootstrap/customize the VM. This is
	// retrieved outside of the customize/DoBootstrap call path in order to use
	// the information to update the VM object's status with the resolved,
	// intended network configuration.
	bootstrapArgs, err := vmlifecycle.GetBootstrapArgs(
		vmCtx,
		s.K8sClient,
		updateArgs.NetworkResults,
		updateArgs.BootstrapData)
	if err != nil {
		return err
	}

	// Update the Kubernetes VM object's status with the resolved, intended
	// network configuration.
	vmlifecycle.UpdateNetworkStatusConfig(vmCtx.VM, bootstrapArgs)

	if err := s.customize(vmCtx, resVM, cfg, bootstrapArgs); err != nil {
		return err
	}

	if err := s.ensureCNSVolumes(vmCtx); err != nil {
		return err
	}

	return nil
}

func (s *Session) poweredOnVMReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo) (bool, error) {

	configSpec := &vimtypes.VirtualMachineConfigSpec{}

	if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		*config,
		configSpec); err != nil {

		return false, err
	}

	UpdateConfigSpecExtraConfig(vmCtx, config, configSpec, nil, nil, vmCtx.VM, nil)
	UpdateConfigSpecChangeBlockTracking(vmCtx, config, configSpec, nil, vmCtx.VM.Spec)

	if pkgcfg.FromContext(vmCtx).Features.IsoSupport {
		if err := vmutil.UpdateConfigSpecCdromDeviceConnection(vmCtx, s.Client.RestClient(), s.K8sClient, config, configSpec); err != nil {
			return false, fmt.Errorf("update CD-ROM device connection error: %w", err)
		}
	}

	refetchProps, err := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("poweredOnVMReconfigure"),
		),
		vmCtx.VM,
		resVM.VcVM(),
		*configSpec)

	if err != nil {
		return false, err
	}

	// Special case for CBT: in order for CBT change take effect for a powered
	// on VM, a checkpoint save/restore is needed. The FSR call allows CBT to
	// take effect for powered-on VMs.
	if configSpec.ChangeTrackingEnabled != nil {
		if err := s.invokeFsrVirtualMachine(vmCtx, resVM); err != nil {
			return refetchProps, fmt.Errorf("failed to invoke FSR for CBT update")
		}
	}

	return refetchProps, nil
}

func (s *Session) attachClusterModule(
	vmCtx pkgctx.VirtualMachineContext,
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

	clusterModuleProvider := clustermodules.NewProvider(s.Client.RestClient())
	return clusterModuleProvider.AddMoRefToModule(vmCtx, moduleUUID, resVM.MoRef())
}

func (s *Session) resizeVMWhenPoweredStateOff(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	moVM mo.VirtualMachine,
	getResizeArgsFn func() (*VMResizeArgs, error)) (bool, error) {

	resizeArgs, err := getResizeArgsFn()
	if err != nil {
		return false, err
	}

	var configSpec vimtypes.VirtualMachineConfigSpec
	var needsResize bool

	if resizeArgs.VMClass != nil {
		needsResize = vmopv1util.ResizeNeeded(*vmCtx.VM, *resizeArgs.VMClass)
		if needsResize {
			if pkgcfg.FromContext(vmCtx).Features.VMResize {
				configSpec, err = resize.CreateResizeConfigSpec(vmCtx, *moVM.Config, resizeArgs.ConfigSpec)
			} else {
				configSpec, err = resize.CreateResizeCPUMemoryConfigSpec(vmCtx, *moVM.Config, resizeArgs.ConfigSpec)
			}
			if err != nil {
				return false, err
			}
		}
	}

	if pkgcfg.FromContext(vmCtx).Features.VMResize {
		if err := vmopv1util.OverwriteResizeConfigSpec(
			vmCtx,
			*vmCtx.VM,
			*moVM.Config,
			&configSpec); err != nil {

			return false, err
		}
	} else if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		*moVM.Config,
		&configSpec); err != nil {

		return false, err
	}

	refetchProps, err := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("resizeVMWhenPoweredStateOff"),
		),
		vmCtx.VM,
		vcVM,
		configSpec)

	if err != nil {
		return false, err
	}

	if needsResize {
		vmopv1util.MustSetLastResizedAnnotation(vmCtx.VM, *resizeArgs.VMClass)
	}

	if resizeArgs.VMClass != nil {
		vmCtx.VM.Status.Class = &vmopv1common.LocalObjectRef{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       "VirtualMachineClass",
			Name:       resizeArgs.VMClass.Name,
		}
	}

	return refetchProps, nil
}

func (s *Session) prePowerOnVMResizeConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimtypes.VirtualMachineConfigSpec, error) {

	var configSpec vimtypes.VirtualMachineConfigSpec

	needsResize := vmopv1util.ResizeNeeded(*vmCtx.VM, updateArgs.VMClass)
	if needsResize {
		cs, err := resize.CreateResizeConfigSpec(vmCtx, *config, updateArgs.ConfigSpec)
		if err != nil {
			return nil, err
		}

		configSpec = cs
	}

	if err := vmopv1util.OverwriteResizeConfigSpec(vmCtx, *vmCtx.VM, *config, &configSpec); err != nil {
		return nil, err
	}

	return &configSpec, nil
}

func (s *Session) updateVMDesiredPowerStateOff(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getResizeArgsFn func() (*VMResizeArgs, error),
	existingPowerState vmopv1.VirtualMachinePowerState) (bool, error) {

	var (
		powerOff     bool
		refetchProps bool
	)

	if existingPowerState == vmopv1.VirtualMachinePowerStateOn {
		powerOff = true
	} else if existingPowerState == vmopv1.VirtualMachinePowerStateSuspended {
		powerOff = vmCtx.VM.Spec.PowerOffMode == vmopv1.VirtualMachinePowerOpModeHard ||
			vmCtx.VM.Spec.PowerOffMode == vmopv1.VirtualMachinePowerOpModeTrySoft
	}
	if powerOff {
		if err := res.NewVMFromObject(vcVM).SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			existingPowerState,
			vmCtx.VM.Spec.PowerState,
			vmCtx.VM.Spec.PowerOffMode); err != nil {

			return false, err
		}

		refetchProps = true
	}

	// A VM's hardware can only be upgraded if the VM is powered off.
	opResult, err := vmutil.ReconcileMinHardwareVersion(
		vmCtx,
		vcVM.Client(),
		vmCtx.MoVM,
		false,
		vmCtx.VM.Spec.MinHardwareVersion)
	if err != nil {
		return refetchProps, err
	}
	if opResult == vmutil.ReconcileMinHardwareVersionResultUpgraded {
		refetchProps = true
	}

	if f := pkgcfg.FromContext(vmCtx).Features; f.VMResize || f.VMResizeCPUMemory {
		refetch, err := s.resizeVMWhenPoweredStateOff(
			vmCtx,
			vcVM,
			vmCtx.MoVM,
			getResizeArgsFn)
		if err != nil {
			return refetchProps, err
		}
		if refetch {
			refetchProps = true
		}
	} else {
		refetch, err := defaultReconfigure(vmCtx, vcVM, *vmCtx.MoVM.Config)
		if err != nil {
			return refetchProps, err
		}
		if refetch {
			refetchProps = true
		}
	}

	return refetchProps, err
}

func (s *Session) updateVMDesiredPowerStateSuspended(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	existingPowerState vmopv1.VirtualMachinePowerState) (bool, error) {

	var (
		refetchProps bool
	)

	if existingPowerState == vmopv1.VirtualMachinePowerStateOn {
		if err := res.NewVMFromObject(vcVM).SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			existingPowerState,
			vmCtx.VM.Spec.PowerState,
			vmCtx.VM.Spec.SuspendMode); err != nil {
			return false, err
		}

		refetchProps = true
	}

	refetch, err := defaultReconfigure(vmCtx, vcVM, *vmCtx.MoVM.Config)
	if err != nil {
		return refetchProps, err
	}
	if refetch {
		refetchProps = true
	}

	return refetchProps, nil
}

func (s *Session) updateVMDesiredPowerStateOn(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	existingPowerState vmopv1.VirtualMachinePowerState) (refetchProps bool, err error) {

	config := vmCtx.MoVM.Config

	// See GoVmomi's VirtualMachine::Device() explanation for this check.
	if config == nil {
		return refetchProps, fmt.Errorf(
			"VM config is not available, connectionState=%s",
			vmCtx.MoVM.Summary.Runtime.ConnectionState)
	}

	resVM := res.NewVMFromObject(vcVM)

	if existingPowerState == vmopv1.VirtualMachinePowerStateOn {
		// Check to see if a possible restart is required.
		// Please note a VM may only be restarted if it is powered on.
		if vmCtx.VM.Spec.NextRestartTime != "" {
			// If non-empty, the value of spec.nextRestartTime is guaranteed
			// to be a valid RFC3339Nano timestamp due to the webhooks,
			// however, we still check for the error due to testing that may
			// not involve webhooks.
			nextRestartTime, err := time.Parse(time.RFC3339Nano, vmCtx.VM.Spec.NextRestartTime)
			if err != nil {
				return refetchProps, fmt.Errorf("spec.nextRestartTime %q cannot be parsed with %q %w",
					vmCtx.VM.Spec.NextRestartTime, time.RFC3339Nano, err)
			}

			result, err := vmutil.RestartAndWait(
				logr.NewContext(vmCtx, vmCtx.Logger),
				vcVM.Client(),
				vmutil.ManagedObjectFromObject(vcVM),
				false,
				nextRestartTime,
				vmutil.ParsePowerOpMode(string(vmCtx.VM.Spec.RestartMode)))
			if err != nil {
				return refetchProps, err
			}
			if result.AnyChange() {
				refetchProps = true
				lastRestartTime := metav1.NewTime(nextRestartTime)
				vmCtx.VM.Status.LastRestartTime = &lastRestartTime
			}
		}

		// Do not pass classConfigSpec to poweredOnVMReconfigure when VM is already powered
		// on since we do not have to get VM class at this point.
		var reconfigured bool
		reconfigured, err = s.poweredOnVMReconfigure(vmCtx, resVM, config)
		if err != nil {
			return refetchProps, err
		}
		refetchProps = refetchProps || reconfigured

		return refetchProps, err
	}

	if existingPowerState == vmopv1.VirtualMachinePowerStateSuspended {
		// A suspended VM cannot be reconfigured.
		err = resVM.SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			existingPowerState,
			vmCtx.VM.Spec.PowerState,
			vmopv1.VirtualMachinePowerOpModeHard)
		return err == nil, err
	}

	updateArgs, err := getUpdateArgsFn()
	if err != nil {
		return refetchProps, err
	}

	// TODO: Find a better place for this?
	err = s.attachClusterModule(vmCtx, resVM, updateArgs.ResourcePolicy)
	if err != nil {
		return refetchProps, err
	}

	// A VM's hardware can only be upgraded if the VM is powered off.
	_, err = vmutil.ReconcileMinHardwareVersion(
		vmCtx,
		vcVM.Client(),
		vmCtx.MoVM,
		false,
		vmCtx.VM.Spec.MinHardwareVersion)
	if err != nil {
		return refetchProps, err
	}

	// Just assume something is going to change after this point - which is most likely true - until
	// this code is refactored more.
	refetchProps = true

	err = s.prepareVMForPowerOn(vmCtx, resVM, config, updateArgs)
	if err != nil {
		return refetchProps, err
	}

	err = resVM.SetPowerState(
		logr.NewContext(vmCtx, vmCtx.Logger),
		existingPowerState,
		vmCtx.VM.Spec.PowerState,
		vmopv1.VirtualMachinePowerOpModeHard)
	if err != nil {
		return refetchProps, err
	}

	if vmCtx.VM.Annotations == nil {
		vmCtx.VM.Annotations = map[string]string{}
	}
	vmCtx.VM.Annotations[vmopv1.FirstBootDoneAnnotation] = "true"

	return refetchProps, err
}

func (s *Session) UpdateVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	getResizeArgsFn func() (*VMResizeArgs, error)) error {

	var refetchProps bool
	var err error

	// Only update VM's power state when VM is not paused.
	if !isVMPaused(vmCtx) {
		// Translate the VM's current power state into the VM Op power state value.
		var existingPowerState vmopv1.VirtualMachinePowerState
		switch vmCtx.MoVM.Summary.Runtime.PowerState {
		case vimtypes.VirtualMachinePowerStatePoweredOn:
			existingPowerState = vmopv1.VirtualMachinePowerStateOn
		case vimtypes.VirtualMachinePowerStatePoweredOff:
			existingPowerState = vmopv1.VirtualMachinePowerStateOff
		case vimtypes.VirtualMachinePowerStateSuspended:
			existingPowerState = vmopv1.VirtualMachinePowerStateSuspended
		}

		switch vmCtx.VM.Spec.PowerState {
		case vmopv1.VirtualMachinePowerStateOff:
			refetchProps, err = s.updateVMDesiredPowerStateOff(
				vmCtx,
				vcVM,
				getResizeArgsFn,
				existingPowerState)

		case vmopv1.VirtualMachinePowerStateSuspended:
			refetchProps, err = s.updateVMDesiredPowerStateSuspended(
				vmCtx,
				vcVM,
				existingPowerState)

		case vmopv1.VirtualMachinePowerStateOn:
			refetchProps, err = s.updateVMDesiredPowerStateOn(
				vmCtx,
				vcVM,
				getUpdateArgsFn,
				existingPowerState)
		}
	} else {
		vmCtx.Logger.Info("VirtualMachine is paused. PowerState is not updated.")
		refetchProps, err = defaultReconfigure(vmCtx, vcVM, *vmCtx.MoVM.Config)
	}

	if refetchProps {
		vmCtx.MoVM = mo.VirtualMachine{}
	}

	vmCtx.Logger.V(8).Info(
		"UpdateVM",
		"refetchProps", refetchProps,
		"powerState", vmCtx.VM.Spec.PowerState)

	updateErr := vmlifecycle.UpdateStatus(vmCtx, s.K8sClient, vcVM, refetchProps)
	if updateErr != nil {
		vmCtx.Logger.Error(updateErr, "Updating VM status failed")
		if err == nil {
			err = updateErr
		}
	}

	return err
}

// Source of truth is EC and Annotation.
func isVMPaused(vmCtx pkgctx.VirtualMachineContext) bool {
	vm := vmCtx.VM

	adminPaused := vmutil.IsPausedByAdmin(vmCtx.MoVM)
	devopsPaused := annotations.HasPaused(vm)

	if adminPaused || devopsPaused {
		if vm.Labels == nil {
			vm.Labels = make(map[string]string)
		}
		switch {
		case adminPaused && devopsPaused:
			vm.Labels[vmopv1.PausedVMLabelKey] = "both"
		case adminPaused:
			vm.Labels[vmopv1.PausedVMLabelKey] = "admin"
		case devopsPaused:
			vm.Labels[vmopv1.PausedVMLabelKey] = "devops"
		}
		return true
	}
	delete(vm.Labels, vmopv1.PausedVMLabelKey)
	return false
}

func defaultReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	configInfo vimtypes.VirtualMachineConfigInfo) (bool, error) {

	var configSpec vimtypes.VirtualMachineConfigSpec
	if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		configInfo,
		&configSpec); err != nil {

		return false, err
	}

	return doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("defaultReconfigure"),
		),
		vmCtx.VM,
		vcVM,
		configSpec)
}

func doReconfigure(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	vcVM *object.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec) (bool, error) {

	var defaultConfigSpec vimtypes.VirtualMachineConfigSpec
	if apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		return false, nil
	}

	resVM := res.NewVMFromObject(vcVM)
	taskInfo, err := resVM.Reconfigure(ctx, &configSpec)

	UpdateVMGuestIDReconfiguredCondition(vm, configSpec, taskInfo)

	if err != nil {
		return false, err
	}

	return true, err
}

// UpdateVMGuestIDReconfiguredCondition deletes the VM's GuestIDReconfigured
// condition if the configSpec doesn't contain a guestID, or if the taskInfo
// does not contain an invalid guestID property error. Otherwise, it sets the
// condition to false with the invalid guest ID property value in the reason.
func UpdateVMGuestIDReconfiguredCondition(
	vm *vmopv1.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec,
	taskInfo *vimtypes.TaskInfo) {

	if configSpec.GuestId != "" && vmutil.IsTaskInfoErrorInvalidGuestID(taskInfo) {
		conditions.MarkFalse(
			vm,
			vmopv1.GuestIDReconfiguredCondition,
			"Invalid",
			fmt.Sprintf(
				"The specified guest ID value is not supported: %s",
				configSpec.GuestId,
			),
		)
		return
	}

	conditions.Delete(vm, vmopv1.GuestIDReconfiguredCondition)
}
