// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

// state is shared between Reconcile and OnResult via context.
type state struct {
	// BlockedPowerOff holds fields not applied because the VM is powered on
	// and the field is not hot-pluggable.
	BlockedPowerOff []string
	// PowerCyclePending is true when vmx.reboot.powerCycle was injected.
	PowerCyclePending bool
	// Blocked holds fields skipped due to unmet prerequisites (hwVer, EFI,
	// vNUMA topology). Each entry embeds a per-field reason.
	Blocked []string
}

type reconciler struct{}

var _ vmconfig.ReconcilerWithContext = reconciler{}

// New returns a new ReconcilerWithContext for NIC-level ExtraConfig and
// device-spec reconciliation.
func New() vmconfig.ReconcilerWithContext {
	return reconciler{}
}

// Reconcile is a convenience wrapper for callers that invoke the reconciler
// directly without the ReconcilerWithContext lifecycle.
func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

func (r reconciler) Name() string { return "networkextraconfig" }

func (r reconciler) WithContext(parent context.Context) context.Context {
	return ctxgen.WithContext(
		parent,
		contextKeyValue,
		func() state { return state{} })
}

func (r reconciler) Reconcile(
	ctx context.Context,
	_ ctrlclient.Client,
	_ *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	if ctx == nil {
		panic("context is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}
	if configSpec == nil {
		panic("configSpec is nil")
	}

	if !pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI {
		return nil
	}
	if moVM.Config == nil {
		return nil
	}
	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return nil
	}

	ci := *moVM.Config
	observed := pkgutil.OptionValues(ci.ExtraConfig)
	poweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn
	matcher := defaultNICMatcher(ci.Hardware.Device)
	nicModeMap := vmopv1util.VMXNet3NICModeMap()

	var overlay pkgutil.OptionValues
	var blockedPowerOff, blocked []string
	powerCyclePending := false

	log := pkglog.FromContextOrDefault(ctx)

	// ── Per-NIC loop ──────────────────────────────────────────────────────────
	for i, iface := range vm.Spec.Network.Interfaces {
		matchedDev := matcher(iface, i)
		if matchedDev == nil {
			log.V(4).Info("no hardware device for spec NIC; skipping", "interfaceName", iface.Name, "specIdx", i)
			continue
		}

		namespaceIdx, ok := ethernetDeviceIndex(matchedDev)
		if !ok {
			continue
		}

		prefix := vmopv1util.EthernetExtraConfigPrefix(matchedDev.GetVirtualDevice().Key)

		// ── ExtraConfig path ──────────────────────────────────────────────────
		if iface.VMXNet3 != nil {
			overlay = append(overlay, extraconfig.TranslateVMXNet3NICFirstClass(ctx, matchedDev.GetVirtualDevice().Key, iface.VMXNet3)...)
		}

		specBagKeys := map[string]bool{}
		for _, kv := range iface.AdvancedProperties {
			if vmopv1util.IsFirstClassVMXnet3NICKey(kv.Key) {
				pkglog.FromContextOrDefault(ctx).Info(
					"AdvancedProperties contains a first-class NIC key; skipping",
					"interfaceName", iface.Name,
					"key", kv.Key)
				continue
			}
			specBagKeys[kv.Key] = true
			overlay = append(overlay, &vimtypes.OptionValue{Key: prefix + kv.Key, Value: kv.Value})
		}

		// Clear removed managed keys.
		mkKey := fmt.Sprintf(vsphereconst.NICExtraConfigManagedKeysKeyFmt, namespaceIdx)
		managed := extraconfig.LoadDeviceManagedKeys(observed, mkKey)
		for _, mk := range managed {
			if !specBagKeys[mk] {
				overlay = append(overlay, &vimtypes.OptionValue{Key: prefix + mk, Value: ""})
			}
		}

		// Update managed keys tracking entry when the bag key set changes.
		nextMK := make([]string, 0, len(specBagKeys))
		for k := range specBagKeys {
			nextMK = append(nextMK, k)
		}
		sort.Strings(nextMK)
		if strings.Join(nextMK, ",") != strings.Join(managed, ",") {
			overlay = append(overlay, &vimtypes.OptionValue{Key: mkKey, Value: strings.Join(nextMK, ",")})
		}

		// ── DeviceChange path ─────────────────────────────────────────────────
		b, bpo := reconcileNICFields(*vm, iface, matchedDev, ci, configSpec)
		blocked = append(blocked, b...)
		blockedPowerOff = append(blockedPowerOff, bpo...)
	}

	// ── ExtraConfig semantic diff + mode routing ──────────────────────────────
	allDesiredEC := pkgutil.OptionValues(configSpec.ExtraConfig).Merge(overlay...)
	semanticResult := extraconfig.VMXNet3SemanticDiff(ctx, observed, allDesiredEC, vmopv1util.VMXNet3NICKeyMap())

	var result pkgutil.OptionValues
	for _, entry := range semanticResult {
		kv := entry.GetOptionValue()
		if kv == nil {
			continue
		}
		// Normalize the live key (e.g. "ethernet0.ctxPerDev") to its template
		// form ("ethernet%d.ctxPerDev") for mode map lookup.
		mode, isFC := nicModeMap[vmopv1util.NormalizeEthernetDeviceKey(kv.Key)]
		switch {
		case isFC && mode == vmopv1util.VMXModePowerOff && poweredOn:
			blockedPowerOff = append(blockedPowerOff, kv.Key)
		default:
			result = append(result, entry)
			if isFC && mode == vmopv1util.VMXModePowerCycle && poweredOn {
				powerCyclePending = true
			}
		}
	}
	if powerCyclePending {
		result = append(result, &vimtypes.OptionValue{
			Key:   vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle,
			Value: vsphereconst.ExtraConfigTrue,
		})
	}

	configSpec.ExtraConfig = result

	ctxgen.SetContext(ctx, contextKeyValue, func(s state) state {
		s.BlockedPowerOff = blockedPowerOff
		s.PowerCyclePending = powerCyclePending
		s.Blocked = blocked
		return s
	})

	return nil
}

func (r reconciler) OnResult(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	resultErr error) error {

	if ctx == nil {
		panic("context is nil")
	}
	if vm == nil {
		panic("vm is nil")
	}

	if !pkgcfg.FromContext(ctx).Features.TelcoVMServiceAPI {
		return nil
	}

	s := ctxgen.FromContext(ctx, contextKeyValue, func(s state) state { return s })

	if resultErr != nil && !pkgerr.IsNoRequeueNoError(resultErr) {
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachineNetworkErrorReason,
			"%v", resultErr)
		return nil
	}

	// Update status: NIC ExtraConfig keys and device-spec fields.
	if moVM.Config != nil && vm.Spec.Network != nil {
		observed := pkgutil.OptionValues(moVM.Config.ExtraConfig)
		matcher := defaultNICMatcher(moVM.Config.Hardware.Device)
		nicKeyMap := vmopv1util.VMXNet3NICKeyMap()

		log := pkglog.FromContextOrDefault(ctx)
		for i, iface := range vm.Spec.Network.Interfaces {
			matchedDev := matcher(iface, i)
			if matchedDev == nil {
				log.V(4).Info("no hardware device for spec NIC; skipping status update", "interfaceName", iface.Name, "specIdx", i)
				continue
			}

			namespaceIdx, ok := ethernetDeviceIndex(matchedDev)
			if !ok {
				continue
			}

			prefix := vmopv1util.EthernetExtraConfigPrefix(matchedDev.GetVirtualDevice().Key)

			// First-class ExtraConfig fields: expand template keys with the
			// device's namespace index to get the live VMX key.
			for tmplKey := range nicKeyMap {
				fullKey := fmt.Sprintf(tmplKey, namespaceIdx)
				if v, ok2 := observed.GetString(fullKey); ok2 && v != "" {
					vm.Status.ExtraConfig = append(vm.Status.ExtraConfig,
						vmopv1common.KeyValuePair{Key: fullKey, Value: v})
				}
			}

			// Managed bag keys.
			mkKey := fmt.Sprintf(vsphereconst.NICExtraConfigManagedKeysKeyFmt, namespaceIdx)
			for _, mk := range extraconfig.LoadDeviceManagedKeys(observed, mkKey) {
				fullKey := prefix + mk
				if v, ok2 := observed.GetString(fullKey); ok2 && v != "" {
					vm.Status.ExtraConfig = append(vm.Status.ExtraConfig,
						vmopv1common.KeyValuePair{Key: fullKey, Value: v})
				}
			}

			// Device-spec fields in status.network.interfaces[i].
			updateInterfaceStatus(vm, iface, matchedDev, moVM)
		}
	}

	// Set VirtualMachineNetworkConfigSynced condition.
	vmPoweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn
	var powerCycleOnVM bool
	if vmPoweredOn && moVM.Config != nil {
		_, powerCycleOnVM = pkgutil.OptionValues(moVM.Config.ExtraConfig).GetString(
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)
	}

	switch {
	case len(s.Blocked) > 0:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachinePrerequisiteNotMetReason,
			"Prerequisites not met: %s", strings.Join(s.Blocked, "; "))
	case len(s.BlockedPowerOff) > 0:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachinePowerOffRequiredReason,
			"VM power off required to apply: %s", strings.Join(s.BlockedPowerOff, ", "))
	case s.PowerCyclePending || powerCycleOnVM:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachinePowerCyclePendingReason,
			"applied changes take effect on next power cycle")
	default:
		conditions.MarkTrue(vm, vmopv1.VirtualMachineNetworkConfigSynced)
	}

	return nil
}

// updateInterfaceStatus populates the device-spec status fields
// (VNUMANodeID, VMXNet3) for interface i in vm.Status.Network.Interfaces.
func updateInterfaceStatus(
	vm *vmopv1.VirtualMachine,
	iface vmopv1.VirtualMachineNetworkInterfaceSpec,
	matchedDev vimtypes.BaseVirtualDevice,
	moVM mo.VirtualMachine,
) {
	if vm.Status.Network == nil {
		return
	}

	// Find the matching status entry by interface name or index.
	var statusIface *vmopv1.VirtualMachineNetworkInterfaceStatus
	for j := range vm.Status.Network.Interfaces {
		if vm.Status.Network.Interfaces[j].Name == iface.Name {
			statusIface = &vm.Status.Network.Interfaces[j]
			break
		}
	}
	if statusIface == nil {
		return
	}

	vdev := matchedDev.GetVirtualDevice()

	// VNUMANodeID: reflect observed device NumaNode; nil or negative means no affinity.
	numaNode := vdev.NumaNode
	if numaNode != nil && *numaNode >= 0 {
		statusIface.VNUMANodeID = numaNode
	} else {
		statusIface.VNUMANodeID = nil
	}

	// VMXNet3-specific status.
	vmxnet3Dev, isVMXNet3 := matchedDev.(*vimtypes.VirtualVmxnet3)
	if !isVMXNet3 {
		statusIface.VMXNet3 = nil
		return
	}

	vmxnet3Status := &vmopv1.VirtualMachineNetworkInterfaceVMXNet3Status{
		UPTv2Enabled: vmxnet3Dev.Uptv2Enabled,
	}

	// UPTv2 runtime state from moVM.Runtime.Device.
	for _, rdi := range moVM.Runtime.Device {
		if rdi.Key != vdev.Key {
			continue
		}
		ethState, ok := rdi.RuntimeState.(*vimtypes.VirtualMachineDeviceRuntimeInfoVirtualEthernetCardRuntimeState)
		if !ok {
			break
		}
		vmxnet3Status.UPTv2Active = ethState.Uptv2Active
		vmxnet3Status.UPTv2InactiveReasonVM = ethState.Uptv2InactiveReasonVm
		vmxnet3Status.UPTv2InactiveReasonOther = ethState.Uptv2InactiveReasonOther
		break
	}

	statusIface.VMXNet3 = vmxnet3Status
}
