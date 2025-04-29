// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// OverwriteResizeConfigSpec applies any set fields in the VM Spec or changes required from
// the current VM state to the ConfigSpec. These are fields that we can change without the
// VM Class.
func OverwriteResizeConfigSpec(
	ctx context.Context,
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) error {

	if err := OverwriteAlwaysResizeConfigSpec(ctx, vm, ci, cs); err != nil {
		return err
	}

	if adv := vm.Spec.Advanced; adv != nil {
		ptr.OverwriteWithUser(&cs.ChangeTrackingEnabled, adv.ChangeBlockTracking, ci.ChangeTrackingEnabled)
	}

	overwriteGuestID(vm, ci, cs)
	overwriteExtraConfig(vm, ci, cs)
	ReconcileNetworkDeviceConnectionState(&ci, cs)

	return nil
}

// OverwriteAlwaysResizeConfigSpec applies any set fields in the VM Spec or
// changes required from the current VM state to the ConfigSpec. These are
// fields that change without the VM Class.
func OverwriteAlwaysResizeConfigSpec(
	_ context.Context,
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) error {

	overwriteManagedBy(vm, ci, cs)
	overwriteExtraConfigNamespaceName(vm, ci, cs)

	return nil
}

func overwriteGuestID(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) {

	// After the VM has been created, don't use the VM Class ConfigSpec's GuestID.
	// Only update if the VM Spec.GuestID is set. Note GuestID is not a part of
	// CreateResizeConfigSpec() so it should always already be empty here.
	cs.GuestId = ""

	overwrite(&cs.GuestId, vm.Spec.GuestID, ci.GuestId)
}

func overwriteExtraConfig(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) {

	var toMerge []vimtypes.BaseOptionValue

	toMerge = append(toMerge, ensureNamespaceName(vm, ci, cs)...)
	toMerge = append(toMerge, overrideMMIOSize(vm, ci, cs)...)
	toMerge = append(toMerge, clearMMPowerOffEC(vm, ci, cs)...)
	toMerge = append(toMerge, updateV1Alpha1CompatibleEC(vm, ci, cs)...)

	cs.ExtraConfig = util.OptionValues(cs.ExtraConfig).Merge(toMerge...)
}

func overwriteExtraConfigNamespaceName(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) {

	var toMerge []vimtypes.BaseOptionValue

	toMerge = append(toMerge, ensureNamespaceName(vm, ci, cs)...)

	cs.ExtraConfig = util.OptionValues(cs.ExtraConfig).Merge(toMerge...)
}

func overwriteManagedBy(
	_ vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) {

	var current vimtypes.ManagedByInfo
	if ci.ManagedBy != nil {
		current = *ci.ManagedBy
	}

	if cs.ManagedBy == nil {
		cs.ManagedBy = &vimtypes.ManagedByInfo{}
	}

	user := vimtypes.ManagedByInfo{
		ExtensionKey: vmopv1.ManagedByExtensionKey,
		Type:         vmopv1.ManagedByExtensionType,
	}

	overwrite(cs.ManagedBy, user, current)

	var empty vimtypes.ManagedByInfo
	if *cs.ManagedBy == empty {
		cs.ManagedBy = nil
	}
}

func ensureNamespaceName(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) []vimtypes.BaseOptionValue {

	outEC := []vimtypes.BaseOptionValue{}
	curEC := util.OptionValues(ci.ExtraConfig).StringMap()
	inEC := util.OptionValues(cs.ExtraConfig).StringMap()

	key := constants.ExtraConfigVMServiceNamespacedName
	val := vm.NamespacedName()
	if val == "/" {
		val = ""
	}

	// Does the VM have the key set in EC?
	if v, ok := curEC[key]; ok {
		if v == val {
			// The key is present and correct; is the ConfigSpec trying to
			// set it again?
			if _, ok := inEC[key]; ok {
				// Remove the entry from the ConfigSpec.
				cs.ExtraConfig = util.OptionValues(cs.ExtraConfig).Delete(key)
			}
		} else {
			// The key is present but incorrect.
			outEC = append(outEC, &vimtypes.OptionValue{
				Key:   key,
				Value: val,
			})
		}
	} else {
		// The key is not present.
		outEC = append(outEC, &vimtypes.OptionValue{
			Key:   key,
			Value: val,
		})
	}

	return outEC
}

func overrideMMIOSize(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) []vimtypes.BaseOptionValue {

	// TODO: This is essentially what the old code checked and should be OK for most situations but
	//  can be improved: we might be removing the existing passthru devices so we wouldn't really
	//  need to set this (and maybe remove the EC fields instead).
	if !hasvGPUOrDDPIODevicesInVM(ci) && !util.HasVirtualPCIPassthroughDeviceChange(cs.DeviceChange) {
		return nil
	}

	mmIOSize := vm.Annotations[constants.PCIPassthruMMIOOverrideAnnotation]
	if mmIOSize == "" {
		mmIOSize = constants.PCIPassthruMMIOSizeDefault
	}
	if mmIOSize == "0" {
		return nil
	}

	mmioEC := []vimtypes.BaseOptionValue{
		&vimtypes.OptionValue{Key: constants.PCIPassthruMMIOSizeExtraConfigKey, Value: mmIOSize},
		&vimtypes.OptionValue{Key: constants.PCIPassthruMMIOExtraConfigKey, Value: constants.ExtraConfigTrue},
	}

	var out []vimtypes.BaseOptionValue //nolint:prealloc
	curEC := util.OptionValues(ci.ExtraConfig)

	for _, ov := range mmioEC {
		k, v := ov.GetOptionValue().Key, ov.GetOptionValue().Value

		if vv, ok := curEC.GetString(k); ok && v == vv {
			// Current value is already the desired value. Remove any update.
			cs.ExtraConfig = util.OptionValues(cs.ExtraConfig).Delete(k)
			continue
		}

		out = append(out, ov)
	}

	return out
}

func clearMMPowerOffEC(
	_ vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	_ *vimtypes.VirtualMachineConfigSpec) []vimtypes.BaseOptionValue {

	// Ensure MMPowerOffVMExtraConfigKey is no longer part of ExtraConfig as
	// setting it to an empty value removes it.

	v, ok := util.OptionValues(ci.ExtraConfig).GetString(constants.MMPowerOffVMExtraConfigKey)
	if !ok || v == "" {
		return nil
	}

	return []vimtypes.BaseOptionValue{
		&vimtypes.OptionValue{Key: constants.MMPowerOffVMExtraConfigKey, Value: ""},
	}
}

func updateV1Alpha1CompatibleEC(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	_ *vimtypes.VirtualMachineConfigSpec) []vimtypes.BaseOptionValue {

	// This special EC field was just for a handful of custom spun images that used the
	// old v1a1 OvfEnv bootstrap method.
	bs := vm.Spec.Bootstrap
	if bs == nil || bs.LinuxPrep == nil || bs.VAppConfig == nil {
		return nil
	}

	v, ok := util.OptionValues(ci.ExtraConfig).GetString(constants.VMOperatorV1Alpha1ExtraConfigKey)
	if !ok || v != constants.VMOperatorV1Alpha1ConfigReady {
		return nil
	}

	return []vimtypes.BaseOptionValue{
		&vimtypes.OptionValue{Key: constants.VMOperatorV1Alpha1ExtraConfigKey, Value: constants.VMOperatorV1Alpha1ConfigEnabled},
	}
}

// ReconcileNetworkDeviceConnectionState ensures the VMs network interface
// devices are connected and start connected. Please note, this function should
// only modify the configSpec if:
//   - there is a network device in configInfo that is not connected or does not
//     start connected and is not being removed in the configSpec.
//   - there is a new network device in configSpec (i.e. it has a negative
//     device key) and is not marked connected or not marked as starting
//     connected.
//
// The ci parameter may be nil when this function is used to ensure the correct
// behavior for a ConfigSpec used to create a new VM.
func ReconcileNetworkDeviceConnectionState(
	ci *vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) {

	// Get a list of current NICs.
	curDevLst := []*vimtypes.VirtualEthernetCard{}
	curDevMap := map[int32]*vimtypes.VirtualEthernetCard{}
	if ci != nil {
		for i := range ci.Hardware.Device {
			if bd, ok := ci.Hardware.Device[i].(vimtypes.BaseVirtualEthernetCard); ok {
				d := bd.GetVirtualEthernetCard()
				curDevMap[d.Key] = d
				curDevLst = append(curDevLst, d)
			}
		}
	}

	// Parse the ConfigSpec for new/modified NICs.
	modDevs := map[int32]*vimtypes.VirtualEthernetCard{}
	if cs != nil {
		for i := range cs.DeviceChange {
			ds := cs.DeviceChange[i].GetVirtualDeviceConfigSpec()
			if bd, ok := ds.Device.(vimtypes.BaseVirtualEthernetCard); ok {
				d := bd.GetVirtualEthernetCard()
				if _, ok := curDevMap[d.Key]; ok {
					//
					// NIC already exists.
					//
					if ds.Operation == vimtypes.VirtualDeviceConfigSpecOperationRemove {
						//
						// NIC is being removed.
						//
						delete(curDevMap, d.Key)
					} else {
						//
						// NIC is being updated.
						//
						modDevs[d.Key] = d
					}
				} else {
					//
					// NIC is being added, ensure it is connected.
					//
					if d.Connectable == nil {
						d.Connectable = &vimtypes.VirtualDeviceConnectInfo{}
					}
					d.Connectable.StartConnected = true
					d.Connectable.Connected = true
				}
			}
		}
	}

	// Ensure all current NICs are connected and start connected.
	for _, d := range curDevLst {
		if _, ok := curDevMap[d.Key]; ok {

			if d.Connectable == nil ||
				!d.Connectable.Connected ||
				!d.Connectable.StartConnected {

				connectable := d.Connectable
				if connectable == nil {
					connectable = &vimtypes.VirtualDeviceConnectInfo{}
				}
				connectable.Connected = true
				connectable.StartConnected = true

				// The NIC is either not connected or not set to start
				// connected.
				if md, ok := modDevs[d.Key]; ok {
					// The NIC is already part of the ConfigSpec, so update
					// it there so its connection state information is
					// correct.
					md.Connectable = connectable
				} else if cs != nil {
					// Add the NIC to the ConfigSpec to ensure it is
					// connected and starts connected.
					d.Connectable = connectable
					cs.DeviceChange = append(cs.DeviceChange,
						&vimtypes.VirtualDeviceConfigSpec{
							Operation: vimtypes.VirtualDeviceConfigSpecOperationEdit,
							Device:    d,
						})
				}
			}
		}
	}
}

func hasvGPUOrDDPIODevicesInVM(
	config vimtypes.VirtualMachineConfigInfo) bool {

	if len(util.SelectNvidiaVgpu(config.Hardware.Device)) > 0 {
		return true
	}
	if len(util.SelectDynamicDirectPathIO(config.Hardware.Device)) > 0 {
		return true
	}
	return false
}

func overwrite[T comparable](dst *T, user, current T) {
	if dst == nil {
		panic("dst is nil")
	}

	// Determine what the ultimate desired value is. If set the user
	// value takes precedence.
	var desired, empty T
	switch {
	case user != empty:
		desired = user
	case *dst != empty:
		desired = *dst
	default:
		// Leave *dst as-is.
		return
	}

	if current == empty || current != desired {
		// An update is required to the desired value.
		*dst = desired
	} else if current == desired {
		// Already at the desired value so no update is required.
		*dst = empty
	}
}
