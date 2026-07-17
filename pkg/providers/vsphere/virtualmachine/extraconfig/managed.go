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

// SpecBagKeys returns the set of valid bag-key names in advanced.ExtraConfig,
// skipping system-reserved and first-class keys. Returns an empty (non-nil)
// map when advanced is nil.
func SpecBagKeys(advanced *vmopv1.VirtualMachineAdvancedSpec) map[string]bool {
	keys := make(map[string]bool)
	if advanced == nil {
		return keys
	}

	firstClassKeys := vmopv1util.AdvancedVMXKeyMap()
	for _, kv := range advanced.ExtraConfig {
		if vmopv1util.IsSystemReservedExtraConfigKey(kv.Key) {
			continue
		}
		if _, isFC := firstClassKeys[kv.Key]; isFC {
			continue
		}
		keys[kv.Key] = true
	}
	return keys
}

// DesiredVMExtraConfig assembles the full desired set of VM-level ExtraConfig
// entries for spec.advanced: first-class VMX translations (including ""
// clear-if-present entries for nil/zero fields) plus user bag keys, with ""
// clear entries for previously-managed bag keys no longer in spec.
//
// managedKeys is the previously-tracked bag-key set (see LoadVMManagedKeys),
// used to detect and clear bag keys that were removed from spec.
func DesiredVMExtraConfig(
	ctx context.Context,
	advanced *vmopv1.VirtualMachineAdvancedSpec,
	managedKeys []string,
) pkgutil.OptionValues {
	if advanced == nil {
		advanced = &vmopv1.VirtualMachineAdvancedSpec{}
	}

	specBagKeys := SpecBagKeys(advanced)
	var desired pkgutil.OptionValues

	for _, kv := range advanced.ExtraConfig {
		if specBagKeys[kv.Key] {
			desired = append(desired, &vimtypes.OptionValue{Key: kv.Key, Value: kv.Value})
		}
	}
	for _, mk := range managedKeys {
		if !specBagKeys[mk] {
			desired = append(desired, &vimtypes.OptionValue{Key: mk, Value: ""})
		}
	}

	return append(desired, TranslateFirstClass(ctx, advanced)...)
}

// RouteByVMXMode routes a semantic diff (see SemanticDiff) by vmxmode:
// PowerOff-mode entries cannot be applied while the VM is powered on, so they
// are excluded from applied and returned in deferred instead. PowerCycle-mode
// entries are included in applied but flagged via powerCyclePending, since
// they only take effect after a guest power cycle. Non-first-class entries
// (e.g. bag keys) are always included in applied.
func RouteByVMXMode(
	diff pkgutil.OptionValues,
	modeMap map[string]vmopv1util.VMXMode,
	poweredOn bool,
) (applied pkgutil.OptionValues, deferred []string, powerCyclePending bool) {

	for _, entry := range diff {
		kv := entry.GetOptionValue()
		if kv == nil {
			continue
		}
		mode, isFC := modeMap[kv.Key]
		switch {
		case isFC && mode == vmopv1util.VMXModePowerOff && poweredOn:
			deferred = append(deferred, kv.Key)
		default:
			applied = append(applied, entry)
			if isFC && mode == vmopv1util.VMXModePowerCycle && poweredOn {
				powerCyclePending = true
			}
		}
	}
	return applied, deferred, powerCyclePending
}

// VMExtraConfigDiff computes the desired-vs-observed diff for spec.advanced's
// ExtraConfig and classifies it by vmxmode, shared by the config-mutation
// path (Reconcile) and the status/condition path (reconcileStatusExtraConfig
// in pkg/providers/vsphere/vmlifecycle).
//
// existingEC is any already-assembled ExtraConfig from other reconcilers to
// merge onto before diffing (e.g. configSpec.ExtraConfig, so the config path
// can still submit a single Reconfigure covering every reconciler's changes);
// pass nil to scope the diff to only spec.advanced's own keys, as the status
// path does so an unrelated reconciler's pending change can't be mistaken for
// an ExtraConfig mismatch.
func VMExtraConfigDiff(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	observed pkgutil.OptionValues,
	managedKeys []string,
	existingEC pkgutil.OptionValues,
) (applied pkgutil.OptionValues, deferred []string, powerCyclePending bool) {
	desired := DesiredVMExtraConfig(ctx, vm.Spec.Advanced, managedKeys)
	semanticResult := SemanticDiff(ctx, observed, existingEC.Merge(desired...))
	poweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn
	return RouteByVMXMode(semanticResult, vmopv1util.AdvancedVMXModeMap(), poweredOn)
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
