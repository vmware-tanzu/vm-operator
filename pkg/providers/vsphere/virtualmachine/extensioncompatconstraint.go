// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package virtualmachine

import (
	vimtypes "github.com/vmware/govmomi/vim25/types"
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
