// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package v1alpha6

import "testing"

// vmxNet3TuningCEL holds the same boolean logic as the XValidation CEL on
// VirtualMachineNetworkInterfaceSpec (post-mutation object: type must be
// VMXNet3 whenever vmxnet3 is set).
func vmxNet3TuningCEL(iface VirtualMachineNetworkInterfaceSpec) bool {
	if iface.VMXNet3 == nil {
		return true
	}
	return iface.Type == VirtualMachineNetworkInterfaceTypeVMXNet3
}

func TestVMXNet3TuningCELMatchesXValidationRule(t *testing.T) {
	t.Parallel()
	vmx := &VirtualMachineNetworkInterfaceVMXNet3Spec{}
	cases := []struct {
		name  string
		iface VirtualMachineNetworkInterfaceSpec
		ok    bool
	}{
		{
			name: "no vmxnet3 block",
			iface: VirtualMachineNetworkInterfaceSpec{
				Name: "eth0",
				Type: VirtualMachineNetworkInterfaceTypeSRIOV,
			},
			ok: true,
		},
		{
			name: "vmxnet3 with VMXNet3 type",
			iface: VirtualMachineNetworkInterfaceSpec{
				Name:    "eth0",
				Type:    VirtualMachineNetworkInterfaceTypeVMXNet3,
				VMXNet3: vmx,
			},
			ok: true,
		},
		{
			name: "vmxnet3 with SRIOV type",
			iface: VirtualMachineNetworkInterfaceSpec{
				Name:    "eth0",
				Type:    VirtualMachineNetworkInterfaceTypeSRIOV,
				VMXNet3: vmx,
			},
			ok: false,
		},
		{
			name: "vmxnet3 with empty type",
			iface: VirtualMachineNetworkInterfaceSpec{
				Name:    "eth0",
				VMXNet3: vmx,
			},
			ok: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := vmxNet3TuningCEL(tc.iface); got != tc.ok {
				t.Fatalf("vmxNet3TuningCEL(...) = %v, want %v", got, tc.ok)
			}
		})
	}
}
