// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	vmopv1common "github.com/vmware-tanzu/vm-operator/api/v1alpha6/common"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	ctxgen "github.com/vmware-tanzu/vm-operator/pkg/context/generic"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

// Reason constants for VirtualMachineConditionExtraConfigSynced.
const (
	ReasonPowerOffRequired  = "PowerOffRequired"
	ReasonPowerCyclePending = "PowerCyclePending"
	ReasonError             = "Error"
)

type contextKeyType uint8

const contextKeyValue contextKeyType = 0

// sortedFirstClassVMXKeys is the sorted list of VMX keys mapped to first-class
// API fields. Precomputed once to avoid rebuilding on every OnResult call.
var sortedFirstClassVMXKeys = sync.OnceValue(func() []string {
	keys := vmopv1util.AdvancedVMXKeyMap()
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
})

// state is shared between Reconcile and OnResult via context.
type state struct {
	// NextManagedKeys is the sorted list of spec.advanced.extraConfig bag keys.
	NextManagedKeys []string
	// Deferred holds first-class VMX keys skipped due to PowerOff mode while VM is on.
	Deferred []string
	// PowerCyclePending is true when vmx.reboot.powerCycle was injected this cycle.
	PowerCyclePending bool
}

type reconciler struct{}

var _ vmconfig.ReconcilerWithContext = reconciler{}

// New returns a new ReconcilerWithContext for spec.advanced ExtraConfig.
func New() vmconfig.ReconcilerWithContext {
	return reconciler{}
}

func Reconcile(
	ctx context.Context,
	k8sClient ctrlclient.Client,
	vimClient *vim25.Client,
	vm *vmopv1.VirtualMachine,
	moVM mo.VirtualMachine,
	configSpec *vimtypes.VirtualMachineConfigSpec) error {

	return New().Reconcile(ctx, k8sClient, vimClient, vm, moVM, configSpec)
}

func (r reconciler) Name() string { return "extraconfig" }

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

	// No live VM state yet (e.g. during create before the VM exists in vSphere).
	if moVM.Config == nil {
		return nil
	}

	observed := pkgutil.OptionValues(moVM.Config.ExtraConfig)
	poweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn
	firstClassKeys := vmopv1util.AdvancedVMXKeyMap()
	modeMap := vmopv1util.AdvancedVMXModeMap()

	// Collect overlay entries to apply on top of configSpec.ExtraConfig.
	var overlay pkgutil.OptionValues

	// Overlay bag keys from spec.advanced.extraConfig, skipping reserved and first-class keys.
	// The webhook rejects reserved and first-class keys before they reach the reconciler;
	// this guard is a defence-in-depth measure.
	specBagKeys := make(map[string]bool)
	if vm.Spec.Advanced != nil {
		for _, kv := range vm.Spec.Advanced.ExtraConfig {
			if vmopv1util.IsSystemReservedExtraConfigKey(kv.Key) || isFirstClassKey(kv.Key, firstClassKeys) {
				continue
			}
			specBagKeys[kv.Key] = true
			overlay = append(overlay, &vimtypes.OptionValue{Key: kv.Key, Value: kv.Value})
		}
	}

	// Clear bag keys previously tracked but no longer in spec.
	managed := extraconfig.LoadVMManagedKeys(observed)
	for _, mk := range managed {
		if !specBagKeys[mk] {
			overlay = append(overlay, &vimtypes.OptionValue{Key: mk, Value: ""})
		}
	}

	// Update the managed keys tracking entry when the bag key set changes.
	nextManagedKeys := make([]string, 0, len(specBagKeys))
	for k := range specBagKeys {
		nextManagedKeys = append(nextManagedKeys, k)
	}
	sort.Strings(nextManagedKeys)
	nextMKStr := strings.Join(nextManagedKeys, ",")
	if nextMKStr != strings.Join(managed, ",") {
		overlay = append(overlay, &vimtypes.OptionValue{
			Key:   vsphereconst.ExtraConfigManagedKeysKey,
			Value: nextMKStr,
		})
	}

	// Overlay all first-class translations (including "" for nil/zero fields).
	// vmxmode routing happens after SemanticDiff, not here.
	advanced := vm.Spec.Advanced
	if advanced == nil {
		advanced = &vmopv1.VirtualMachineAdvancedSpec{}
	}
	for _, entry := range extraconfig.TranslateFirstClass(ctx, advanced) {
		kv := entry.GetOptionValue()
		if kv == nil {
			continue
		}
		desiredStr, _ := kv.Value.(string)
		overlay = append(overlay, &vimtypes.OptionValue{Key: kv.Key, Value: desiredStr})
	}

	// Assemble: apply our overlay on top of what other reconcilers already set.
	var allDesiredEC pkgutil.OptionValues
	if len(overlay) > 0 {
		allDesiredEC = pkgutil.OptionValues(configSpec.ExtraConfig).Merge(overlay...)
	} else {
		allDesiredEC = pkgutil.OptionValues(configSpec.ExtraConfig)
	}

	// Suppress entries semantically identical to what vSphere already has.
	// Only entries representing genuine changes remain.
	semanticResult := extraconfig.SemanticDiff(ctx, observed, allDesiredEC)

	// Route result by vmxmode: PowerOff entries cannot be applied while the VM is
	// on (they are deferred), PowerCycle entries are applied but take effect only
	// after a guest power cycle.
	var deferred []string
	powerCyclePending := false
	var result pkgutil.OptionValues
	for _, entry := range semanticResult {
		kv := entry.GetOptionValue()
		if kv == nil {
			continue
		}
		mode, isFC := modeMap[kv.Key]
		switch {
		case isFC && mode == vmopv1util.VMXModePowerOff && poweredOn:
			deferred = append(deferred, kv.Key)
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
		s.NextManagedKeys = nextManagedKeys
		s.Deferred = deferred
		s.PowerCyclePending = powerCyclePending
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

	// On task error, mark condition false and bail.
	if resultErr != nil {
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineExtraConfigSynced,
			ReasonError,
			"%v", resultErr)
		return nil
	}

	// Compute status.ExtraConfig: reflect what the VM currently has for user-controlled keys.
	if moVM.Config != nil {
		observed := pkgutil.OptionValues(moVM.Config.ExtraConfig)
		sortedKeys := sortedFirstClassVMXKeys()
		var statusEC []vmopv1common.KeyValuePair
		for _, k := range sortedKeys {
			if v, ok := observed.GetString(k); ok && v != "" {
				statusEC = append(statusEC, vmopv1common.KeyValuePair{Key: k, Value: v})
			}
		}
		for _, k := range s.NextManagedKeys {
			if v, ok := observed.GetString(k); ok && v != "" {
				statusEC = append(statusEC, vmopv1common.KeyValuePair{Key: k, Value: v})
			}
		}
		vm.Status.ExtraConfig = statusEC
	}

	// Set VirtualMachineExtraConfigSynced condition.
	//
	// powerCycleOnVM tracks whether a prior cycle left vmx.reboot.powerCycle=TRUE
	// on the VM. When the VM is already powered off, all config has been applied
	// and ESXi will clear the flag automatically on the next boot — nothing more
	// is required from the user, so we treat it as synced.
	vmPoweredOn := vm.Status.PowerState == vmopv1.VirtualMachinePowerStateOn
	var powerCycleOnVM bool
	if vmPoweredOn && moVM.Config != nil {
		_, powerCycleOnVM = pkgutil.OptionValues(moVM.Config.ExtraConfig).GetString(
			vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle)
	}

	switch {
	case len(s.Deferred) > 0:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineExtraConfigSynced,
			ReasonPowerOffRequired,
			"VM power off required to apply: %s", strings.Join(s.Deferred, ", "))
	case s.PowerCyclePending || powerCycleOnVM:
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineExtraConfigSynced,
			ReasonPowerCyclePending,
			"applied changes take effect on next power cycle")
	default:
		conditions.MarkTrue(vm, vmopv1.VirtualMachineExtraConfigSynced)
	}

	return nil
}

// isFirstClassKey returns true when the key maps to a vmx-tagged first-class
// field in VirtualMachineAdvancedSpec.
func isFirstClassKey(key string, firstClassKeys map[string]int) bool {
	_, ok := firstClassKeys[key]
	return ok
}
