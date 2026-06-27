// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig

import (
	"context"
	"reflect"
	"strings"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// LoadVMManagedKeys parses the comma-separated managed-keys entry from the
// observed VM-level ExtraConfig. Returns nil if the key is absent or empty.
func LoadVMManagedKeys(observed pkgutil.OptionValues) []string {
	return loadManagedKeys(observed, vsphereconst.ExtraConfigManagedKeysKey)
}

// LoadDeviceManagedKeys parses the managed-keys entry from observed ExtraConfig
// for the given managedKeysKey (e.g. fmt.Sprintf(NICExtraConfigManagedKeysKeyFmt, idx)).
// Returns nil if the entry is absent or empty.
func LoadDeviceManagedKeys(observed pkgutil.OptionValues, managedKeysKey string) []string {
	return loadManagedKeys(observed, managedKeysKey)
}

// loadManagedKeys parses a comma-separated list of bare key names from the
// ExtraConfig entry at key. Returns nil if the entry is absent or empty.
func loadManagedKeys(observed pkgutil.OptionValues, key string) []string {
	raw, ok := observed.GetString(key)
	if !ok || raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// SemanticDiff filters the assembled configSpec ExtraConfig before it is sent
// to vSphere, dropping entries that are semantically identical to what the VM
// already has. It is the final deduplication step before configSpec is submitted.
//
// For first-class keys, both values are decoded to Go types before comparison so
// that ESXi string normalization differences (e.g. "true" vs "TRUE") are not
// treated as changes. Other keys use plain string equality.
func SemanticDiff(
	ctx context.Context,
	observed pkgutil.OptionValues,
	merged pkgutil.OptionValues,
) pkgutil.OptionValues {
	return semanticDiff[vmopv1.VirtualMachineAdvancedSpec](
		ctx, observed, merged, vmopv1util.AdvancedVMXKeyMap(),
		func(k string) string { return k })
}

// VMXNet3SemanticDiff filters assembled VMXNet3 NIC ExtraConfig entries before
// submission, dropping entries semantically identical to what the VM already has.
//
// keyMap must contain template-form VMX keys (e.g. "ethernet%d.ctxPerDev"),
// as returned by VMXNet3NICKeyMap(). Live keys in merged (e.g.
// "ethernet0.ctxPerDev") are normalized to their template form before lookup.
// Keys absent from keyMap are treated as non-first-class (plain string
// comparison).
func VMXNet3SemanticDiff(
	ctx context.Context,
	observed pkgutil.OptionValues,
	merged pkgutil.OptionValues,
	keyMap map[string]int,
) pkgutil.OptionValues {
	return semanticDiff[vmopv1.VirtualMachineNetworkInterfaceVMXNet3Spec](
		ctx, observed, merged, keyMap, vmopv1util.NormalizeEthernetDeviceKey)
}

// semanticDiff is the generic implementation shared by SemanticDiff and
// VMXNet3SemanticDiff. T is the spec struct type whose fields are used for semantic
// comparison of first-class keys. keyMap maps VMX key strings to field indices
// in T. keyFromLive maps a live ExtraConfig key to the form used as map keys
// (identity for VM-level fields; NormalizeEthernetDeviceKey for NIC fields).
func semanticDiff[T any](
	ctx context.Context,
	observed pkgutil.OptionValues,
	merged pkgutil.OptionValues,
	keyMap map[string]int,
	keyFromLive func(string) string,
) pkgutil.OptionValues {

	if len(merged) == 0 {
		return nil
	}

	log := pkglog.FromContextOrDefault(ctx)
	structType := reflect.TypeOf(*new(T))

	var out pkgutil.OptionValues
	for _, entry := range merged {
		kv := entry.GetOptionValue()
		if kv == nil {
			continue
		}

		fieldIdx, isFirstClass := keyMap[keyFromLive(kv.Key)]
		if !isFirstClass {
			// Non-first-class key: plain string comparison.
			observedStr, isObserved := observed.GetString(kv.Key)
			desiredStr, _ := kv.Value.(string)
			if !isObserved || observedStr != desiredStr {
				out = append(out, &vimtypes.OptionValue{Key: kv.Key, Value: kv.Value})
			}
			continue
		}

		// First-class key: decode both sides to Go types and compare semantically.
		fieldType := structType.Field(fieldIdx).Type
		desiredStr, _ := kv.Value.(string)
		observedStr, isObserved := observed.GetString(kv.Key)

		// Reset (empty string): only emit if the key is actually present in observed.
		if desiredStr == "" {
			if isObserved {
				out = append(out, &vimtypes.OptionValue{Key: kv.Key, Value: kv.Value})
			}
			continue
		}

		decodedDesired := reflect.New(fieldType).Elem()
		if err := vmopv1util.DecodeVMXFieldValue(ctx, decodedDesired, desiredStr); err != nil {
			log.V(1).Error(err, "unsupported field type for VMX decode; treating desired as zero-value",
				"key", kv.Key)
		}
		desiredValue := decodedDesired.Interface()

		decodedObserved := reflect.New(fieldType).Elem()
		if isObserved && observedStr != "" {
			if err := vmopv1util.DecodeVMXFieldValue(ctx, decodedObserved, observedStr); err != nil {
				log.V(1).Error(err, "unsupported field type for VMX decode; treating observed as zero",
					"key", kv.Key)
			}
		}
		observedValue := decodedObserved.Interface()

		if !isObserved || !reflect.DeepEqual(desiredValue, observedValue) {
			out = append(out, &vimtypes.OptionValue{Key: kv.Key, Value: kv.Value})
		}
	}
	return out
}
