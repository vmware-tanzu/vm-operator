// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
)

var _ = Describe("NetOP assignment mode helpers",
	Label(testlabels.API),
	func() {
		DescribeTable("EffectiveNetOPIPv4AssignmentMode",
			func(st netopv1alpha1.NetworkInterfaceStatus, want netopv1alpha1.NetworkInterfaceIPAssignmentMode) {
				Expect(effectiveNetOPIPv4AssignmentMode(st)).To(Equal(want))
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
				Expect(effectiveNetOPIPv6AssignmentMode(st)).To(Equal(want))
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

		DescribeTable("NetOPInterfaceIPFamilyPolicyFromIPAMModes",
			func(modes []corev1.IPFamily, want netopv1alpha1.NetworkInterfaceIPFamilyPolicy) {
				Expect(NetOPInterfaceIPFamilyPolicyFromIPAMModes(modes)).To(Equal(want))
			},
			Entry("nil slice defaults to IPv4Only", []corev1.IPFamily(nil), netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only),
			Entry("empty slice defaults to IPv4Only", []corev1.IPFamily{}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only),
			Entry("IPv4 only", []corev1.IPFamily{corev1.IPv4Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv4Only),
			Entry("IPv6 only", []corev1.IPFamily{corev1.IPv6Protocol}, netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only),
			Entry("dual stack order v4 v6",
				[]corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
				netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack),
			Entry("dual stack order v6 v4",
				[]corev1.IPFamily{corev1.IPv6Protocol, corev1.IPv4Protocol},
				netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack),
		)

		Describe("syncNetOPIPFamilyPolicyFromIPAMModes", func() {
			It("clears NetOP IPFamilyPolicy when VM IPAMModes is nil and NetOP had a policy", func() {
				netIf := &netopv1alpha1.NetworkInterface{
					Spec: netopv1alpha1.NetworkInterfaceSpec{
						IPFamilyPolicy: netopv1alpha1.NetworkInterfaceIPFamilyPolicyDualStack,
					},
				}
				iface := &vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth0"}
				syncNetOPIPFamilyPolicyFromIPAMModes(iface, netIf)
				Expect(netIf.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")))
			})

			It("clears NetOP IPFamilyPolicy when VM IPAMModes is empty slice", func() {
				netIf := &netopv1alpha1.NetworkInterface{
					Spec: netopv1alpha1.NetworkInterfaceSpec{
						IPFamilyPolicy: netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only,
					},
				}
				iface := &vmopv1.VirtualMachineNetworkInterfaceSpec{
					Name:      "eth0",
					IPAMModes: []corev1.IPFamily{},
				}
				syncNetOPIPFamilyPolicyFromIPAMModes(iface, netIf)
				Expect(netIf.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicy("")))
			})

			It("sets NetOP IPFamilyPolicy when VM IPAMModes is non-empty", func() {
				netIf := &netopv1alpha1.NetworkInterface{}
				iface := &vmopv1.VirtualMachineNetworkInterfaceSpec{
					Name:      "eth0",
					IPAMModes: []corev1.IPFamily{corev1.IPv6Protocol},
				}
				syncNetOPIPFamilyPolicyFromIPAMModes(iface, netIf)
				Expect(netIf.Spec.IPFamilyPolicy).To(Equal(netopv1alpha1.NetworkInterfaceIPFamilyPolicyIPv6Only))
			})
		})
	})
