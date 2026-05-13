// Copyright (c) 2026 Broadcom. All Rights Reserved.
// Broadcom Confidential. The term "Broadcom" refers to Broadcom Inc.
// and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package backfill

import (
	"context"
	"fmt"
	"reflect"

	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// ExtraConfigFromMoVM populates spec.advanced.* vmx-tagged fields
// from moVM.Config.ExtraConfig.
//
// Only nil/zero spec fields are written; existing values are left unchanged so
// that spec wins over the live vSphere state (drift case). Keys not matching a
// vmx-tagged first-class field are silently dropped — this allowlist prevents
// vSphere bookkeeping keys from polluting the spec. No entries are appended to
// spec.advanced.ExtraConfig.
//
// Host-default sentinels (auto, default, dontcare) leave the corresponding
// spec field nil, implementing the nil=auto convention: nil means "let the
// hypervisor decide."
//
// Called once per VM during schema upgrade when FeatureVersionTelcoVMServiceAPI
// is being set. Returns true if any field was mutated.
func ExtraConfigFromMoVM(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine) (bool, error) {

	if moVM.Config == nil {
		return false, nil
	}

	return backfillAdvancedSpec(ctx, vm, moVM.Config.ExtraConfig)
}

func backfillAdvancedSpec(
	ctx context.Context,
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

		// Decode into a advancedSpec struct to avoid initialising vm.Spec.Advanced
		// prematurely. If the raw value is a host-default sentinel (auto,
		// default, dontcare) the decoded field stays zero and we skip without
		// touching spec — preserving the nil=auto convention for *bool fields.
		var advancedSpec vmopv1.VirtualMachineAdvancedSpec
		advancedSpecFieldValue := reflect.ValueOf(&advancedSpec).Elem().Field(fieldIdx)
		if err := vmopv1util.DecodeVMXFieldValue(ctx, advancedSpecFieldValue, raw); err != nil {
			return false, fmt.Errorf("decode vmx field %q: %w", ov.Key, err)
		}
		if advancedSpecFieldValue.IsZero() {
			continue
		}

		if vm.Spec.Advanced == nil {
			vm.Spec.Advanced = &vmopv1.VirtualMachineAdvancedSpec{}
		}
		reflect.ValueOf(vm.Spec.Advanced).Elem().Field(fieldIdx).Set(advancedSpecFieldValue)
		mutated = true
	}
	return mutated, nil
}
