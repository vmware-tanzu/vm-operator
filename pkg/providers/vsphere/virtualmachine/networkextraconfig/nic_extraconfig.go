// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig

import (
	"context"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

// NICSpecBagKeys returns the set of valid bag-key names in iface's
// AdvancedProperties, skipping first-class VMXNet3 keys.
func NICSpecBagKeys(ctx context.Context, iface vmopv1.VirtualMachineNetworkInterfaceSpec) map[string]bool {
	keys := make(map[string]bool)
	for _, kv := range iface.AdvancedProperties {
		if vmopv1util.IsFirstClassVMXnet3NICKey(kv.Key) {
			pkglog.FromContextOrDefault(ctx).Info(
				"AdvancedProperties contains a first-class NIC key; skipping",
				"interfaceName", iface.Name,
				"key", kv.Key)
			continue
		}
		keys[kv.Key] = true
	}
	return keys
}

// DesiredNICExtraConfig assembles this NIC's desired ExtraConfig entries:
// first-class VMXNet3 translations plus user bag keys (namespaced under this
// NIC's ethernetN prefix), with "" clear entries for previously-managed bag
// keys no longer requested.
//
// managedKeys is this NIC's previously-tracked bag-key set (see
// extraconfig.LoadDeviceManagedKeys), used to detect and clear bag keys
// removed from spec.
func DesiredNICExtraConfig(
	ctx context.Context,
	iface vmopv1.VirtualMachineNetworkInterfaceSpec,
	devKey int32,
	managedKeys []string,
) pkgutil.OptionValues {
	prefix := vmopv1util.EthernetExtraConfigPrefix(devKey)
	specBagKeys := NICSpecBagKeys(ctx, iface)

	var desired pkgutil.OptionValues
	if iface.VMXNet3 != nil {
		desired = append(desired, extraconfig.TranslateVMXNet3NICFirstClass(ctx, devKey, iface.VMXNet3)...)
	}

	for _, kv := range iface.AdvancedProperties {
		if specBagKeys[kv.Key] {
			desired = append(desired, &vimtypes.OptionValue{Key: prefix + kv.Key, Value: kv.Value})
		}
	}
	for _, mk := range managedKeys {
		if !specBagKeys[mk] {
			desired = append(desired, &vimtypes.OptionValue{Key: prefix + mk, Value: ""})
		}
	}

	return desired
}

// NICExtraConfigDiff computes the desired-vs-observed ExtraConfig diff for
// the assembled per-NIC overlay and classifies it by vmxmode, shared by the
// config-mutation path (pkg/vmconfig/networkextraconfig.Reconcile) and the
// status/condition path (reconcileStatusNetworkExtraConfig in
// pkg/providers/vsphere/vmlifecycle).
//
// existingEC is any already-assembled ExtraConfig from other reconcilers/NICs
// to merge onto before diffing (e.g. configSpec.ExtraConfig, so the config
// path can still submit a single Reconfigure covering every reconciler's
// changes); pass nil to scope the diff to only overlay's own keys, as the
// status path does so an unrelated reconciler's pending change can't be
// mistaken for a NIC ExtraConfig mismatch.
func NICExtraConfigDiff(
	ctx context.Context,
	observed pkgutil.OptionValues,
	overlay pkgutil.OptionValues,
	existingEC pkgutil.OptionValues,
	poweredOn bool,
) (applied pkgutil.OptionValues, deferred []string, powerCyclePending bool) {

	merged := existingEC.Merge(overlay...)
	semanticResult := extraconfig.VMXNet3SemanticDiff(ctx, observed, merged, vmopv1util.VMXNet3NICKeyMap())

	modeMap := vmopv1util.VMXNet3NICModeMap()
	for _, entry := range semanticResult {
		kv := entry.GetOptionValue()
		if kv == nil {
			continue
		}
		// Normalize the live key (e.g. "ethernet0.ctxPerDev") to its template
		// form ("ethernet%d.ctxPerDev") for mode map lookup.
		mode, isFC := modeMap[vmopv1util.NormalizeEthernetDeviceKey(kv.Key)]
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
