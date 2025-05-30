// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package session

import (
	"context"
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
}

func UpdateEthCardDeviceChanges(
	ctx context.Context,
	results *network.NetworkInterfaceResults,
	currentEthCards object.VirtualDeviceList) ([]vimtypes.BaseVirtualDeviceConfigSpec, error) {

	return network.ReconcileNetworkInterfaces(ctx, results, currentEthCards)
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

func (s *Session) prePowerOnVMConfigSpec(
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

func (s *Session) prePowerOnVMReconfigure(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	config *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	var configSpec *vimtypes.VirtualMachineConfigSpec
	var needsResize bool
	var err error

	if pkgcfg.FromContext(vmCtx).Features.VMResize {
		configSpec, needsResize, err = s.prePowerOnVMResizeConfigSpec(vmCtx, config, updateArgs)
	} else {
		configSpec, needsResize, err = s.prePowerOnVMConfigSpec(vmCtx, config, updateArgs)
	}
	if err != nil {
		return err
	}

	if _, err := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("prePowerOnVMReconfigure"),
		),
		s.K8sClient,
		vmCtx.VM,
		resVM.VcVM(),
		vmCtx.MoVM,
		*configSpec); err != nil {

		return err
	}

	if needsResize {
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
	updateArgs *VMUpdateArgs) error {

	if pkgcfg.FromContext(vmCtx).Features.MutableNetworks {
		return s.fixupMacAddressMutableNetworks(vmCtx, resVM, updateArgs)
	}

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

func (s *Session) fixupMacAddressMutableNetworks(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	updateArgs *VMUpdateArgs) error {

	if !updateArgs.NetworkResults.AddedEthernetCard {
		return nil
	}

	networkDevices, err := resVM.GetNetworkDevices(vmCtx)
	if err != nil {
		return err
	}

	for idx, r := range updateArgs.NetworkResults.Results {
		if r.DeviceKey != 0 {
			matchingIdx := slices.IndexFunc(networkDevices,
				func(d vimtypes.BaseVirtualDevice) bool { return d.GetVirtualDevice().Key == r.DeviceKey })
			if matchingIdx >= 0 {
				networkDevices = append(networkDevices[:matchingIdx], networkDevices[matchingIdx+1:]...)
			}
			continue
		}

		matchingIdx := network.FindMatchingEthCard(networkDevices, r.Device.(vimtypes.BaseVirtualEthernetCard))
		if matchingIdx >= 0 {
			matchDev := networkDevices[matchingIdx].(vimtypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
			updateArgs.NetworkResults.Results[idx].DeviceKey = matchDev.Key
			updateArgs.NetworkResults.Results[idx].MacAddress = matchDev.MacAddress

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

func (s *Session) prepareVMForPowerOn(
	vmCtx pkgctx.VirtualMachineContext,
	resVM *res.VirtualMachine,
	cfg *vimtypes.VirtualMachineConfigInfo,
	updateArgs *VMUpdateArgs) error {

	netIfList, err := s.ensureNetworkInterfaces(vmCtx)
	if err != nil {
		return err
	}
	updateArgs.NetworkResults = netIfList

	if err := s.prePowerOnVMReconfigure(vmCtx, resVM, cfg, updateArgs); err != nil {
		return err
	}

	for i := range updateArgs.NetworkResults.OrphanedNetworkInterfaces {
		if err := s.K8sClient.Delete(vmCtx, updateArgs.NetworkResults.OrphanedNetworkInterfaces[i]); err != nil {
			return err
		}
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

	UpdateConfigSpecExtraConfig(vmCtx, config, configSpec, vmCtx.VM, nil)
	UpdateConfigSpecChangeBlockTracking(vmCtx, config, configSpec, vmCtx.VM.Spec)

	if err := virtualmachine.UpdateConfigSpecCdromDeviceConnection(vmCtx, s.Client.RestClient(), s.K8sClient, config, configSpec); err != nil {
		return false, fmt.Errorf("update CD-ROM device connection error: %w", err)
	}

	refetchProps, err := doReconfigure(
		logr.NewContext(
			vmCtx,
			vmCtx.Logger.WithName("poweredOnVMReconfigure"),
		),
		s.K8sClient,
		vmCtx.VM,
		resVM.VcVM(),
		vmCtx.MoVM,
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
		s.K8sClient,
		vmCtx.VM,
		vcVM,
		vmCtx.MoVM,
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
		refetch, err := defaultReconfigure(vmCtx, s.K8sClient, vcVM)
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

	refetch, err := defaultReconfigure(vmCtx, s.K8sClient, vcVM)
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

	var skipPowerOn bool
	for k, v := range vmCtx.VM.Annotations {
		if strings.HasPrefix(k, vmopv1.CheckAnnotationPowerOn+"/") {
			skipPowerOn = true
			vmCtx.Logger.Info(
				"Skipping poweron due to annotation",
				"annotationKey", k, "annotationValue", v)
		}
	}

	if !skipPowerOn && existingPowerState == vmopv1.VirtualMachinePowerStateSuspended {
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

	if !skipPowerOn {
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
	}

	return refetchProps, err
}

func (s *Session) UpdateVirtualMachine(
	vmCtx pkgctx.VirtualMachineContext,
	vcVM *object.VirtualMachine,
	getUpdateArgsFn func() (*VMUpdateArgs, error),
	getResizeArgsFn func() (*VMResizeArgs, error)) error {

	var (
		refetchProps bool
		updateErr    error
		isPaused     = isVMPaused(vmCtx)
		hasTask      = vmCtx.VM.Status.TaskID != ""
	)

	// Only update VM's power state when VM:
	// - is not paused
	// - does not have an outstanding task
	if !isPaused && !hasTask {
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
			refetchProps, updateErr = s.updateVMDesiredPowerStateOff(
				vmCtx,
				vcVM,
				getResizeArgsFn,
				existingPowerState)

		case vmopv1.VirtualMachinePowerStateSuspended:
			refetchProps, updateErr = s.updateVMDesiredPowerStateSuspended(
				vmCtx,
				vcVM,
				existingPowerState)

		case vmopv1.VirtualMachinePowerStateOn:
			refetchProps, updateErr = s.updateVMDesiredPowerStateOn(
				vmCtx,
				vcVM,
				getUpdateArgsFn,
				existingPowerState)
		}
	} else {
		vmCtx.Logger.Info("PowerState is not updated.",
			"isPaused", isPaused, "hasTask", hasTask)
		refetchProps, updateErr = defaultReconfigure(vmCtx, s.K8sClient, vcVM)
	}

	if updateErr != nil {
		updateErr = fmt.Errorf("updating state failed with %w", updateErr)
	}

	if refetchProps {
		vmCtx.Logger.V(8).Info(
			"Refetching properties",
			"refetchProps", refetchProps,
			"powerState", vmCtx.VM.Spec.PowerState)

		vmCtx.MoVM = mo.VirtualMachine{}

		if err := vcVM.Properties(
			vmCtx,
			vcVM.Reference(),
			vmlifecycle.VMStatusPropertiesSelector,
			&vmCtx.MoVM); err != nil {

			err = fmt.Errorf("refetching props failed with %w", err)
			if updateErr == nil {
				updateErr = err
			} else {
				updateErr = fmt.Errorf("%w, %w", updateErr, err)
			}
		}
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

	var networkDeviceKeysToSpecIdx map[int32]int
	if vmCtx.MoVM.Config != nil {
		networkDeviceKeysToSpecIdx = network.MapEthernetDevicesToSpecIdx(vmCtx, s.K8sClient, vmCtx.MoVM)
	}

	err := vmlifecycle.UpdateStatus(
		vmCtx,
		s.K8sClient,
		vcVM,
		vmlifecycle.UpdateStatusData{
			NetworkDeviceKeysToSpecIdx: networkDeviceKeysToSpecIdx,
		})
	if err != nil {
		err = fmt.Errorf("updating status failed with %w", err)
		if updateErr == nil {
			updateErr = err
		} else {
			updateErr = fmt.Errorf("%w, %w", updateErr, err)
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
	vcVM *object.VirtualMachine) (bool, error) {

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

		return false, err
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
	configSpec vimtypes.VirtualMachineConfigSpec) (bool, error) {

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

				return false, err
			}
		}
	}

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
			"The specified guest ID value is not supported: %s", configSpec.GuestId,
		)
		return
	}

	conditions.Delete(vm, vmopv1.GuestIDReconfiguredCondition)
}
