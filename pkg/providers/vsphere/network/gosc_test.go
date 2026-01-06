// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"

	vimtypes "github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
)

var _ = Describe("GOSC", func() {
	const (
		macAddr1 = "50:8A:80:9D:28:22"

		ipv4Gateway = "192.168.1.1"
		ipv4        = "192.168.1.10"
		ipv4CIDR    = ipv4 + "/24"
		ipv6Gateway = "fd8e:b5a0:f172:123::1"
		ipv6        = "fd8e:b5a0:f172:123::f"
		ipv6Subnet  = 48

		dnsServer1 = "9.9.9.9"
	)

	Context("GuestOSCustomization", func() {

		var (
			results         network.NetworkInterfaceResults
			adapterMappings []vimtypes.CustomizationAdapterMapping
			err             error
		)

		BeforeEach(func() {
			results = network.NetworkInterfaceResults{}
		})

		JustBeforeEach(func() {
			adapterMappings, err = network.GuestOSCustomization(results, logr.Discard())
		})

		Context("IPv4/6 Static adapter", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  ipv4CIDR,
								IsIPv4:  true,
								Gateway: ipv4Gateway,
							},
							{
								IPCIDR:  ipv6 + fmt.Sprintf("/%d", ipv6Subnet),
								IsIPv4:  false,
								Gateway: ipv6Gateway,
							},
						},
						MacAddress:  macAddr1,
						Name:        "eth0",
						DHCP4:       false,
						DHCP6:       false,
						MTU:         1500, // AFAIK not supported via GOSC
						Nameservers: []string{dnsServer1},
						Routes:      nil, // AFAIK not supported via GOSC
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				Expect(adapter.Gateway).To(Equal([]string{ipv4Gateway}))
				Expect(adapter.SubnetMask).To(Equal("255.255.255.0"))
				Expect(adapter.DnsServerList).To(Equal([]string{dnsServer1}))
				Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIp{}))
				fixedIP := adapter.Ip.(*vimtypes.CustomizationFixedIp)
				Expect(fixedIP.IpAddress).To(Equal(ipv4))

				ipv6Spec := adapter.IpV6Spec
				Expect(ipv6Spec).ToNot(BeNil())
				Expect(ipv6Spec.Gateway).To(Equal([]string{ipv6Gateway}))
				Expect(ipv6Spec.Ip).To(HaveLen(1))
				Expect(ipv6Spec.Ip[0]).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIpV6{}))
				addressSpec := ipv6Spec.Ip[0].(*vimtypes.CustomizationFixedIpV6)
				Expect(addressSpec.IpAddress).To(Equal(ipv6))
				Expect(addressSpec.SubnetMask).To(BeEquivalentTo(ipv6Subnet))
			})

			Context("Gateway4/6 are disabled", func() {
				BeforeEach(func() {
					results.Results[0].IPConfigs[0].Gateway = ""
					results.Results[0].IPConfigs[1].Gateway = ""
				})

				It("returns success", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(adapterMappings).To(HaveLen(1))
					mapping := adapterMappings[0]

					adapter := mapping.Adapter
					Expect(adapter.Gateway).To(BeNil())
					Expect(adapter.SubnetMask).To(Equal("255.255.255.0"))
					Expect(adapter.DnsServerList).To(Equal([]string{dnsServer1}))
					Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIp{}))
					fixedIP := adapter.Ip.(*vimtypes.CustomizationFixedIp)
					Expect(fixedIP.IpAddress).To(Equal(ipv4))

					ipv6Spec := adapter.IpV6Spec
					Expect(ipv6Spec).ToNot(BeNil())
					Expect(ipv6Spec.Gateway).To(BeNil())
					Expect(ipv6Spec.Ip).To(HaveLen(1))
					Expect(ipv6Spec.Ip[0]).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIpV6{}))
					addressSpec := ipv6Spec.Ip[0].(*vimtypes.CustomizationFixedIpV6)
					Expect(addressSpec.IpAddress).To(Equal(ipv6))
					Expect(addressSpec.SubnetMask).To(BeEquivalentTo(ipv6Subnet))
				})
			})
		})

		Context("IPv4/6 DHCP", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						MacAddress:  macAddr1,
						Name:        "eth0",
						DHCP4:       true,
						DHCP6:       true,
						MTU:         1500, // AFAIK not support via GOSC
						Nameservers: []string{dnsServer1},
						Routes:      nil, // AFAIK not support via GOSC
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				Expect(adapter.Gateway).To(BeEmpty())
				Expect(adapter.SubnetMask).To(BeEmpty())

				Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationDhcpIpGenerator{}))
				ipv6Spec := adapter.IpV6Spec
				Expect(ipv6Spec).ToNot(BeNil())
				Expect(ipv6Spec.Ip).To(HaveLen(1))
				Expect(ipv6Spec.Ip[0]).To(BeAssignableToTypeOf(&vimtypes.CustomizationDhcpIpV6Generator{}))
			})
		})

		Context("NoIPAM", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						MacAddress: macAddr1,
						Name:       "eth0",
						NoIPAM:     true,
						MTU:        1500, // AFAIK not support via GOSC
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				Expect(adapter.Gateway).To(BeEmpty())
				Expect(adapter.SubnetMask).To(BeEmpty())
				Expect(adapter.Ip).To(BeNil())
				Expect(adapter.IpV6Spec).To(BeNil())
			})
		})

		Context("TC-001: IPv4-Only Static", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "192.168.1.100/24",
								IsIPv4:  true,
								Gateway: "192.168.1.1",
							},
						},
						MacAddress:  macAddr1,
						Name:        "eth0",
						DHCP4:       false,
						DHCP6:       false,
						MTU:         1500,
						Nameservers: []string{dnsServer1},
					},
				}
			})

			It("returns success with IPv4-only configuration", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				Expect(adapter.Gateway).To(Equal([]string{"192.168.1.1"}))
				Expect(adapter.SubnetMask).To(Equal("255.255.255.0"))
				Expect(adapter.DnsServerList).To(Equal([]string{dnsServer1}))
				Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIp{}))
				fixedIP := adapter.Ip.(*vimtypes.CustomizationFixedIp)
				Expect(fixedIP.IpAddress).To(Equal("192.168.1.100"))
				Expect(adapter.IpV6Spec).To(BeNil())
			})
		})

		Context("TC-002: IPv6-Only Static with IPv6-Only Fix", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "2001:db8::100/64",
								IsIPv4:  false,
								Gateway: "2001:db8::1",
							},
						},
						MacAddress:  macAddr1,
						Name:        "eth0",
						DHCP4:       false,
						DHCP6:       false,
						MTU:         1500,
						Nameservers: []string{"2001:4860:4860::8888"},
					},
				}
			})

			It("returns success with IPv6-only fix applied", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				// IPv6-only fix: adapter.Ip should be set to DHCP even though no IPv4 needed
				Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationDhcpIpGenerator{}))
				Expect(adapter.IpV6Spec).ToNot(BeNil())
				Expect(adapter.IpV6Spec.Gateway).To(Equal([]string{"2001:db8::1"}))
				Expect(adapter.IpV6Spec.Ip).To(HaveLen(1))
				Expect(adapter.IpV6Spec.Ip[0]).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIpV6{}))
				addressSpec := adapter.IpV6Spec.Ip[0].(*vimtypes.CustomizationFixedIpV6)
				Expect(addressSpec.IpAddress).To(Equal("2001:db8::100"))
				Expect(addressSpec.SubnetMask).To(BeEquivalentTo(64))
			})
		})

		Context("TC-021: DHCP4 + Static IPv6", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "2001:db8::100/64",
								IsIPv4:  false,
								Gateway: "2001:db8::1",
							},
						},
						MacAddress:  macAddr1,
						Name:        "eth0",
						DHCP4:       true,
						DHCP6:       false,
						MTU:         1500,
						Nameservers: []string{dnsServer1},
					},
				}
			})

			It("returns success with DHCP4 and static IPv6", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationDhcpIpGenerator{}))
				Expect(adapter.IpV6Spec).ToNot(BeNil())
				Expect(adapter.IpV6Spec.Gateway).To(Equal([]string{"2001:db8::1"}))
				Expect(adapter.IpV6Spec.Ip).To(HaveLen(1))
				Expect(adapter.IpV6Spec.Ip[0]).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIpV6{}))
				addressSpec := adapter.IpV6Spec.Ip[0].(*vimtypes.CustomizationFixedIpV6)
				Expect(addressSpec.IpAddress).To(Equal("2001:db8::100"))
				Expect(addressSpec.SubnetMask).To(BeEquivalentTo(64))
			})
		})

		Context("TC-022: Static IPv4 + DHCP6", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "192.168.1.100/24",
								IsIPv4:  true,
								Gateway: "192.168.1.1",
							},
						},
						MacAddress:  macAddr1,
						Name:        "eth0",
						DHCP4:       false,
						DHCP6:       true,
						MTU:         1500,
						Nameservers: []string{dnsServer1},
					},
				}
			})

			It("returns success with static IPv4 and DHCP6", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(adapterMappings).To(HaveLen(1))
				mapping := adapterMappings[0]

				adapter := mapping.Adapter
				Expect(mapping.MacAddress).To(Equal(macAddr1))
				Expect(adapter.Gateway).To(Equal([]string{"192.168.1.1"}))
				Expect(adapter.SubnetMask).To(Equal("255.255.255.0"))
				Expect(adapter.Ip).To(BeAssignableToTypeOf(&vimtypes.CustomizationFixedIp{}))
				fixedIP := adapter.Ip.(*vimtypes.CustomizationFixedIp)
				Expect(fixedIP.IpAddress).To(Equal("192.168.1.100"))
				Expect(adapter.IpV6Spec).ToNot(BeNil())
				Expect(adapter.IpV6Spec.Ip).To(HaveLen(1))
				Expect(adapter.IpV6Spec.Ip[0]).To(BeAssignableToTypeOf(&vimtypes.CustomizationDhcpIpV6Generator{}))
			})
		})
	})
})
