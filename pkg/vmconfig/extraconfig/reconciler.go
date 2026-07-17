// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package extraconfig

import (
	"context"
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
	vsphereconst "github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/virtualmachine/extraconfig"
	pkgutil "github.com/vmware-tanzu/vm-operator/pkg/util"
	"github.com/vmware-tanzu/vm-operator/pkg/vmconfig"
)

type reconciler struct{}

var _ vmconfig.Reconciler = reconciler{}

// New returns a new Reconciler for spec.advanced ExtraConfig.
func New() vmconfig.Reconciler {
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

	// Current bag-key set from spec (skips reserved and first-class keys;
	// the webhook rejects those before they reach the reconciler, so this is
	// a defence-in-depth measure). Used below to detect bag-key
	// additions/removals against the previously-tracked managed-keys marker.
	specBagKeys := extraconfig.SpecBagKeys(vm.Spec.Advanced)

	managed := extraconfig.LoadVMManagedKeys(observed)

	// Diff spec.advanced against observed, merged onto whatever other
	// reconcilers already set on configSpec, and route it by vmxmode:
	// PowerOff entries cannot be applied while the VM is on (they are
	// deferred here, but this reconciler has no further use for that list —
	// see reconcileStatusExtraConfig for the status/condition use of the same
	// diff). PowerCycle entries are applied but take effect only after a
	// guest power cycle.
	result, _, powerCyclePending := extraconfig.VMExtraConfigDiff(
		ctx, vm, observed, managed, pkgutil.OptionValues(configSpec.ExtraConfig))

	// Update the managed keys tracking entry when the bag key set changes.
	// Not part of the diff above since it's pure bookkeeping, not a
	// vmxmode-routed spec.advanced field.
	nextManagedKeys := sets.List(sets.KeySet(specBagKeys))
	nextMKStr := strings.Join(nextManagedKeys, ",")
	if nextMKStr != strings.Join(managed, ",") {
		result = append(result, &vimtypes.OptionValue{
			Key:   vsphereconst.ExtraConfigManagedKeysKey,
			Value: nextMKStr,
		})
	}

	if powerCyclePending {
		result = append(result, &vimtypes.OptionValue{
			Key:   vsphereconst.ExtraConfigReservedKeyVMXRebootPowerCycle,
			Value: vsphereconst.ExtraConfigTrue,
		})
	}

	configSpec.ExtraConfig = result

	return nil
}

// OnResult marks ExtraConfigSynced=False when the Reconfigure task this cycle
// submitted actually failed. That is the only thing this function has
// definitive knowledge of that reconcileStatusExtraConfig
// (pkg/providers/vsphere/vmlifecycle) cannot derive on its own: it runs
// before Reconcile/doReconfigure even attempt the change, using vmCtx.MoVM as
// fetched at the start of this reconcile. reconcileStatusExtraConfig is the
// sole source of True/False+reason otherwise — computed fresh from that same
// moVM plus spec.advanced every reconcile, so it is correct whether or not
// this function also runs in a given pass.
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
			vmopv1.VirtualMachineExtraConfigSynced,
			vmopv1.VirtualMachineExtraConfigErrorReason,
			"%v", resultErr)
	}

	return nil
}
