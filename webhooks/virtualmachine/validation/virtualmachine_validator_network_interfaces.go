// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

const (
	invalidNICUnitNumberRangeFmt = "unit number must be between %d and %d"
	invalidNICUnitNumberInUse    = "unit number is already used by another network interface on this VM"
)

// validateNICUnitNumbers validates spec.network.interfaces[i].unitNumber.
//
// The rules differ depending on whether the VMNetworkUnitNumbers feature is
// enabled:
//
//   - When the feature is disabled, a unit number that is new or changed
//     relative to the old VM is rejected: values persisted while the feature
//     is off would later poison the spec-wins backfill, and a renumbered or
//     cleared value must not take effect while the feature is off. Mere
//     presence of an unchanged value is NOT rejected — every UPDATE
//     (including the VM controller's own patches and finalizer removal, i.e.
//     delete) is validated as a whole object with no early-out, so a
//     presence-based rejection would make an already-backfilled VM
//     undeletable while the feature is off.
//   - When the feature is enabled, every set unit number must be within the
//     ethernet-card PCI unit range (vmopv1util.NICUnitNumberFirst through
//     vmopv1util.NICUnitNumberMax) and unique among all interfaces on the VM.
//
// Additionally, when the feature is enabled and the VM is powered on, an
// interface that existed on the old VM may not change its unit number:
// changing an already-set value, or clearing one (set -> nil), is rejected
// regardless of the requesting account. Setting a previously-unset value on
// a powered-on VM is only allowed for the VM Operator service account — the
// schema-upgrade backfill records observed values into a running VM's spec
// through that account — since a user-originated nil -> set change could
// claim a slot occupied by another interface's device and trigger a
// device-identity swap on the next reconcile. A newly-added interface is not
// a change and is not subject to the powered-on rules; it arrives with a
// mutator-assigned value on an upgraded VM (or without one on a VM that has
// not been upgraded yet).
//
// The powered-on rules are skipped entirely when the old VM is not schema
// upgraded: the schema-upgrade patch carries the annotation along with any
// backfilled values, so judging that patch by these rules would reject the
// backfill itself.
func (v validator) validateNICUnitNumbers(
	ctx *pkgctx.WebhookRequestContext,
	vm, oldVM *vmopv1.VirtualMachine,
	p *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	if vm.Spec.Network == nil {
		return allErrs
	}

	if !pkgcfg.FromContext(ctx).Features.VMNetworkUnitNumbers {
		// Reject only a unit number that is new or changed relative to oldVM.
		// Note this covers delete: a VM being deleted whose interfaces carry
		// unchanged, previously-backfilled values remains admissible.
		return append(allErrs, validateNICUnitNumbersFeatureOff(vm, oldVM, p)...)
	}

	var (
		poweredOnVM = oldVM != nil &&
			oldVM.Spec.PowerState == vmopv1.VirtualMachinePowerStateOn
		oldUnitByName map[string]*int32
	)

	if poweredOnVM {
		// The powered-on rules are skipped when the old VM is not schema
		// upgraded because the patch on schema upgrade will contain the
		// annotation along with any backfilled values.
		if err := vmopv1util.IsObjectUpgraded(ctx, oldVM); err != nil {
			ctx.Logger.V(4).Info(
				"Skipping NIC unit number powered-on validation because VM is not schema upgraded",
				"reason", err.Error(),
			)
			poweredOnVM = false
		} else {
			oldUnitByName = oldNICUnitNumbersByName(oldVM)
		}
	}

	occupiedSlots := sets.New[int32]()

	for i := range vm.Spec.Network.Interfaces {
		iface := &vm.Spec.Network.Interfaces[i]
		ifacePath := p.Index(i).Child("unitNumber")

		if iface.UnitNumber == nil {
			if poweredOnVM {
				// A set -> nil (cleared) transition is a change and is
				// rejected while the VM is powered on, regardless of the
				// requesting account.
				if oldUnit, existed := oldUnitByName[iface.Name]; existed && oldUnit != nil {
					allErrs = append(allErrs, field.Forbidden(
						ifacePath,
						updatesNotAllowedWhenPowerOn,
					))
				}
			}
			continue
		}

		unit := *iface.UnitNumber

		if unit < vmopv1util.NICUnitNumberFirst || unit > vmopv1util.NICUnitNumberMax {
			allErrs = append(allErrs, field.Invalid(
				ifacePath,
				unit,
				fmt.Sprintf(invalidNICUnitNumberRangeFmt,
					vmopv1util.NICUnitNumberFirst, vmopv1util.NICUnitNumberMax),
			))
			continue
		}

		if occupiedSlots.Has(unit) {
			allErrs = append(allErrs, field.Invalid(
				ifacePath,
				unit,
				invalidNICUnitNumberInUse,
			))
			continue
		}
		occupiedSlots.Insert(unit)

		if poweredOnVM {
			oldUnit, existed := oldUnitByName[iface.Name]
			if !existed {
				// A newly-added interface is not a "change".
				continue
			}

			switch {
			case oldUnit != nil && *oldUnit != unit:
				// An already-set value may not move to a different slot while
				// the VM is powered on.
				allErrs = append(allErrs, field.Forbidden(
					ifacePath,
					updatesNotAllowedWhenPowerOn,
				))
			case oldUnit == nil && !ctx.IsVMOperatorAccount:
				// A user may not set a previously-unset value on a powered-on
				// VM: a nil -> set change could claim a slot occupied by
				// another interface's device and trigger a device-identity
				// swap on the next reconcile. Only the VM Operator service
				// account may do this, since the schema-upgrade backfill
				// records observed values into a running VM's spec through
				// that account.
				allErrs = append(allErrs, field.Forbidden(
					ifacePath,
					updatesNotAllowedWhenPowerOn,
				))
			}
			// oldUnit != nil && *oldUnit == unit is unchanged and allowed; a
			// nil -> set change by the VM Operator account (the backfill
			// write path) is allowed.
		}
	}

	return allErrs
}

// validateNICUnitNumbersFeatureOff rejects a unit number that is new or
// changed relative to oldVM while the VMNetworkUnitNumbers feature is
// disabled. Presence of an unchanged value is not rejected: ValidateUpdate
// validates the whole object on every UPDATE with no delete or no-change
// early-out, so a presence-based rejection would fail every update to an
// already-backfilled VM — including the controller's own patches and
// finalizer removal, making the VM undeletable. A set -> nil transition is a
// change and is rejected so the value cannot be silently wiped while the
// feature is off.
func validateNICUnitNumbersFeatureOff(
	vm, oldVM *vmopv1.VirtualMachine,
	p *field.Path) field.ErrorList {

	var allErrs field.ErrorList

	ifacePath := func(i int) *field.Path {
		return p.Index(i).Child("unitNumber")
	}

	if oldVM == nil {
		// Create: any unit number is new.
		for i := range vm.Spec.Network.Interfaces {
			iface := &vm.Spec.Network.Interfaces[i]
			if iface.UnitNumber != nil {
				allErrs = append(allErrs, field.Forbidden(
					ifacePath(i),
					fmt.Sprintf(featureNotEnabled, "VM Network Unit Numbers"),
				))
			}
		}
		return allErrs
	}

	oldUnitByName := oldNICUnitNumbersByName(oldVM)

	for i := range vm.Spec.Network.Interfaces {
		iface := &vm.Spec.Network.Interfaces[i]
		oldUnit, ok := oldUnitByName[iface.Name]

		if iface.UnitNumber == nil {
			if oldUnit == nil {
				// No unit number involved: a newly-added interface without a
				// unit number, or a pre-existing interface that carries none.
				continue
			}
			// set -> nil: a changed value, rejected so the value cannot be
			// silently wiped while the feature is off.
		} else if ok && oldUnit != nil && *oldUnit == *iface.UnitNumber {
			// Unchanged value: not rejected (see I14 in the spec plan).
			continue
		}

		allErrs = append(allErrs, field.Forbidden(
			ifacePath(i),
			fmt.Sprintf(featureNotEnabled, "VM Network Unit Numbers"),
		))
	}

	return allErrs
}

// oldNICUnitNumbersByName returns a map of interface name to unit number
// pointer for the given VM's network interfaces.
func oldNICUnitNumbersByName(vm *vmopv1.VirtualMachine) map[string]*int32 {
	m := make(map[string]*int32)
	if vm.Spec.Network != nil {
		for i := range vm.Spec.Network.Interfaces {
			iface := &vm.Spec.Network.Interfaces[i]
			m[iface.Name] = iface.UnitNumber
		}
	}
	return m
}
