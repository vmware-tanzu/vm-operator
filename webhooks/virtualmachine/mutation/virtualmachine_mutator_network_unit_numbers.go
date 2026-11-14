// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package mutation

import (
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	pkglog "github.com/vmware-tanzu/vm-operator/pkg/log"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	callOnUpdateOnlyMessageNICUnitNumbers = "MutateNICUnitNumbersOnUpdate should only be called on update"

	skippedNoSlotMessageNICUnitNumbers = "Skipping NIC: no available unit number on the NIC bus"
)

// MutateNICUnitNumbersOnUpdate mutates network interfaces that do not carry a
// unit number by assigning each the next available unit number in the 7-16
// ethernet-card band, in spec list order, mirroring how the volume and CD-ROM
// mutators assign disk and CD-ROM unit numbers. Explicit user-provided values
// are left untouched and merely reserve their slot in the occupied set.
//
// This is an update-path mutation, like MutateCdromControllerOnUpdate: a VM
// being created never has the upgrade annotations, so no unit numbers are
// assigned at create time. The schema-upgrade gate below is evaluated against
// the NEW object so that assignment begins on the same admission request that
// carries the schema upgrade's annotation stamp.
//
// Note that this mutator intentionally runs on VM Operator's own patches too,
// including the schema-upgrade backfill's: there is no privileged-account
// bypass. The backfill may deliberately leave an interface's unit number nil
// (e.g. when the observed value would make the spec inadmissible); on this
// VM — by then upgraded — that nil value is indistinguishable from a
// newly-added interface and receives a free slot, exactly as
// AddControllersForVolumes assigns a unit number to a volume the disk
// backfill skipped.
func MutateNICUnitNumbersOnUpdate(
	ctx *pkgctx.WebhookRequestContext,
	_ ctrlclient.Client,
	vm, oldVM *vmopv1.VirtualMachine) (bool, error) {

	if !pkgcfg.FromContext(ctx).Features.VMNetworkUnitNumbers {
		return false, nil
	}

	if oldVM == nil {
		ctx.Logger.Info(callOnUpdateOnlyMessageNICUnitNumbers)
		return false, nil
	}

	// Check the schema upgrade status on the new VM to ensure the mutation
	// is applied immediately after the schema upgrade completes.
	if err := vmopv1util.IsObjectUpgraded(ctx, vm); err != nil {
		pkglog.FromContextOrDefault(ctx).Info(
			"Skipping NIC unit number mutation",
			"reason", err.Error())
		return false, nil
	}

	if vm.Spec.Network == nil || len(vm.Spec.Network.Interfaces) == 0 {
		return false, nil
	}

	var (
		occupiedSlots = sets.New[int32]()
		unnumbered    []*vmopv1.VirtualMachineNetworkInterfaceSpec
	)

	// First phase: reserve the slots of every interface that carries an
	// explicit unit number, and collect the remainder.
	for i := range vm.Spec.Network.Interfaces {
		iface := &vm.Spec.Network.Interfaces[i]
		if iface.UnitNumber != nil {
			occupiedSlots.Insert(*iface.UnitNumber)
		} else {
			unnumbered = append(unnumbered, iface)
		}
	}

	var wasMutated bool

	// Second phase: assign the next available unit number to each remaining
	// interface, in list order.
	for _, iface := range unnumbered {
		nextUnit := vmopv1util.NextAvailableUnitNumber(
			vmopv1util.NICBusSpec{},
			occupiedSlots)
		if nextUnit < 0 {
			// Defensive only: with ten interfaces (the API's MaxItems) and
			// ten valid slots, uniqueness guarantees a free slot for any
			// interface that can be admitted. Leave the interface unassigned
			// rather than erroring; the validating webhook reports it.
			ctx.Logger.Info(skippedNoSlotMessageNICUnitNumbers,
				"interface", iface.Name)
			continue
		}

		occupiedSlots.Insert(nextUnit)
		iface.UnitNumber = ptr.To(nextUnit)
		wasMutated = true
	}

	return wasMutated, nil
}
