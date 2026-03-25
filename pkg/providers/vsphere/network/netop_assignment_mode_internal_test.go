// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
)

func TestEffectiveNetOPIPv4AssignmentMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		st   netopv1alpha1.NetworkInterfaceStatus
		want netopv1alpha1.NetworkInterfaceIPAssignmentMode
	}{
		{
			name: "explicit DHCP",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPAssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
		},
		{
			name: "explicit static pool",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPAssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
		},
		{
			name: "unset with IPv4 IPConfig implies static pool",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPConfigs: []netopv1alpha1.IPConfig{
					{IP: "10.0.0.1", IPFamily: corev1.IPv4Protocol},
				},
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
		},
		{
			name: "unset without IPv4 implies DHCP",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPConfigs: []netopv1alpha1.IPConfig{
					{IP: "2001:db8::1", IPFamily: corev1.IPv6Protocol},
				},
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := effectiveNetOPIPv4AssignmentMode(tc.st)
			if got != tc.want {
				t.Fatalf("effectiveNetOPIPv4AssignmentMode() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEffectiveNetOPIPv6AssignmentMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		st   netopv1alpha1.NetworkInterfaceStatus
		want netopv1alpha1.NetworkInterfaceIPAssignmentMode
	}{
		{
			name: "explicit DHCP",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPv6AssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
		},
		{
			name: "explicit static pool",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPv6AssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
		},
		{
			name: "explicit none",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPv6AssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeNone,
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeNone,
		},
		{
			name: "unset defaults to none even if IPv6 appears in IPConfigs",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPConfigs: []netopv1alpha1.IPConfig{
					{IP: "2001:db8::1", IPFamily: corev1.IPv6Protocol},
				},
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeNone,
		},
		{
			name: "unset without IPv6 implies none",
			st: netopv1alpha1.NetworkInterfaceStatus{
				IPConfigs: []netopv1alpha1.IPConfig{
					{IP: "10.0.0.1", IPFamily: corev1.IPv4Protocol},
				},
			},
			want: netopv1alpha1.NetworkInterfaceIPAssignmentModeNone,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := effectiveNetOPIPv6AssignmentMode(tc.st)
			if got != tc.want {
				t.Fatalf("effectiveNetOPIPv6AssignmentMode() = %q, want %q", got, tc.want)
			}
		})
	}
}
