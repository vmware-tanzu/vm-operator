// Copyright (c) 2024 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// OverwriteResizeConfigSpec applies any set fields in the VM Spec to the ConfigSpec.
func OverwriteResizeConfigSpec(
	_ context.Context,
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) error {

	if adv := vm.Spec.Advanced; adv != nil {
		ptr.OverwriteWithUser(&cs.ChangeTrackingEnabled, adv.ChangeBlockTracking, ci.ChangeTrackingEnabled)
	}

	overrideExtraConfig(vm, ci, cs)

	return nil
}

func overrideExtraConfig(
	vm vmopv1.VirtualMachine,
	ci vimtypes.VirtualMachineConfigInfo,
	cs *vimtypes.VirtualMachineConfigSpec) {

	var toMerge []vimtypes.BaseOptionValue

	toMerge = append(toMerge, overrideMMIOSize(vm, ci, cs)...)

	cs.ExtraConfig = util.OptionValues(cs.ExtraConfig).Merge(toMerge...)
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
