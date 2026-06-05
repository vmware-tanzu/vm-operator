// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("NetOP assignment mode helpers",
	Label(testlabels.API),
	func() {
		DescribeTable("EffectiveNetOPIPv4AssignmentMode",
			func(st netopv1alpha1.NetworkInterfaceStatus, want netopv1alpha1.NetworkInterfaceIPAssignmentMode) {
				Expect(network.EffectiveNetOPIPv4AssignmentMode(st)).To(Equal(want))
			},
			Entry("explicit DHCP",
				netopv1alpha1.NetworkInterfaceStatus{
					IPAssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP),
			Entry("explicit static pool",
				netopv1alpha1.NetworkInterfaceStatus{
					IPAssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool),
			Entry("unset with IPv4 IPConfig implies static pool",
				netopv1alpha1.NetworkInterfaceStatus{
					IPConfigs: []netopv1alpha1.IPConfig{
						{IP: "10.0.0.1", IPFamily: corev1.IPv4Protocol},
					},
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool),
			Entry("unset without IPv4 implies DHCP",
				netopv1alpha1.NetworkInterfaceStatus{
					IPConfigs: []netopv1alpha1.IPConfig{
						{IP: "2001:db8::1", IPFamily: corev1.IPv6Protocol},
					},
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP),
		)

		DescribeTable("EffectiveNetOPIPv6AssignmentMode",
			func(st netopv1alpha1.NetworkInterfaceStatus, want netopv1alpha1.NetworkInterfaceIPAssignmentMode) {
				Expect(network.EffectiveNetOPIPv6AssignmentMode(st)).To(Equal(want))
			},
			Entry("explicit DHCP",
				netopv1alpha1.NetworkInterfaceStatus{
					IPv6AssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP,
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP),
			Entry("explicit static pool",
				netopv1alpha1.NetworkInterfaceStatus{
					IPv6AssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool,
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool),
			Entry("explicit none",
				netopv1alpha1.NetworkInterfaceStatus{
					IPv6AssignmentMode: netopv1alpha1.NetworkInterfaceIPAssignmentModeNone,
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeNone),
			Entry("unset defaults to none even if IPv6 appears in IPConfigs",
				netopv1alpha1.NetworkInterfaceStatus{
					IPConfigs: []netopv1alpha1.IPConfig{
						{IP: "2001:db8::1", IPFamily: corev1.IPv6Protocol},
					},
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeNone),
			Entry("unset without IPv6 implies none",
				netopv1alpha1.NetworkInterfaceStatus{
					IPConfigs: []netopv1alpha1.IPConfig{
						{IP: "10.0.0.1", IPFamily: corev1.IPv4Protocol},
					},
				},
				netopv1alpha1.NetworkInterfaceIPAssignmentModeNone),
		)

		DescribeTable("IPAMModesToNetOPInterfaceIPFamilyPolicy",
			func(modes []corev1.IPFamily, want netopv1alpha1.NetworkInterfaceIPFamilyPolicy) {
				Expect(network.IPAMModesToNetOPInterfaceIPFamilyPolicy(modes)).To(Equal(want))
			},
			Entry("nil slice returns empty string", []corev1.IPFamily(nil), netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")),
			Entry("empty slice returns empty string", []corev1.IPFamily{}, netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")),
			Entry("IPv4 only", []corev1.IPFamily{corev1.IPv4Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only),
			Entry("IPv6 only", []corev1.IPFamily{corev1.IPv6Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only),
			Entry("dual stack order v4 v6",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
				netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack),
			Entry("dual stack order v6 v4",
				[]corev1.IPFamily{corev1.IPv6Protocol, corev1.IPv4Protocol},
				netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack),
		)

		DescribeTable("IPAMModesToVPCInterfaceIPType",
			func(modes []corev1.IPFamily, want vpcv1alpha1.IPAddressType) {
				Expect(network.IPAMModesToVPCInterfaceIPType(modes)).To(Equal(want))
			},
			Entry("nil slice returns empty string", []corev1.IPFamily(nil), vpcv1alpha1.IPAddressType("")),
			Entry("empty slice returns empty string", []corev1.IPFamily{}, vpcv1alpha1.IPAddressType("")),
			Entry("IPv4 only", []corev1.IPFamily{corev1.IPv4Protocol}, vpcv1alpha1.IPAddressTypeIPv4),
			Entry("IPv6 only", []corev1.IPFamily{corev1.IPv6Protocol}, vpcv1alpha1.IPAddressTypeIPv6),
			Entry("dual stack order v4 v6",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
				vpcv1alpha1.IPAddressTypeIPv4IPv6),
			Entry("dual stack order v6 v4",
				[]corev1.IPFamily{corev1.IPv6Protocol, corev1.IPv4Protocol},
				vpcv1alpha1.IPAddressTypeIPv4IPv6),
		)

		DescribeTable("IPAMModesToVPCStaticIPAllocationType",
			func(modes []corev1.IPFamily, dhcp4, dhcp6 *bool, want vpcv1alpha1.StaticIPAllocationType) {
				Expect(network.IPAMModesToVPCStaticIPAllocationType(modes, dhcp4, dhcp6)).To(Equal(want))
			},
			// Brownfield: nil/empty modes always returns unset regardless of DHCP flags.
			Entry("nil modes returns empty (brownfield)",
				[]corev1.IPFamily(nil), (*bool)(nil), (*bool)(nil), vpcv1alpha1.StaticIPAllocationType("")),
			Entry("empty modes returns empty (brownfield)",
				[]corev1.IPFamily{}, (*bool)(nil), (*bool)(nil), vpcv1alpha1.StaticIPAllocationType("")),
			// Both nil → no explicit intent → NSX derives.
			Entry("IPv4 only, both nil → unset",
				[]corev1.IPFamily{corev1.IPv4Protocol}, (*bool)(nil), (*bool)(nil),
				vpcv1alpha1.StaticIPAllocationType("")),
			Entry("dual stack, both nil → unset",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, (*bool)(nil), (*bool)(nil),
				vpcv1alpha1.StaticIPAllocationType("")),
			// DHCP4=false (explicit static) cases.
			Entry("IPv4 only, DHCP4=false → static IPv4",
				[]corev1.IPFamily{corev1.IPv4Protocol}, ptr.To(false), (*bool)(nil),
				vpcv1alpha1.StaticIPAllocationTypeIPv4),
			Entry("IPv4 only, DHCP4=true → None",
				[]corev1.IPFamily{corev1.IPv4Protocol}, ptr.To(true), (*bool)(nil),
				vpcv1alpha1.StaticIPAllocationTypeNone),
			// DHCP6=false (explicit static) cases.
			Entry("IPv6 only, DHCP6=false → static IPv6",
				[]corev1.IPFamily{corev1.IPv6Protocol}, (*bool)(nil), ptr.To(false),
				vpcv1alpha1.StaticIPAllocationTypeIPv6),
			Entry("IPv6 only, DHCP6=true → None",
				[]corev1.IPFamily{corev1.IPv6Protocol}, (*bool)(nil), ptr.To(true),
				vpcv1alpha1.StaticIPAllocationTypeNone),
			// Dual stack: only explicitly-static families are included.
			Entry("dual stack, DHCP4=true, DHCP6=nil → None (v4 DHCP, v6 unknown)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, ptr.To(true), (*bool)(nil),
				vpcv1alpha1.StaticIPAllocationTypeNone),
			Entry("dual stack, DHCP4=false, DHCP6=nil → IPv4 (v4 static, v6 unknown)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, ptr.To(false), (*bool)(nil),
				vpcv1alpha1.StaticIPAllocationTypeIPv4),
			Entry("dual stack, DHCP4=nil, DHCP6=true → None (v4 unknown, v6 DHCP)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, (*bool)(nil), ptr.To(true),
				vpcv1alpha1.StaticIPAllocationTypeNone),
			Entry("dual stack, DHCP4=nil, DHCP6=false → IPv6 (v4 unknown, v6 static)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, (*bool)(nil), ptr.To(false),
				vpcv1alpha1.StaticIPAllocationTypeIPv6),
			Entry("dual stack, DHCP4=false, DHCP6=false → IPv4IPv6 (both static)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, ptr.To(false), ptr.To(false),
				vpcv1alpha1.StaticIPAllocationTypeIPv4IPv6),
			Entry("dual stack, DHCP4=true, DHCP6=true → None (both DHCP)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, ptr.To(true), ptr.To(true),
				vpcv1alpha1.StaticIPAllocationTypeNone),
			Entry("dual stack, DHCP4=true, DHCP6=false → IPv6 (v4 DHCP, v6 static)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, ptr.To(true), ptr.To(false),
				vpcv1alpha1.StaticIPAllocationTypeIPv6),
			Entry("dual stack, DHCP4=false, DHCP6=true → IPv4 (v4 static, v6 DHCP)",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol}, ptr.To(false), ptr.To(true),
				vpcv1alpha1.StaticIPAllocationTypeIPv4),
			Entry("dual stack order v6 v4, DHCP4=false, DHCP6=false → IPv4IPv6",
				[]corev1.IPFamily{corev1.IPv6Protocol, corev1.IPv4Protocol}, ptr.To(false), ptr.To(false),
				vpcv1alpha1.StaticIPAllocationTypeIPv4IPv6),
		)

	})
