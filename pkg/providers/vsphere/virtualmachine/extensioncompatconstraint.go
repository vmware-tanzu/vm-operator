// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
)

// extensionCompatConstraintKey identifies an extension compatibility
// constraint's identity on the wire: the (ConstraintType, ConstraintKind)
// pair. ConstraintName is a descriptive label only and is deliberately
// excluded from this key.
type extensionCompatConstraintKey struct {
	constraintType string
	constraintKind string
}

// extensionCompatibilityConstraintSetMatches reports whether current already
// contains exactly the desired set of INVARIANT constraints, ignoring
// ConstraintName and the order of the Constraint slice.
func extensionCompatibilityConstraintSetMatches(
	current *vimtypes.VirtualMachineExtensionCompatibilityConstraintSet) bool {

	desired := extensionCompatibilityConstraintSet().Constraint

	if current == nil || len(current.Constraint) != len(desired) {
		return false
	}

	currentKeys := make(map[extensionCompatConstraintKey]struct{}, len(current.Constraint))
	for _, c := range current.Constraint {
		currentKeys[extensionCompatConstraintKey{c.ConstraintType, c.ConstraintKind}] = struct{}{}
	}

	for _, c := range desired {
		if _, ok := currentKeys[extensionCompatConstraintKey{c.ConstraintType, c.ConstraintKind}]; !ok {
			return false
		}
	}

	return true
}

// UpdateConfigSpecExtensionCompatibilityConstraint sets
// configSpec.ExtensionCompatibilityConstraint to the desired set of 6
// INVARIANT constraints if config's current set does not already match it.
//
// A reconfigure is a full-set-replace, so re-sending the full desired set
// corrects any drift in one shot: missing constraints, stale leftovers from
// an older desired set, or a set left behind by a snapshot revert (which
// restores ConfigInfo.managedBy via the VMX delta chain but leaves
// VCDB-backed constraints untouched).
func UpdateConfigSpecExtensionCompatibilityConstraint(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	if extensionCompatibilityConstraintSetMatches(config.ExtensionCompatibilityConstraint) {
		return
	}

	configSpec.ExtensionCompatibilityConstraint = extensionCompatibilityConstraintSet()
}

// ClearConfigSpecExtensionCompatibilityConstraint removes the extension
// compatibility constraints VM Operator registered on the VM. This should be
// called when unregistering a VM from Supervisor (see CleanupVMServiceState),
// since the constraints otherwise persist and continue to be enforced
// against whatever manages the VM next.
//
// The constraint set is declared by the VM's managing extension as a whole
// (see ManagedBy), not per-constraint, so it is only cleared when VM
// Operator is still the managing extension.
func ClearConfigSpecExtensionCompatibilityConstraint(
	config *vimtypes.VirtualMachineConfigInfo,
	configSpec *vimtypes.VirtualMachineConfigSpec) {

	if config == nil || config.ExtensionCompatibilityConstraint == nil {
		return
	}

	if config.ManagedBy == nil ||
		config.ManagedBy.ExtensionKey != vmopv1.ManagedByExtensionKey ||
		config.ManagedBy.Type != vmopv1.ManagedByExtensionType {
		return
	}

	// A reconfigure's constraint set is a full-set-replace, so a non-nil,
	// empty set clears the field.
	configSpec.ExtensionCompatibilityConstraint = &vimtypes.VirtualMachineExtensionCompatibilityConstraintSet{}
}
