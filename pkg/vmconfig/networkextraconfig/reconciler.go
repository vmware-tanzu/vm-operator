// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package networkextraconfig

import (
	"context"
	"fmt"
	"strings"

	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	vimtypes "github.com/vmware/govmomi/vim25/types"

	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/conditions"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgerr "github.com/vmware-tanzu/vm-operator/pkg/errors"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/networkextraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for NIC-level ExtraConfig and device-spec
// reconciliation.
func New() vmconfig.Reconciler {
	return reconciler{}
}

// Reconcile is a convenience wrapper for callers that invoke the reconciler
// directly without the vmconfig.Reconciler lifecycle.
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
	matcher := networkextraconfig.DefaultNICMatcher(ci.Hardware.Device)

	var overlay pkgutil.OptionValues
	log := pkglog.FromContextOrDefault(ctx)

	// ── Per-NIC loop ──────────────────────────────────────────────────────────
	for i, iface := range vm.Spec.Network.Interfaces {
		matchedDev := matcher(iface, i)
		if matchedDev == nil {
			log.V(4).Info("no hardware device for spec NIC; skipping", "interfaceName", iface.Name, "specIdx", i)
			continue
		}

		namespaceIdx, ok := networkextraconfig.EthernetDeviceIndex(matchedDev)
		if !ok {
			continue
		}
		devKey := matchedDev.GetVirtualDevice().Key
		mkKey := fmt.Sprintf(vsphereconst.NICExtraConfigManagedKeysKeyFmt, namespaceIdx)
		managed := extraconfig.LoadDeviceManagedKeys(observed, mkKey)

		// ── ExtraConfig path ──────────────────────────────────────────────────
		overlay = append(overlay, networkextraconfig.DesiredNICExtraConfig(ctx, iface, devKey, managed)...)

		// Update the managed keys tracking entry when the bag key set
		// changes. Not part of the diff below since it's pure bookkeeping,
		// not a vmxmode-routed field.
		specBagKeys := networkextraconfig.NICSpecBagKeys(ctx, iface)
		nextMK := sets.List(sets.KeySet(specBagKeys))
		if strings.Join(nextMK, ",") != strings.Join(managed, ",") {
			overlay = append(overlay, &vimtypes.OptionValue{Key: mkKey, Value: strings.Join(nextMK, ",")})
		}

		// ── DeviceChange path ─────────────────────────────────────────────────
		networkextraconfig.ReconcileNICFields(*vm, iface, matchedDev, ci, configSpec, false)
	}

	// Diff the assembled overlay against observed, merged onto whatever other
	// reconcilers already set on configSpec, and route it by vmxmode:
	// PowerOff entries cannot be applied while the VM is on (they are
	// deferred here, but this reconciler has no further use for that list —
	// see reconcileStatusNetworkExtraConfig for the status/condition use of
	// the same diff). PowerCycle entries are applied but take effect only
	// after a guest power cycle.
	result, _, powerCyclePending := networkextraconfig.NICExtraConfigDiff(
		ctx, observed, overlay, pkgutil.OptionValues(configSpec.ExtraConfig), poweredOn)

	if powerCyclePending {
		result = append(result, &vimtypes.OptionValue{
			Key:   vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle,
			Value: vsphereconst.ExtraConfigTrue,
		})
	}

	configSpec.ExtraConfig = result

	return nil
}

// OnResult marks NetworkConfigSynced=False when the Reconfigure task this
// cycle submitted actually failed. That is the only thing this function has
// definitive knowledge of that reconcileStatusNetworkExtraConfig
// (pkg/providers/vsphere/vmlifecycle) cannot derive on its own: it runs
// before Reconcile/doReconfigure even attempt the change, using vmCtx.MoVM as
// fetched at the start of this reconcile. reconcileStatusNetworkExtraConfig
// is the sole source of True/False+reason otherwise — computed fresh from
// that same moVM plus spec.network every reconcile, so it is correct whether
// or not this function also runs in a given pass.
func (r reconciler) OnResult(
	ctx context.Context,
	vm *vmopv1.VirtualMachine,
	_ mo.VirtualMachine,
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

	if resultErr != nil && !pkgerr.IsNoRequeueNoError(resultErr) {
		conditions.MarkFalse(
			vm,
			vmopv1.VirtualMachineNetworkConfigSynced,
			vmopv1.VirtualMachineNetworkConfigErrorReason,
			"%v", resultErr)
	}

	return nil
}
