// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util"
)

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
