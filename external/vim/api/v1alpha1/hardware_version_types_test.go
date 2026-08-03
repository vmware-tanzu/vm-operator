// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"encoding/json"
	"testing"

	vimv1 "github.com/vmware-tanzu/vm-operator/external/vim/api/v1alpha1"
)

func TestHardwareVersionIsValid(t *testing.T) {
	tests := []struct {
		name string
		hv   vimv1.HardwareVersion
		want bool
	}{
		{name: "zero value is invalid", hv: vimv1.HardwareVersion(0), want: false},
		{name: "VMX3 is valid", hv: vimv1.VMX3, want: true},
		{name: "VMX22 is valid", hv: vimv1.VMX22, want: true},
		{
			name: "a value beyond VMX22 is still valid",
			hv:   vimv1.HardwareVersion(255), want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.hv.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHardwareVersionIsSupported(t *testing.T) {
	tests := []struct {
		name string
		hv   vimv1.HardwareVersion
		want bool
	}{
		{
			name: "zero value is not supported",
			hv:   vimv1.HardwareVersion(0), want: false,
		},
		{
			name: "below MinValidHardwareVersion is not supported",
			hv:   vimv1.HardwareVersion(2), want: false,
		},
		{
			name: "MinValidHardwareVersion is supported",
			hv:   vimv1.MinValidHardwareVersion, want: true,
		},
		{
			name: "MaxValidHardwareVersion is supported",
			hv:   vimv1.MaxValidHardwareVersion, want: true,
		},
		{
			name: "above MaxValidHardwareVersion is not supported",
			hv:   vimv1.MaxValidHardwareVersion + 1, want: false,
		},
		{name: "VMX4 is supported", hv: vimv1.VMX4, want: true},
		{
			name: "hardware version 5 is reserved/invalid and not supported",
			hv:   vimv1.HardwareVersion(5), want: false,
		},
		{name: "VMX6 is supported", hv: vimv1.VMX6, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.hv.IsSupported(); got != tt.want {
				t.Errorf("IsSupported() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHardwareVersionString(t *testing.T) {
	tests := []struct {
		name string
		hv   vimv1.HardwareVersion
		want string
	}{
		{
			name: "zero value stringifies to an empty string",
			hv:   vimv1.HardwareVersion(0), want: "",
		},
		{name: "VMX21 stringifies to vmx-21", hv: vimv1.VMX21, want: "vmx-21"},
		{name: "VMX3 stringifies to vmx-3", hv: vimv1.VMX3, want: "vmx-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.hv.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseHardwareVersion(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    vimv1.HardwareVersion
		wantErr bool
	}{
		{name: "bare integer", in: "19", want: vimv1.VMX19},
		{name: "vmx-prefixed integer", in: "vmx-19", want: vimv1.VMX19},
		{name: "prefix is case-insensitive", in: "VMX-19", want: vimv1.VMX19},
		{name: "mixed-case prefix", in: "Vmx-21", want: vimv1.VMX21},
		{
			name: "leading zeros are still parsed as a number",
			in:   "vmx-007", want: vimv1.HardwareVersion(7),
		},
		{
			name: "zero parses to the invalid hardware version without an error",
			in:   "0", want: vimv1.HardwareVersion(0),
		},

		{name: "empty string is invalid", in: "", wantErr: true},
		{name: "non-numeric suffix is invalid", in: "vmx-abc", wantErr: true},
		{name: "missing hyphen is invalid", in: "vmx19", wantErr: true},
		{name: "negative number is invalid", in: "-1", wantErr: true},
		{name: "value overflowing uint8 is invalid", in: "vmx-256", wantErr: true},
		{name: "surrounding whitespace is invalid", in: " 19 ", wantErr: true},
		{name: "trailing garbage is invalid", in: "19x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := vimv1.ParseHardwareVersion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseHardwareVersion(%q) expected an error, got nil"+
						" (result %v)", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseHardwareVersion(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseHardwareVersion(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMustParseHardwareVersion(t *testing.T) {
	t.Run("valid input returns the parsed version", func(t *testing.T) {
		got := vimv1.MustParseHardwareVersion("vmx-20")
		if got != vimv1.VMX20 {
			t.Errorf("MustParseHardwareVersion() = %v, want %v", got, vimv1.VMX20)
		}
	})

	t.Run("invalid input panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("MustParseHardwareVersion() expected a panic for" +
					" invalid input, got none")
			}
		}()
		vimv1.MustParseHardwareVersion("not-a-version")
	})
}

func TestHardwareVersionTextMarshaling(t *testing.T) {
	t.Run("MarshalText round-trips through UnmarshalText", func(t *testing.T) {
		want := vimv1.VMX18

		b, err := want.MarshalText()
		if err != nil {
			t.Fatalf("MarshalText() unexpected error: %v", err)
		}
		if string(b) != "vmx-18" {
			t.Errorf("MarshalText() = %q, want %q", string(b), "vmx-18")
		}

		var got vimv1.HardwareVersion
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("UnmarshalText(%q) unexpected error: %v", b, err)
		}
		if got != want {
			t.Errorf("UnmarshalText(%q) = %v, want %v", b, got, want)
		}
	})

	t.Run("UnmarshalText propagates a parse error", func(t *testing.T) {
		var hv vimv1.HardwareVersion
		if err := hv.UnmarshalText([]byte("not-a-version")); err == nil {
			t.Error("UnmarshalText() expected an error, got nil")
		}
	})
}

func TestHardwareVersionJSONMarshaling(t *testing.T) {
	t.Run("MarshalJSON round-trips through UnmarshalJSON", func(t *testing.T) {
		want := vimv1.VMX17

		b, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("json.Marshal() unexpected error: %v", err)
		}
		if string(b) != `"vmx-17"` {
			t.Errorf("json.Marshal() = %s, want %s", b, `"vmx-17"`)
		}

		var got vimv1.HardwareVersion
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("json.Unmarshal(%s) unexpected error: %v", b, err)
		}
		if got != want {
			t.Errorf("json.Unmarshal(%s) = %v, want %v", b, got, want)
		}
	})

	t.Run("UnmarshalJSON propagates a parse error", func(t *testing.T) {
		var hv vimv1.HardwareVersion
		if err := json.Unmarshal([]byte(`"not-a-version"`), &hv); err == nil {
			t.Error("json.Unmarshal() expected an error, got nil")
		}
	})
}

func TestGetHardwareVersions(t *testing.T) {
	got := vimv1.GetHardwareVersions()

	if len(got) == 0 {
		t.Fatal("GetHardwareVersions() returned an empty slice")
	}

	for _, hv := range got {
		if !hv.IsSupported() {
			t.Errorf("GetHardwareVersions() included unsupported version %v", hv)
		}
	}

	first, last := got[0], got[len(got)-1]
	if first != vimv1.MinValidHardwareVersion {
		t.Errorf("GetHardwareVersions()[0] = %v, want %v",
			first, vimv1.MinValidHardwareVersion)
	}
	if last != vimv1.MaxValidHardwareVersion {
		t.Errorf("GetHardwareVersions()[last] = %v, want %v",
			last, vimv1.MaxValidHardwareVersion)
	}

	for _, hv := range got {
		if hv == vimv1.HardwareVersion(5) {
			t.Error("GetHardwareVersions() included the reserved/invalid version 5")
		}
	}

	t.Run("returns a copy, not the internal slice", func(t *testing.T) {
		got[0] = vimv1.HardwareVersion(0)

		again := vimv1.GetHardwareVersions()
		if again[0] != vimv1.MinValidHardwareVersion {
			t.Errorf("mutating the returned slice affected a subsequent"+
				" call: got %v, want %v", again[0], vimv1.MinValidHardwareVersion)
		}
	})
}
