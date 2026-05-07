// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// BackfillExtraConfigFromMoVM populates spec.advanced.* vmx-tagged fields and
// spec.network.interfaces[i].vmxnet3.* from moVM.Config.ExtraConfig.
//
// Only nil/zero spec fields are written; existing values are left unchanged so
// that spec wins over the live vSphere state (drift case). Keys not matching a
// vmx-tagged first-class field are silently dropped — this allowlist prevents
// vSphere bookkeeping keys from polluting the spec. No entries are appended to
// spec.advanced.ExtraConfig or interfaces[i].advancedProperties.
//
// Called once per VM during schema upgrade when FeatureVersionTelcoVMServiceAPI
// is being set. Returns true if any field was mutated.
func BackfillExtraConfigFromMoVM(
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (bool, error) {

	if moVM.Config == nil {
		return false, nil
	}

	mutated := false

	if m, err := backfillAdvancedSpec(vm, moVM.Config.ExtraConfig); err != nil {
		return false, err
	} else if m {
		mutated = true
	}

	if vm.Spec.Network != nil {
		ethDevs := collectEthernetDevicesFromMoVM(moVM)
		// Spec interfaces are zipped by position to ethernet devices (same
		// convention as FillEmptyNetworkInterfaceTypesFromMoVM). The extraConfig
		// key index X in ethernetX.* is derived from the device key
		// (X = deviceKey - 4000), not from the spec array position.
		// TODO: proper device-to-spec matching beyond positional zip.
		for i := range vm.Spec.Network.Interfaces {
			if i >= len(ethDevs) {
				break
			}
			devKey := ethDevs[i].GetVirtualDevice().Key
			if m, err := backfillNICSpec(
				vmopv1util.EthernetExtraConfigPrefix(devKey),
				&vm.Spec.Network.Interfaces[i],
				moVM.Config.ExtraConfig); err != nil {
				return false, err
			} else if m {
				mutated = true
			}
		}
	}

	return mutated, nil
}

func backfillAdvancedSpec(
	vm *vmopv1.VirtualMachine,
	extraConfig []vimtypes.BaseOptionValue) (bool, error) {

	mutated := false
	for _, bov := range extraConfig {
		ov, ok := bov.(*vimtypes.OptionValue)
		if !ok {
			continue
		}
		raw, ok := ov.Value.(string)
		if !ok {
			continue
		}

		fieldIdx, exists := vmopv1util.AdvancedVMXKeyMap()[ov.Key]
		if !exists {
			continue
		}

		// Spec wins: skip if the field is already non-zero.
		if vm.Spec.Advanced != nil {
			rv := reflect.ValueOf(vm.Spec.Advanced).Elem().Field(fieldIdx)
			if !rv.IsZero() {
				continue
			}
		}

		if vm.Spec.Advanced == nil {
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
		}

		rv := reflect.ValueOf(vm.Spec.Advanced).Elem().Field(fieldIdx)
		if err := vmopv1util.DecodeVMXFieldValue(rv, raw); err != nil {
			return false, fmt.Errorf("decode vmx field %q: %w", ov.Key, err)
		}
		mutated = true
	}
	return mutated, nil
}

func backfillNICSpec(
	prefix string,
	iface *vmopv1.VirtualMachineNetworkInterfaceSpec,
	extraConfig []vimtypes.BaseOptionValue) (bool, error) {

	// Only backfill vmxnet3 fields for VMXNet3 (or type-unset) NICs.
	if iface.Type != "" &&
		iface.Type != vmopv1.VirtualMachineNetworkInterfaceTypeVMXNet3 {
		return false, nil
	}
	mutated := false

	for _, bov := range extraConfig {
		ov, ok := bov.(*vimtypes.OptionValue)
		if !ok {
			continue
		}

		rest, found := strings.CutPrefix(ov.Key, prefix)
		if !found {
			continue
		}

		raw, ok := ov.Value.(string)
		if !ok {
			continue
		}

		fieldIdx, exists := vmopv1util.NICVMXKeyMap()[rest]
		if !exists {
			continue // not in our vmxnet3 allowlist, drop
		}

		// Spec wins: skip if the field is already non-zero.
		if iface.VMXNet3 != nil {
			rv := reflect.ValueOf(iface.VMXNet3).Elem().Field(fieldIdx)
			if !rv.IsZero() {
				continue
			}
		}

		if iface.VMXNet3 == nil {
			iface.VMXNet3 = &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec{}
		}

		rv := reflect.ValueOf(iface.VMXNet3).Elem().Field(fieldIdx)
		if err := vmopv1util.DecodeVMXFieldValue(rv, raw); err != nil {
			return false, fmt.Errorf("decode vmx nic field %q: %w", ov.Key, err)
		}
		mutated = true
	}
	return mutated, nil
}
