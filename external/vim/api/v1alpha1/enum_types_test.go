// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"

	v1alpha1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
)

func TestSupportLevelVimTypeRoundTrip(t *testing.T) {
	levels := []v1alpha1.SupportLevel{
		v1alpha1.SupportLevelDeprecated,
		v1alpha1.SupportLevelExperimental,
		v1alpha1.SupportLevelLegacy,
		v1alpha1.SupportLevelSupported,
		v1alpha1.SupportLevelTechPreview,
		v1alpha1.SupportLevelTerminated,
		v1alpha1.SupportLevelUnsupported,
	}

	for _, want := range levels {
		vimType := want.ToVimType()

		var got v1alpha1.SupportLevel
		got.FromVimType(vimType)

		if got != want {
			t.Errorf("FromVimType(%q) = %q, want %q", vimType, got, want)
		}
	}
}

func TestSupportLevelFromVimTypeKnownValues(t *testing.T) {
	// These are the literal wire values vim.vm.GuestOsDescriptor.SupportLevel
	// sends, which are lowercase/camelCase, not the PascalCase values used by
	// this type's own constants.
	cases := map[string]v1alpha1.SupportLevel{
		"deprecated":   v1alpha1.SupportLevelDeprecated,
		"experimental": v1alpha1.SupportLevelExperimental,
		"legacy":       v1alpha1.SupportLevelLegacy,
		"supported":    v1alpha1.SupportLevelSupported,
		"techPreview":  v1alpha1.SupportLevelTechPreview,
		"terminated":   v1alpha1.SupportLevelTerminated,
		"unsupported":  v1alpha1.SupportLevelUnsupported,
	}

	for raw, want := range cases {
		var got v1alpha1.SupportLevel
		got.FromVimType(raw)

		if got != want {
			t.Errorf("FromVimType(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestVirtualMachineGuestOSFamilyFromVimTypeKnownValues(t *testing.T) {
	cases := map[string]v1alpha1.VirtualMachineGuestOSFamily{
		"darwinGuestFamily": v1alpha1.VirtualMachineGuestOSFamilyDarwin,
		"linuxGuest":        v1alpha1.VirtualMachineGuestOSFamilyLinux,
		"netwareGuest":      v1alpha1.VirtualMachineGuestOSFamilyNetware,
		"otherGuestFamily":  v1alpha1.VirtualMachineGuestOSFamilyOther,
		"solarisGuest":      v1alpha1.VirtualMachineGuestOSFamilySolaris,
		"windowsGuest":      v1alpha1.VirtualMachineGuestOSFamilyWindows,
	}

	for raw, want := range cases {
		var got v1alpha1.VirtualMachineGuestOSFamily
		got.FromVimType(raw)

		// Regression test: FromVimType previously overwrote the matched
		// value with the raw, unmapped string unconditionally after the
		// switch statement, so every known value round-tripped back out as
		// itself, and none of them ever resolved to their PascalCase
		// constant.
		if got != want {
			t.Errorf("FromVimType(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestVirtualMachineGuestOSFamilyFromVimTypeUnknownValue(t *testing.T) {
	var got v1alpha1.VirtualMachineGuestOSFamily
	got.FromVimType("someFutureGuestFamily")

	if want := v1alpha1.VirtualMachineGuestOSFamily("someFutureGuestFamily"); got != want {
		t.Errorf("FromVimType(unknown) = %q, want %q", got, want)
	}
}
