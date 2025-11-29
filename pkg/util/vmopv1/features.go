// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	pkgcfg "github.com/vmware-tanzu/vm-operator/pkg/config"
	pkgconst "github.com/vmware-tanzu/vm-operator/pkg/constants"
)

// NotUpgradedErr is returned by IsVirtualMachineSchemaUpgraded when an object
// has not yet been upgraded to the target build, schema, or feature version.
type NotUpgradedErr struct {
	Type   string
	Object string
	Target string
}

func (e NotUpgradedErr) String() string {
	return fmt.Sprintf("%s not upgraded: current=%s, target=%s",
		e.Type, e.Object, e.Target)
}

func (e NotUpgradedErr) Error() string {
	return e.String()
}

// IsObjectSchemaUpgraded returns nil if below conditions are all met
// by checking the upgrade annotations:
//
//  1. The object's build version has been upgraded to the current build
//     version.
//  2. The object's schema version has been upgraded to the current schema
//     version.
//  3. The object's feature version has been upgraded to the current feature
//     version.
func IsObjectSchemaUpgraded(
	ctx context.Context,
	obj ctrlclient.Object) error {

	var (
		a     = obj.GetAnnotations()
		objBV = a[pkgconst.UpgradedToBuildVersionAnnotationKey]
		objSV = a[pkgconst.UpgradedToSchemaVersionAnnotationKey]
		objFV = ParseFeatureVersion(
			a[pkgconst.UpgradedToFeatureVersionAnnotationKey])

		tgtBV = pkgcfg.FromContext(ctx).BuildVersion
		tgtSV = vmopv1.GroupVersion.Version
		tgtFV = ActivatedFeatureVersion(ctx)
	)

	var errs []error

	if objBV == "" || objBV != tgtBV {
		errs = append(errs, NotUpgradedErr{
			Type:   "buildVersion",
			Object: objBV,
			Target: tgtBV,
		})
	}

	if objSV == "" || objSV != tgtSV {
		errs = append(errs, NotUpgradedErr{
			Type:   "schemaVersion",
			Object: objSV,
			Target: tgtSV,
		})
	}

	// To be considered upgraded, the object's feature version must:
	//
	//   * Not be empty, and...
	//   * equal the target version, or...
	//   * equal the target version or contain all the bits in the target
	//     version.
	//
	// The last case is when a feature or capability has been disabled
	// *after* an object has already has its feature version upgraded.
	if objFV == FeatureVersionEmpty || !objFV.IsOrSuperset(tgtFV) {
		errs = append(errs, NotUpgradedErr{
			Type:   "featureVersion",
			Object: objFV.String(),
			Target: tgtFV.String(),
		})
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// FeatureVersion is a bitmask of the activated features and is used to track
// which features were activated when an object's data was upgraded.
type FeatureVersion uint16

//
// !!!!!!!!!!!!!!!!!!!!!!!!!!!! PLEASE NOTE !!!!!!!!!!!!!!!!!!!!!!!!!!!!
//
// The order of the below constants MUST stay the same, as they control
// the mask. If the mask order changes, then the value stored as an
// annotation on the object could be interpreted incorrectly.
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

// FeatureVersions returns all possible, valid FeatureVersion elements.
func FeatureVersions() []FeatureVersion {
	return []FeatureVersion{
		FeatureVersionBase,
		FeatureVersionVMSharedDisks,
		FeatureVersionAllDisksArePVCs,
	}
}

// IsValid returns true if the feature version contains only valid feature bits
// and is non-zero.
func (a FeatureVersion) IsValid() bool {
	// Check that:
	// 1. v is not zero (at least one bit must be set)
	// 2. v has no bits set outside the valid bits
	return a != FeatureVersionEmpty && (a & ^FeatureVersionAll) == 0
}

// String returns the string-ified version of the feature version.
func (a FeatureVersion) String() string {
	if !a.IsValid() {
		return ""
	}
	return strconv.Itoa(int(a))
}

// Has returns true if the given feature version includes the specified feature
// version.
func (a FeatureVersion) Has(b FeatureVersion) bool {
	return a&b != 0
}

// IsOrSuperset returns true if the given feature version is equal to the
// specified feature version or if the specified feature version's members all
// exist in the given version.
func (a FeatureVersion) IsOrSuperset(b FeatureVersion) bool {
	if a == b {
		return true
	}

	// Collect the bits set in b.
	var bm []FeatureVersion
	for _, f := range FeatureVersions() {
		if b.Has(f) {
			bm = append(bm, f)
		}
	}

	// Ensure a contains all of the members set in b.
	for _, f := range bm {
		if !a.Has(f) {
			return false
		}
	}

	return true
}

// Set adds the specified feature to the feature version.
func (a *FeatureVersion) Set(b FeatureVersion) {
	*a |= b
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
