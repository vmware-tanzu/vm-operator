// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha4/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/clustermodules"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	res "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/resources"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/paused"
	"github.com/vmware-tanzu/vm-operator/pkg/util/resize"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	vmutil "github.com/vmware-tanzu/vm-operator/pkg/util/vsphere/vm"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

var (
	ErrSetPowerState          = res.ErrSetPowerState
	ErrBackingUpVM            = virtualmachine.ErrBackingUp
	ErrReconfigure            = pkgerr.NoRequeueError{Message: "reconfiguring vm"}
	ErrUpgradeHardwareVersion = pkgerr.NoRequeueError{Message: "upgrading hardware version"}
	ErrRestartVM              = pkgerr.NoRequeueError{Message: "restarting vm"}
)

// VMUpdateArgs contains the arguments needed to update a VM on VC.
type VMUpdateArgs struct {
	VMClass        vmopv1.VirtualMachineClass
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	MinCPUFreq     uint64
	ExtraConfig    map[string]string
	BootstrapData  vmlifecycle.BootstrapData
	ConfigSpec     vimtypes.VirtualMachineConfigSpec
	NetworkResults network.NetworkInterfaceResults
}

// VMResizeArgs contains the arguments needed to resize a VM on VC.
type VMResizeArgs struct {
	VMClass *vmopv1.VirtualMachineClass
	// ConfigSpec derived from the class and VM Spec.
	ConfigSpec vimtypes.VirtualMachineConfigSpec

	BootstrapData  vmlifecycle.BootstrapData
	ResourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
	NetworkResults network.NetworkInterfaceResults
}

func findMatchingEthCard(
	currentEthCards object.VirtualDeviceList,
	ethCard vimtypes.BaseVirtualEthernetCard) int {

	ethDev := ethCard.GetVirtualEthernetCard()
	matchingIdx := -1

	for idx := range currentEthCards {
		curDev := currentEthCards[idx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()

		if ethDev.AddressType == string(vimtypes.VirtualEthernetCardMacTypeManual) {
			if ethDev.MacAddress != curDev.MacAddress {
				continue
			}
		}

		if ethDev.ExternalId != "" {
			if ethDev.ExternalId != curDev.ExternalId {
				continue
			}
		}

		if curDev.Backing == nil {
			continue
		}

		var backingMatch bool
		switch a := ethDev.Backing.(type) {
		case *vimtypes.VirtualEthernetCardNetworkBackingInfo:
			backingMatch = resize.MatchVirtualEthernetCardNetworkBackingInfo(a, curDev.Backing)
		case *vimtypes.VirtualEthernetCardDistributedVirtualPortBackingInfo:
			backingMatch = resize.MatchVirtualEthernetCardDistributedVirtualPortBackingInfo(a, curDev.Backing)
		case *vimtypes.VirtualEthernetCardOpaqueNetworkBackingInfo:
			backingMatch = resize.MatchVirtualEthernetCardOpaqueNetworkBackingInfo(a, curDev.Backing)
		}

		if backingMatch {
			matchingIdx = idx
			break
		}
	}

	return matchingIdx
}

func UpdateEthCardDeviceChanges(
	_ context.Context,
	results *network.NetworkInterfaceResults,
	currentEthCards object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	var deviceChanges []vimtypes.BaseVirtualDeviceConfigSpec

	for idx, r := range results.Results {
		matchingIdx := findMatchingEthCard(currentEthCards, r.Device.(vimtypes.BaseVirtualEthernetCard))
		if matchingIdx == -1 {
			results.AddedEthernetCard = true
			deviceChanges = append(deviceChanges, &vimtypes.VirtualDeviceConfigSpec{
				Device:    r.Device,
				Operation: vimtypes.VirtualDeviceConfigSpecOperationAdd,
			})
		} else {
			matchDev := currentEthCards[matchingIdx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			results.Results[idx].DeviceKey = matchDev.Key
			results.Results[idx].MacAddress = matchDev.MacAddress

			// This expected ethernet card from the results matched with a current one so claim it.
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

// UpdateConfigSpecExtraConfig updates the ExtraConfig of the given ConfigSpec.
// At a minimum, config and configSpec must be non-nil, in which case it will
// just ensure MMPowerOffVMExtraConfigKey is no longer part of ExtraConfig.
func UpdateConfigSpecExtraConfig(
	ctx context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
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

func UpdateConfigSpecChangeBlockTracking(
	ctx context.Context,
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vmSpec vmopv1.VirtualMachineSpec) {

	if adv := vmSpec.Advanced; adv != nil && adv.ChangeBlockTracking != nil {
		if !apiEquality.Semantic.DeepEqual(config.ChangeTrackingEnabled, adv.ChangeBlockTracking) {
			configSpec.ChangeTrackingEnabled = adv.ChangeBlockTracking
		}
		return
	}
}

func UpdateHardwareConfigSpec(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec,
	vmClassSpec *vmopv1.VirtualMachineClassSpec) {

	if nCPUs := int32(vmClassSpec.Hardware.Cpus); config.Hardware.NumCPU != nCPUs { //nolint:gosec // disable G115
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

// updateConfigSpec overlays the VM Class spec with the provided ConfigSpec to
// form a desired ConfigSpec that will be used to reconfigure the VM.
func updateConfigSpec(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimtypes.VirtualMachineConfigSpec, bool, error) {

	configSpec := &vimtypes.VirtualMachineConfigSpec{}

	if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		*config,
		configSpec); err != nil {

		return nil, false, err
	}

	UpdateConfigSpecExtraConfig(vmCtx, config, configSpec, vmCtx.VM, updateArgs.ExtraConfig)
	UpdateConfigSpecAnnotation(config, configSpec)
	UpdateConfigSpecChangeBlockTracking(vmCtx, config, configSpec, vmCtx.VM.Spec)
	UpdateConfigSpecGuestID(config, configSpec, vmCtx.VM.Spec.GuestID)

	needsResize := false
	if pkgcfg.FromContext(vmCtx).Features.VMResizeCPUMemory && vmopv1util.ResizeNeeded(*vmCtx.VM, updateArgs.VMClass) {
		needsResize = true
		vmClassSpec := updateArgs.VMClass.Spec
		UpdateHardwareConfigSpec(config, configSpec, &vmClassSpec)
		resize.CompareCPUAllocation(*config, updateArgs.ConfigSpec, configSpec)
		resize.CompareMemoryAllocation(*config, updateArgs.ConfigSpec, configSpec)
	}

	return configSpec, needsResize, nil
}

func (s *Session) getConfigSpecForPoweredOffVM(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimtypes.VirtualMachineConfigSpec, bool, error) {

	configSpec, needsResize, err := updateConfigSpec(vmCtx, config, updateArgs)
	if err != nil {
		return nil, false, err
	}

	virtualDevices := object.VirtualDeviceList(config.Hardware.Device)
	currentDisks := virtualDevices.SelectByType((*vimtypes.VirtualDisk)(nil))
	currentEthCards := virtualDevices.SelectByType((*vimtypes.VirtualEthernetCard)(nil))

	diskDeviceChanges, err := updateVirtualDiskDeviceChanges(vmCtx, currentDisks)
	if err != nil {
		return nil, false, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, diskDeviceChanges...)

	ethCardDeviceChanges, err := UpdateEthCardDeviceChanges(vmCtx, &updateArgs.NetworkResults, currentEthCards)
	if err != nil {
		return nil, false, err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, ethCardDeviceChanges...)

	cdromDeviceChanges, err := virtualmachine.UpdateCdromDeviceChanges(vmCtx, s.Client.RestClient(), s.K8sClient, virtualDevices)
	if err != nil {
		return nil, false, fmt.Errorf("update CD-ROM device changes error: %w", err)
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, cdromDeviceChanges...)

	return configSpec, needsResize, nil
}

func (s *Session) poweredOffReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	var configSpec *vimtypes.VirtualMachineConfigSpec
	var needsResize bool
	var err error

	if pkgcfg.FromContext(vmCtx).Features.VMResize {
		configSpec, needsResize, err = s.getResizeConfigSpecForPoweredOffVM(
			vmCtx, config, updateArgs)
	} else {
		configSpec, needsResize, err = s.getConfigSpecForPoweredOffVM(
			vmCtx, config, updateArgs)
	}
	if err != nil {
		return err
	}

	reconfigErr := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("poweredOffReconfigure"),
		),
		s.K8sClient,
		vmCtx.VM,
		resVM.VcVM(),
		vmCtx.MoVM,
		*configSpec)

	if needsResize && errors.Is(reconfigErr, ErrReconfigure) {
		vmopv1util.MustSetLastResizedAnnotation(vmCtx.VM, updateArgs.VMClass)

		vmCtx.VM.Status.Class = &vmopv1common.LocalObjectRef{
			APIVersion: vmopv1.GroupVersion.String(),
			Kind:       "VirtualMachineClass",
			Name:       updateArgs.VMClass.Name,
		}
	}

	return reconfigErr
}

func (s *Session) ensureNetworkInterfaces(
	vmCtx pkgctx.VirtualMachineContext) (network.NetworkInterfaceResults, error) {

	networkSpec := vmCtx.VM.Spec.Network
	if networkSpec == nil || networkSpec.Disabled {
		// TODO: Remove all interfaces.
		return network.NetworkInterfaceResults{}, nil
	}

	results, err := network.CreateAndWaitForNetworkInterfaces(
		vmCtx,
		s.K8sClient,
		s.Client.VimClient(),
		s.Finder,
		&s.ClusterMoRef,
		networkSpec)
	if err != nil {
		return network.NetworkInterfaceResults{}, err
	}

	for idx := range results.Results {
		result := &results.Results[idx]

		dev, err := network.CreateDefaultEthCard(vmCtx, result)
		if err != nil {
			return network.NetworkInterfaceResults{}, err
		}

		result.Device = dev
	}

	if pkgcfg.FromContext(vmCtx).Features.MutableNetworks {
		if err := network.ListOrphanedNetworkInterfaces(vmCtx, s.K8sClient, &results); err != nil {
			return network.NetworkInterfaceResults{}, err
		}
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
	networkResults network.NetworkInterfaceResults) error {

	if pkgcfg.FromContext(vmCtx).Features.MutableNetworks {
		return s.fixupMacAddressMutableNetworks(vmCtx, resVM, networkResults)
	}

	missingMAC := false
	for i := range networkResults.Results {
		if networkResults.Results[i].MacAddress == "" {
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
	for i := 0; i < min(len(networkDevices), len(networkResults.Results)); i++ {
		result := &networkResults.Results[i]

		if result.MacAddress == "" {
			ethCard := networkDevices[i].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			result.MacAddress = ethCard.MacAddress
		}
	}

	return nil
}

func (s *Session) fixupMacAddressMutableNetworks(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	networkResults network.NetworkInterfaceResults) error {

	if !networkResults.AddedEthernetCard {
		return nil
	}

	networkDevices, err := resVM.GetNetworkDevices(vmCtx)
	if err != nil {
		return err
	}

	for idx, r := range networkResults.Results {
		if r.DeviceKey != 0 {
			matchingIdx := slices.IndexFunc(networkDevices,
				func(d vimtypes.BaseVirtualDevice) bool { return d.GetVirtualDevice().Key == r.DeviceKey })
			if matchingIdx >= 0 {
				networkDevices = append(networkDevices[:matchingIdx], networkDevices[matchingIdx+1:]...)
			}
			continue
		}

		matchingIdx := findMatchingEthCard(networkDevices, r.Device.(vimtypes.BaseVirtualEthernetCard))
		if matchingIdx >= 0 {
			matchDev := networkDevices[matchingIdx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			networkResults.Results[idx].DeviceKey = matchDev.Key
			networkResults.Results[idx].MacAddress = matchDev.MacAddress

			networkDevices = append(networkDevices[:matchingIdx], networkDevices[matchingIdx+1:]...)
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

func (s *Session) poweredOnVMReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo) error {

	configSpec := &vimtypes.VirtualMachineConfigSpec{}

	if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		*config,
		configSpec); err != nil {

		return err
	}

	UpdateConfigSpecExtraConfig(vmCtx, config, configSpec, vmCtx.VM, nil)
	UpdateConfigSpecChangeBlockTracking(vmCtx, config, configSpec, vmCtx.VM.Spec)

	if err := virtualmachine.UpdateConfigSpecCdromDeviceConnection(vmCtx, s.Client.RestClient(), s.K8sClient, config, configSpec); err != nil {
		return fmt.Errorf("update CD-ROM device connection error: %w", err)
	}

	if err := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("poweredOnVMReconfigure"),
		),
		s.K8sClient,
		vmCtx.VM,
		resVM.VcVM(),
		vmCtx.MoVM,
		*configSpec); err != nil {

		return err
	}

	// Special case for CBT: in order for CBT change take effect for a powered
	// on VM, a checkpoint save/restore is needed. The FSR call allows CBT to
	// take effect for powered-on VMs.
	if configSpec.ChangeTrackingEnabled != nil {
		if err := s.invokeFsrVirtualMachine(vmCtx, resVM); err != nil {
			return fmt.Errorf("failed to invoke FSR for CBT update")
		}
	}

	return nil
}

func (s *Session) attachClusterModule(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy) error {

	// The clusterModule is required be able to enforce the vm-vm anti-affinity policy.
	clusterModuleName := vmCtx.VM.Annotations[pkgconst.ClusterModuleNameAnnotationKey]
	if clusterModuleName == "" {
		return nil
	}

	// Find ClusterModule UUID from the ResourcePolicy.
	_, moduleUUID := clustermodules.FindClusterModuleUUID(vmCtx, clusterModuleName, s.ClusterMoRef, resourcePolicy)
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
	resizeArgs *VMResizeArgs) error {

	var (
		err         error
		needsResize bool
		configSpec  vimtypes.VirtualMachineConfigSpec
	)

	if resizeArgs.VMClass != nil {
		needsResize = vmopv1util.ResizeNeeded(*vmCtx.VM, *resizeArgs.VMClass)
		if needsResize {
			if pkgcfg.FromContext(vmCtx).Features.VMResize {
				configSpec, err = resize.CreateResizeConfigSpec(vmCtx, *moVM.Config, resizeArgs.ConfigSpec)
			} else {
				configSpec, err = resize.CreateResizeCPUMemoryConfigSpec(vmCtx, *moVM.Config, resizeArgs.ConfigSpec)
			}
			if err != nil {
				return err
			}
		}
	}

	if pkgcfg.FromContext(vmCtx).Features.VMResize {
		if err := vmopv1util.OverwriteResizeConfigSpec(
			vmCtx,
			*vmCtx.VM,
			*moVM.Config,
			&configSpec); err != nil {

			return err
		}
	} else if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		*moVM.Config,
		&configSpec); err != nil {

		return err
	}

	reconfigErr := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("resizeVMWhenPoweredStateOff"),
		),
		s.K8sClient,
		vmCtx.VM,
		vcVM,
		vmCtx.MoVM,
		configSpec)

	if reconfigErr != nil && !errors.Is(reconfigErr, ErrReconfigure) {
		return err
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

	return reconfigErr
}

func (s *Session) getResizeConfigSpecForPoweredOffVM(
	vmCtx pkgctx.VirtualMachineContext,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) (*vimtypes.VirtualMachineConfigSpec, bool, error) {

	var configSpec vimtypes.VirtualMachineConfigSpec

	needsResize := vmopv1util.ResizeNeeded(*vmCtx.VM, updateArgs.VMClass)
	if needsResize {
		cs, err := resize.CreateResizeConfigSpec(vmCtx, *config, updateArgs.ConfigSpec)
		if err != nil {
			return nil, false, err
		}

		configSpec = cs
	}

	if err := vmopv1util.OverwriteResizeConfigSpec(vmCtx, *vmCtx.VM, *config, &configSpec); err != nil {
		return nil, false, err
	}

	return &configSpec, needsResize, nil
}

func (s *Session) updateVMDesiredPowerStateOff(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	getResizeArgsFn func() (*VMResizeArgs, error),
	currentPowerState vmopv1.VirtualMachinePowerState) error {

	vmCtx.Logger.V(4).Info("updateVMDesiredPowerStateOff")

	var setPowerState bool
	if currentPowerState == vmopv1.VirtualMachinePowerStateOn {
		setPowerState = true
	} else if currentPowerState == vmopv1.VirtualMachinePowerStateSuspended {
		setPowerState = vmCtx.VM.Spec.PowerOffMode == vmopv1.VirtualMachinePowerOpModeHard ||
			vmCtx.VM.Spec.PowerOffMode == vmopv1.VirtualMachinePowerOpModeTrySoft
	}
	if setPowerState {
		// Power off the VM and end the reconcile loop early.
		return res.NewVMFromObject(vcVM).SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			currentPowerState,
			vmCtx.VM.Spec.PowerState,
			vmCtx.VM.Spec.PowerOffMode)
	}

	return s.reconcilePoweredOffVM(
		vmCtx,
		vcVM,
		res.NewVMFromObject(vcVM),
		getUpdateArgsFn,
		getResizeArgsFn)
}

func (s *Session) reconcileHardwareVersion(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine) error {

	// A VM's hardware can only be upgraded if the VM is powered off.
	opResult, err := vmutil.ReconcileMinHardwareVersion(
		vmCtx,
		vcVM.Client(),
		vmCtx.MoVM,
		false,
		vmCtx.VM.Spec.MinHardwareVersion)
	if err != nil {
		return err
	}
	if opResult == vmutil.ReconcileMinHardwareVersionResultUpgraded {
		return ErrUpgradeHardwareVersion
	}

	return nil
}

func (s *Session) updateVMDesiredPowerStateSuspended(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	currentPowerState vmopv1.VirtualMachinePowerState) error {

	vmCtx.Logger.V(4).Info("updateVMDesiredPowerStateSuspended")

	if currentPowerState == vmopv1.VirtualMachinePowerStateOn ||
		currentPowerState == vmopv1.VirtualMachinePowerStateOff {

		// Cannot reconfigure a suspended VM.
		if err := defaultReconfigure(vmCtx, s.K8sClient, vcVM); err != nil {
			return err
		}
	}

	if currentPowerState == vmopv1.VirtualMachinePowerStateOn {
		return res.NewVMFromObject(vcVM).SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			currentPowerState,
			vmCtx.VM.Spec.PowerState,
			vmCtx.VM.Spec.SuspendMode)
	}

	return nil
}

func (s *Session) updateVMDesiredPowerStateOn(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	getResizeArgsFn func() (*VMResizeArgs, error),
	currentPowerState vmopv1.VirtualMachinePowerState) error {

	vmCtx.Logger.V(4).Info("updateVMDesiredPowerStateOn")

	config := vmCtx.MoVM.Config

	// See GoVmomi's VirtualMachine::Device() explanation for this check.
	if config == nil {
		return fmt.Errorf(
			"VM config is not available, connectionState=%s",
			vmCtx.MoVM.Summary.Runtime.ConnectionState)
	}

	resVM := res.NewVMFromObject(vcVM)

	if currentPowerState == vmopv1.VirtualMachinePowerStateOn {
		// Check to see if a possible restart is required.
		// Please note a VM may only be restarted if it is powered on.
		if vmCtx.VM.Spec.NextRestartTime != "" {
			// If non-empty, the value of spec.nextRestartTime is guaranteed
			// to be a valid RFC3339Nano timestamp due to the webhooks,
			// however, we still check for the error due to testing that may
			// not involve webhooks.
			nextRestartTime, err := time.Parse(time.RFC3339Nano, vmCtx.VM.Spec.NextRestartTime)
			if err != nil {
				return fmt.Errorf("spec.nextRestartTime %q cannot be parsed with %q %w",
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
				return err
			}
			if result.AnyChange() {
				lastRestartTime := metav1.NewTime(nextRestartTime)
				vmCtx.VM.Status.LastRestartTime = &lastRestartTime
				return ErrRestartVM
			}
		}

		// Do not pass classConfigSpec to poweredOnVMReconfigure when VM is
		// already powered on. Thus we do not need the VM class at this point.
		return s.poweredOnVMReconfigure(vmCtx, resVM, config)
	}

	var skipPowerOn bool
	for k, v := range vmCtx.VM.Annotations {
		if strings.HasPrefix(k, vmopv1.CheckAnnotationPowerOn+"/") {
			skipPowerOn = true
			vmCtx.Logger.Info(
				"Skipping poweron due to annotation",
				"annotationKey", k, "annotationValue", v)
		}
	}

	if currentPowerState == vmopv1.VirtualMachinePowerStateOff {
		// Only reconfigure powered off VMs. Suspended VMs cannot be
		// reconfigured.
		if err := s.reconcilePoweredOffVM(
			vmCtx,
			vcVM,
			resVM,
			getUpdateArgsFn,
			getResizeArgsFn); err != nil {

			return err
		}
	}

	if !skipPowerOn {
		err := resVM.SetPowerState(
			logr.NewContext(vmCtx, vmCtx.Logger),
			currentPowerState,
			vmCtx.VM.Spec.PowerState,
			vmopv1.VirtualMachinePowerOpModeHard)

		if errors.Is(err, ErrSetPowerState) {
			if vmCtx.VM.Annotations == nil {
				vmCtx.VM.Annotations = map[string]string{}
			}
			vmCtx.VM.Annotations[vmopv1.FirstBootDoneAnnotation] = "true"
		}

		return err
	}

	return nil
}

func (s *Session) reconcilePoweredOffVM(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	resVM *res.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	getResizeArgsFn func() (*VMResizeArgs, error)) error {

	var (
		updateArgs     *VMUpdateArgs
		resizeArgs     *VMResizeArgs
		resourcePolicy *vmopv1.VirtualMachineSetResourcePolicy
		bootstrapData  vmlifecycle.BootstrapData
	)

	if f := pkgcfg.FromContext(vmCtx).Features; f.VMResize || f.VMResizeCPUMemory {
		var err error
		resizeArgs, err = getResizeArgsFn()
		if err != nil {
			return err
		}
		bootstrapData = resizeArgs.BootstrapData
		resourcePolicy = resizeArgs.ResourcePolicy
	} else {
		var err error
		updateArgs, err = getUpdateArgsFn()
		if err != nil {
			return err
		}
		bootstrapData = updateArgs.BootstrapData
		resourcePolicy = updateArgs.ResourcePolicy
	}

	// TODO: Find a better place for this?
	if err := s.attachClusterModule(vmCtx, resVM, resourcePolicy); err != nil {
		return err
	}

	// A VM's hardware can only be upgraded if the VM is powered off.
	if err := s.reconcileHardwareVersion(vmCtx, vcVM); err != nil {
		return err
	}

	if vmCtx.VM.Spec.GuestID == "" {
		// Assume the guest ID is valid until we know otherwise.
		conditions.Delete(vmCtx.VM, vmopv1.GuestIDReconfiguredCondition)
	}

	networkResults, err := s.ensureNetworkInterfaces(vmCtx)
	if err != nil {
		return err
	}

	if f := pkgcfg.FromContext(vmCtx).Features; f.VMResize || f.VMResizeCPUMemory {
		resizeArgs.NetworkResults = networkResults
		if err := s.resizeVMWhenPoweredStateOff(
			vmCtx,
			vcVM,
			vmCtx.MoVM,
			resizeArgs); err != nil {

			return err
		}
	} else {
		updateArgs.NetworkResults = networkResults
		if err := s.poweredOffReconfigure(
			vmCtx,
			resVM,
			vmCtx.MoVM.Config,
			updateArgs); err != nil {

			return err
		}
	}

	if err := s.ensureCNSVolumes(vmCtx); err != nil {
		return err
	}

	for i := range networkResults.OrphanedNetworkInterfaces {
		if err := s.K8sClient.Delete(
			vmCtx,
			networkResults.OrphanedNetworkInterfaces[i],
		); err != nil {

			return err
		}
	}

	if err := s.fixupMacAddresses(vmCtx, resVM, networkResults); err != nil {
		return err
	}

	// Get the information required to bootstrap/customize the VM. This is
	// retrieved outside of the customize/DoBootstrap call path in order to use
	// the information to update the VM object's status with the resolved,
	// intended network configuration.
	bootstrapArgs, err := vmlifecycle.GetBootstrapArgs(
		vmCtx,
		s.K8sClient,
		networkResults,
		bootstrapData)
	if err != nil {
		return err
	}

	// Update the Kubernetes VM object's status with the resolved, intended
	// network configuration.
	vmlifecycle.UpdateNetworkStatusConfig(vmCtx.VM, bootstrapArgs)

	// TODO(akutz) For now we only customize the VM if the desired power state
	//             is poweredOn. This is because we do not currently no-op
	//             customization based on the current customization state of the
	//             VM.
	if vmCtx.VM.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn {
		if err := s.customize(
			vmCtx,
			resVM,
			vmCtx.MoVM.Config,
			bootstrapArgs); err != nil {

			return err
		}
	}

	return nil
}

func (s *Session) UpdateVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	getResizeArgsFn func() (*VMResizeArgs, error)) error {

	var (
		updateErr error
		isPaused  = isVMPaused(vmCtx)
		hasTask   = vmCtx.VM.Status.TaskID != ""
	)

	// Only update VM's power state when VM:
	// - is not paused
	// - does not have an outstanding task
	if !isPaused && !hasTask {
		// Translate the VM's current power state into the VM Op power state value.
		var currentPowerState vmopv1.VirtualMachinePowerState
		switch vmCtx.MoVM.Summary.Runtime.PowerState {
		case vimtypes.VirtualMachinePowerStatePoweredOn:
			currentPowerState = vmopv1.VirtualMachinePowerStateOn
		case vimtypes.VirtualMachinePowerStatePoweredOff:
			currentPowerState = vmopv1.VirtualMachinePowerStateOff
		case vimtypes.VirtualMachinePowerStateSuspended:
			currentPowerState = vmopv1.VirtualMachinePowerStateSuspended
		}

		switch vmCtx.VM.Spec.PowerState {
		case vmopv1.VirtualMachinePowerStateOff:
			updateErr = s.updateVMDesiredPowerStateOff(
				vmCtx,
				vcVM,
				getUpdateArgsFn,
				getResizeArgsFn,
				currentPowerState)

		case vmopv1.VirtualMachinePowerStateSuspended:
			updateErr = s.updateVMDesiredPowerStateSuspended(
				vmCtx,
				vcVM,
				currentPowerState)

		case vmopv1.VirtualMachinePowerStateOn:
			updateErr = s.updateVMDesiredPowerStateOn(
				vmCtx,
				vcVM,
				getUpdateArgsFn,
				getResizeArgsFn,
				currentPowerState)
		}
	} else {
		vmCtx.Logger.Info("PowerState is not updated.",
			"isPaused", isPaused, "hasTask", hasTask)
		updateErr = defaultReconfigure(vmCtx, s.K8sClient, vcVM)
	}

	if updateErr != nil && !pkgerr.IsNoRequeueError(updateErr) {
		updateErr = fmt.Errorf("updating state failed with %w", updateErr)
	}

	if pkgcfg.FromContext(vmCtx).Features.BringYourOwnEncryptionKey {
		for _, r := range vmconfig.FromContext(vmCtx) {
			if err := r.OnResult(
				vmCtx,
				vmCtx.VM,
				vmCtx.MoVM,
				updateErr); err != nil {

				err = fmt.Errorf("%s.OnResult failed with %w", r.Name(), err)
				if updateErr == nil {
					updateErr = err
				} else {
					updateErr = fmt.Errorf("%w, %w", updateErr, err)
				}
			}
		}
	}

	return updateErr
}

// Source of truth is EC and Annotation.
func isVMPaused(vmCtx pkgctx.VirtualMachineContext) bool {
	byAdmin := paused.ByAdmin(vmCtx.MoVM)
	byDevOps := paused.ByDevOps(vmCtx.VM)

	if byAdmin || byDevOps {
		if vmCtx.VM.Labels == nil {
			vmCtx.VM.Labels = make(map[string]string)
		}
		switch {
		case byAdmin && byDevOps:
			vmCtx.VM.Labels[vmopv1.PausedVMLabelKey] = "both"
		case byAdmin:
			vmCtx.VM.Labels[vmopv1.PausedVMLabelKey] = "admin"
		case byDevOps:
			vmCtx.VM.Labels[vmopv1.PausedVMLabelKey] = "devops"
		}
		return true
	}
	delete(vmCtx.VM.Labels, vmopv1.PausedVMLabelKey)
	return false
}

func defaultReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	k8sClient ctrlclient.Client,
	vcVM *object.VirtualMachine) error {

	var configInfo vimtypes.VirtualMachineConfigInfo
	if vmCtx.MoVM.Config != nil {
		configInfo = *vmCtx.MoVM.Config
	}

	var configSpec vimtypes.VirtualMachineConfigSpec
	if err := vmopv1util.OverwriteAlwaysResizeConfigSpec(
		vmCtx,
		*vmCtx.VM,
		configInfo,
		&configSpec); err != nil {

		return err
	}

	return doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("defaultReconfigure"),
		),
		k8sClient,
		vmCtx.VM,
		vcVM,
		vmCtx.MoVM,
		configSpec)
}

func doReconfigure(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vm *vmopv1.VirtualMachine,
	vcVM *object.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec vimtypes.VirtualMachineConfigSpec) error {

	logger := logr.FromContextOrDiscard(ctx)
	if pkgcfg.FromContext(ctx).Features.BringYourOwnEncryptionKey {
		for _, r := range vmconfig.FromContext(ctx) {
			logger.Info("Reconciling vmconfig", "reconciler", r.Name())

			if err := r.Reconcile(
				ctx,
				k8sClient,
				vcVM.Client(),
				vm,
				moVM,
				&configSpec); err != nil {

				return err
			}
		}
	}

	var defaultConfigSpec vimtypes.VirtualMachineConfigSpec
	if apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		return nil
	}

	resVM := res.NewVMFromObject(vcVM)
	taskInfo, err := resVM.Reconfigure(ctx, &configSpec)

	UpdateVMGuestIDReconfiguredCondition(vm, configSpec, taskInfo)

	if err != nil {
		return err
	}

	return ErrReconfigure
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
			"The specified guest ID value is not supported: %s", configSpec.GuestId,
		)
		return
	}

	conditions.Delete(vm, vmopv1.GuestIDReconfiguredCondition)
}
