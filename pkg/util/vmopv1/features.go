// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"strconv"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

// IsVirtualMachineSchemaUpgraded returns true if below conditions are all met
// by checking the upgrade annotations:
//
//  1. The VM's build version has been upgraded to the current build version.
//  2. The VM's schema version has been upgraded to the current schema version.
//  3. The VM's feature version has been upgraded to the current feature
//     version.
func IsVirtualMachineSchemaUpgraded(
	ctx context.Context,
	vm vmopv1.VirtualMachine) bool {

	var (
		a  = vm.Annotations
		bv = a[pkgconst.UpgradedToBuildVersionAnnotationKey]
		sv = a[pkgconst.UpgradedToSchemaVersionAnnotationKey]
		fv = a[pkgconst.UpgradedToFeatureVersionAnnotationKey]
	)

	if bv != "" && bv == pkgcfg.FromContext(ctx).BuildVersion &&
		//
		sv != "" && sv == vmopv1.GroupVersion.Version &&
		//
		fv != "" && fv == ActivatedFeatureVersion(ctx).String() {

		return true
	}

	return false
}

// FeatureVersion is a bitmask of the activated features and is used to track
// which features were activated when a VM object's data was upgraded.
type FeatureVersion uint16

//
// !!!!!!!!!!!!!!!!!!!!!!!!!!!! PLEASE NOTE !!!!!!!!!!!!!!!!!!!!!!!!!!!!
//
// The order of the below constants MUST stay the same, as they control
// the mask. If the mask order changes, then the value stored as an
// annotation on the VM could be interpreted incorrectly.
//
// It is PARAMOUNT that if any new FeatureVersion constants are added to the
// mask, that they be added to this bitwise FeatureVersionAll. This constant is
// the key to the IsValid function.
//

const (
	// FeatureVersionBase is the basic feature version.
	FeatureVersionBase FeatureVersion = 1 << iota // 1

	// FeatureVersionVMSharedDisks refers to the VMSharedDisks capability.
	FeatureVersionVMSharedDisks // 2

	// FeatureVersionAllDisksArePVCs refers to the AllDisksArePVCs capability.
	FeatureVersionAllDisksArePVCs // 4
)

const (
	// FeatureVersionEmpty is the empty feature version.
	FeatureVersionEmpty FeatureVersion = 0

	// FeatureVersionAll is all valid feature bits OR'd together.
	FeatureVersionAll = FeatureVersionBase |
		FeatureVersionVMSharedDisks |
		FeatureVersionAllDisksArePVCs // 7
)

// IsValid returns true if the feature version contains only valid feature bits
// and is non-zero.
func (v FeatureVersion) IsValid() bool {
	// Check that:
	// 1. v is not zero (at least one bit must be set)
	// 2. v has no bits set outside the valid bits
	return v != FeatureVersionEmpty && (v & ^FeatureVersionAll) == 0
}

// String returns the string-ified version of the feature version.
func (v FeatureVersion) String() string {
	return strconv.Itoa(int(v))
}

// Has returns true if the given feature version includes the specified feature
// version.
func (v FeatureVersion) Has(f FeatureVersion) bool {
	return v&f != 0
}

// Set adds the specified feature to the feature version.
func (v *FeatureVersion) Set(f FeatureVersion) {
	*v |= f
}

// ParseFeatureVersion parses the provided string and returns the
// FeatureVersion. If the string is empty or invalid, then FeatureVersionEmpty
// is returned.
func ParseFeatureVersion(s string) FeatureVersion {
	u, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return FeatureVersionEmpty
	}
	v := FeatureVersion(u)
	if !v.IsValid() {
		v = FeatureVersionEmpty
	}
	return v
}

// ActivatedFeatureVersion returns the feature version when considering all
// possible, activated features and capabilities.
func ActivatedFeatureVersion(ctx context.Context) FeatureVersion {
	var (
		v = FeatureVersionBase
		f = pkgcfg.FromContext(ctx).Features
	)
	if f.VMSharedDisks {
		v.Set(FeatureVersionVMSharedDisks)
	}
	if f.AllDisksArePVCs {
		v.Set(FeatureVersionAllDisksArePVCs)
	}
	return v
}
