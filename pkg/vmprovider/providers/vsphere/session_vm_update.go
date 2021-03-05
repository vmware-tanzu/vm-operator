// Copyright (c) 2018-2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vsphere

import (
	"context"
	"reflect"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/vmware-tanzu/vm-operator-api/api/v1alpha1"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/task"
	vimTypes "github.com/vmware/govmomi/vim25/types"
	apiEquality "k8s.io/apimachinery/pkg/api/equality"

	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider"
	res "github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere/resources"
)

func (s *Session) createNetworkDevices(vmCtx VMContext) ([]vimTypes.BaseVirtualDevice, error) {
	// This negative device key is the traditional range used for network interfaces.
	deviceKey := int32(-100)
	devices := make([]vimTypes.BaseVirtualDevice, 0, len(vmCtx.VM.Spec.NetworkInterfaces))

	for i := range vmCtx.VM.Spec.NetworkInterfaces {
		vif := vmCtx.VM.Spec.NetworkInterfaces[i]

		np, err := GetNetworkProvider(&vif, s.k8sClient, s.Client.VimClient(), s.Finder, s.cluster, s.scheme)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get network provider")
		}

		dev, err := np.CreateVnic(vmCtx, vmCtx.VM, &vif)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create vnic '%v'", vif)
		}

		nic := dev.(vimTypes.BaseVirtualEthernetCard).GetVirtualEthernetCard()
		nic.Key = deviceKey
		devices = append(devices, dev)

		deviceKey--
	}

	return devices, nil
}

func (s *Session) reconcileVMNicDeviceChanges(
	vmCtx VMUpdateContext,
	existingNICs object.VirtualDeviceList) ([]vimTypes.BaseVirtualDeviceConfigSpec, error) {

	// Assume this is the special condition in cloneVMNicDeviceChanges(), instead of actually
	// wanting to remove all the interfaces. This is a hack that we should address later.
	if len(vmCtx.VM.Spec.NetworkInterfaces) == 0 {
		return nil, nil
	}

	newNetDevices, err := s.createNetworkDevices(vmCtx.VMContext)
	if err != nil {
		return nil, err
	}

	var deviceChanges []vimTypes.BaseVirtualDeviceConfigSpec

	for _, newDev := range newNetDevices {
		newNic := newDev.(vimTypes.BaseVirtualEthernetCard)
		newNicExternalID := newNic.GetVirtualEthernetCard().ExternalId
		newNicBacking := newNic.GetVirtualEthernetCard().Backing
		newNicBackingType := reflect.TypeOf(newNicBacking)

		var matchingIdx = -1

		// Try to match the expected NIC with an existing NIC but this isn't that great. We mostly
		// depend on the backing but we can improve that later on. When not generated, we could use
		// the MAC address. When we support something other than just vmxnet3 we should compare
		// those types too. And we should make this truly reconcile as well by comparing the full
		// state (support EDIT instead of only ADD/REMOVE operations).
		for idx, curDev := range existingNICs {
			nic := curDev.(vimTypes.BaseVirtualEthernetCard)

			// Note only NCP sets the ExternalID.
			if newNicExternalID != "" {
				if nic.GetVirtualEthernetCard().ExternalId == newNicExternalID {
					// Assume everything else matches, but we should improve this later on.
					matchingIdx = idx
					break
				}
			}

			// This assumes we don't have multiple NICs in the same backing network. This is kind of, sort
			// of enforced by the webhook, but we lack a guaranteed way to match up the NICs.

			db := nic.GetVirtualEthernetCard().Backing
			if db == nil || reflect.TypeOf(db) != newNicBackingType {
				continue
			}

			var backingMatch bool

			// Cribbed from VirtualDeviceList.SelectByBackingInfo().
			switch a := db.(type) {
			case *vimTypes.VirtualEthernetCardNetworkBackingInfo:
				// Used only in testing.
				b := newNicBacking.(*vimTypes.VirtualEthernetCardNetworkBackingInfo)
				backingMatch = a.DeviceName == b.DeviceName
			case *vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo:
				b := newNicBacking.(*vimTypes.VirtualEthernetCardDistributedVirtualPortBackingInfo)
				backingMatch = a.Port.SwitchUuid == b.Port.SwitchUuid && a.Port.PortgroupKey == b.Port.PortgroupKey
			case *vimTypes.VirtualEthernetCardOpaqueNetworkBackingInfo:
				b := newNicBacking.(*vimTypes.VirtualEthernetCardOpaqueNetworkBackingInfo)
				backingMatch = a.OpaqueNetworkId == b.OpaqueNetworkId
			}

			if backingMatch {
				matchingIdx = idx
				break
			}
		}

		if matchingIdx == -1 {
			// No matching backing found so add new NIC.
			deviceChanges = append(deviceChanges, &vimTypes.VirtualDeviceConfigSpec{
				Device:    newDev,
				Operation: vimTypes.VirtualDeviceConfigSpecOperationAdd,
			})
		} else {
			// Matching backing found so keep this NIC (don't remove it below).
			existingNICs = append(existingNICs[:matchingIdx], existingNICs[matchingIdx+1:]...)
		}
	}

	// Remove any unmatched existing interfaces.
	var removeDeviceChanges []vimTypes.BaseVirtualDeviceConfigSpec
	for _, dev := range existingNICs {
		removeDeviceChanges = append(removeDeviceChanges, &vimTypes.VirtualDeviceConfigSpec{
			Device:    dev,
			Operation: vimTypes.VirtualDeviceConfigSpecOperationRemove,
		})
	}

	// Process any removes first.
	return append(removeDeviceChanges, deviceChanges...), nil
}

func isCustomizationPendingError(err error) bool {
	if te, ok := err.(task.Error); ok {
		if _, ok := te.Fault().(*vimTypes.CustomizationPending); ok {
			return true
		}
	}
	return false
}

func (s *Session) customizeVM(vmCtx VMUpdateContext, resVM *res.VirtualMachine) error {
	customizationPending, err := resVM.IsGuestCustomizationPending(vmCtx)
	if err != nil {
		return err
	}

	if customizationPending {
		// TODO: We should consider if we want to clear the pending customization instead
		// since it might be stale.
		return nil
	}

	customizationSpec, err := s.GetCustomizationSpec(vmCtx.VMContext, resVM)
	if err != nil {
		return err
	}

	if customizationSpec != nil {
		vmCtx.Logger.Info("Customizing VM", "customizationSpec", customizationSpec)
		if err := resVM.Customize(vmCtx, *customizationSpec); err != nil {
			// Ideally, IsCustomizationPending() above should ensure that the VM does not have any pending
			// customizations. However, since CustomizationPending fault means that this means the VM will
			// NEVER power on, we explicitly ignore that for extra safety.
			if !isCustomizationPendingError(err) {
				return err
			}

			vmCtx.Logger.Info("Ignoring customization error due to pending guest customization")
		}
	}

	return nil
}

func deltaConfigSpecCPUAllocation(
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

func deltaConfigSpecMemoryAllocation(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmClassSpec *v1alpha1.VirtualMachineClassSpec) {

	memAllocation := config.MemoryAllocation
	var memoryReservation *int64
	var memoryLimit *int64

	if !vmClassSpec.Policies.Resources.Requests.Memory.IsZero() {
		rsv := memoryQuantityToMb(vmClassSpec.Policies.Resources.Requests.Memory)
		if memAllocation == nil || memAllocation.Reservation == nil || *memAllocation.Reservation != rsv {
			memoryReservation = &rsv
		}
	}

	if !vmClassSpec.Policies.Resources.Limits.Memory.IsZero() {
		lim := memoryQuantityToMb(vmClassSpec.Policies.Resources.Limits.Memory)
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

func deltaConfigSpecExtraConfig(
	config *vimTypes.VirtualMachineConfigInfo,
	configSpec *vimTypes.VirtualMachineConfigSpec,
	vmImage *v1alpha1.VirtualMachineImage,
	vmSpec v1alpha1.VirtualMachineSpec,
	vmMetadata *vmprovider.VmMetadata,
	globalExtraConfig map[string]string) {

	renderTemplateFn := func(name, text string) string {
		t, err := template.New(name).Parse(text)
		if err != nil {
			return text
		}
		b := strings.Builder{}
		if err := t.Execute(&b, vmSpec); err != nil {
			return text
		}
		return b.String()
	}

	// Create the merged extra config, and apply the VM Spec template on the values.
	extraConfig := make(map[string]string)
	for k, v := range globalExtraConfig {
		extraConfig[k] = v
	}

	if vmMetadata != nil && vmMetadata.Transport == v1alpha1.VirtualMachineMetadataExtraConfigTransport {
		for k, v := range vmMetadata.Data {
			if strings.HasPrefix(k, ExtraConfigGuestInfoPrefix) {
				extraConfig[k] = v
			}
		}
	}
	for k, v := range extraConfig {
		extraConfig[k] = renderTemplateFn(k, v)
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
		currentExtraConfig[VMOperatorV1Alpha1ExtraConfigKey] == VMOperatorV1Alpha1ConfigReady {
		// Set VMOperatorV1Alpha1ExtraConfigKey for v1alpha1 VirtualMachineImage compatibility.
		configSpec.ExtraConfig = append(
			configSpec.ExtraConfig,
			&vimTypes.OptionValue{Key: VMOperatorV1Alpha1ExtraConfigKey, Value: VMOperatorV1Alpha1ConfigEnabled})
	}
}

func deltaConfigSpecVAppConfig(
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

func deltaConfigSpecChangeBlockTracking(
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

func deltaConfigSpec(
	vmCtx VMContext,
	config *vimTypes.VirtualMachineConfigInfo,
	vmImage *v1alpha1.VirtualMachineImage,
	vmClassSpec v1alpha1.VirtualMachineClassSpec,
	vmMetadata *vmprovider.VmMetadata,
	globalExtraConfig map[string]string,
	minCPUFreq uint64) *vimTypes.VirtualMachineConfigSpec {

	configSpec := &vimTypes.VirtualMachineConfigSpec{}

	if config.Annotation != VCVMAnnotation {
		configSpec.Annotation = VCVMAnnotation
	}
	if nCPUs := int32(vmClassSpec.Hardware.Cpus); config.Hardware.NumCPU != nCPUs {
		configSpec.NumCPUs = nCPUs
	}
	if memMB := memoryQuantityToMb(vmClassSpec.Hardware.Memory); int64(config.Hardware.MemoryMB) != memMB {
		configSpec.MemoryMB = memMB
	}
	if config.ManagedBy == nil {
		configSpec.ManagedBy = &vimTypes.ManagedByInfo{
			ExtensionKey: "com.vmware.vcenter.wcp",
			Type:         "VirtualMachine",
		}
	}

	deltaConfigSpecCPUAllocation(config, configSpec, &vmClassSpec, minCPUFreq)
	deltaConfigSpecMemoryAllocation(config, configSpec, &vmClassSpec)
	deltaConfigSpecExtraConfig(config, configSpec, vmImage, vmCtx.VM.Spec, vmMetadata, globalExtraConfig)
	deltaConfigSpecVAppConfig(config, configSpec, vmMetadata)
	deltaConfigSpecChangeBlockTracking(config, configSpec, vmCtx.VM.Spec)

	return configSpec
}

func (s *Session) poweredOffVMReconfigure(vmCtx VMUpdateContext, resVM *res.VirtualMachine, vmConfigArgs vmprovider.VmConfigArgs) error {
	config, err := resVM.GetConfig(vmCtx)
	if err != nil {
		return err
	}

	// TODO: VirtualDeviceList(config.Hardware.Device) instead
	virtualDevices, err := resVM.GetVirtualDevices(vmCtx)
	if err != nil {
		return err
	}

	virtualDisks := virtualDevices.SelectByType((*vimTypes.VirtualDisk)(nil))
	virtualNICs := virtualDevices.SelectByType((*vimTypes.VirtualEthernetCard)(nil))

	configSpec := deltaConfigSpec(
		vmCtx.VMContext,
		config,
		vmConfigArgs.VmImage,
		vmConfigArgs.VmClass.Spec,
		vmConfigArgs.VmMetadata,
		s.extraConfig,
		s.GetCpuMinMHzInCluster(),
	)

	diskDeviceChanges, err := resizeVirtualDisksDeviceChanges(vmCtx.VMContext, virtualDisks)
	if err != nil {
		return err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, diskDeviceChanges...)

	nicDeviceChanges, err := s.reconcileVMNicDeviceChanges(vmCtx, virtualNICs)
	if err != nil {
		return err
	}
	configSpec.DeviceChange = append(configSpec.DeviceChange, nicDeviceChanges...)

	defaultConfigSpec := &vimTypes.VirtualMachineConfigSpec{}
	if !apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		// TODO: Pretty-print differences
		if err := resVM.Reconfigure(vmCtx, configSpec); err != nil {
			vmCtx.Logger.Error(err, "powered off reconfigure failed")
			return err
		}
	}

	return nil
}

func (s *Session) poweredOnVMReconfigure(vmCtx VMUpdateContext, resVM *res.VirtualMachine) error {
	config, err := resVM.GetConfig(vmCtx)
	if err != nil {
		return err
	}

	configSpec := &vimTypes.VirtualMachineConfigSpec{}
	deltaConfigSpecChangeBlockTracking(config, configSpec, vmCtx.VM.Spec)

	defaultConfigSpec := &vimTypes.VirtualMachineConfigSpec{}
	if !apiEquality.Semantic.DeepEqual(configSpec, defaultConfigSpec) {
		// TODO: Pretty-print differences
		if err := resVM.Reconfigure(vmCtx, configSpec); err != nil {
			vmCtx.Logger.Error(err, "powered on reconfigure failed")
			return err
		}

		// Special case for CBT: in order for CBT change take effect for a powered on VM,
		// a checkpoint save/restore is needed.  tracks the implementation of
		// this FSR internally to vSphere.
		if configSpec.ChangeTrackingEnabled != nil {
			if err := s.invokeFsrVirtualMachine(vmCtx.VMContext, resVM); err != nil {
				vmCtx.Logger.Error(err, "Failed to invoke FSR for CBT update")
				return err
			}
		}
	}

	return nil
}

func (s *Session) UpdateVirtualMachine(
	ctx context.Context,
	vm *v1alpha1.VirtualMachine,
	vmConfigArgs vmprovider.VmConfigArgs) (*res.VirtualMachine, error) {

	vmCtx := VMUpdateContext{
		VMContext: VMContext{
			Context: ctx,
			Logger:  log.WithValues("vmName", vm.NamespacedName()),
			VM:      vm,
		},
	}

	resVM, err := s.GetVirtualMachine(vmCtx.VMContext)
	if err != nil {
		return nil, err
	}

	vmCtx.IsOff, err = resVM.IsVMPoweredOff(ctx)
	if err != nil {
		return nil, err
	}

	switch vmCtx.VM.Spec.PowerState {
	case v1alpha1.VirtualMachinePoweredOff:
		if !vmCtx.IsOff {
			err := resVM.SetPowerState(vmCtx, v1alpha1.VirtualMachinePoweredOff)
			if err != nil {
				return nil, err
			}
		}
		// BMV: We'll likely want to reconfigure a powered off VM too, but right now
		// we'll defer that until the pre power on.

	case v1alpha1.VirtualMachinePoweredOn:
		if vmCtx.IsOff {
			err := s.poweredOffVMReconfigure(vmCtx, resVM, vmConfigArgs)
			if err != nil {
				return nil, err
			}

			err = s.customizeVM(vmCtx, resVM)
			if err != nil {
				return nil, err
			}

			err = resVM.SetPowerState(vmCtx, v1alpha1.VirtualMachinePoweredOn)
			if err != nil {
				return nil, err
			}
		} else {
			err := s.poweredOnVMReconfigure(vmCtx, resVM)
			if err != nil {
				return nil, err
			}
		}
	}

	return resVM, nil
}
