// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/vmware/govmomi/vim25/types"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/network"
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
			adapterMappings []types.CustomizationAdapterMapping
			err             error
		)

		BeforeEach(func() {
			results.Results = nil
		})

		JustBeforeEach(func() {
			adapterMappings, err = network.GuestOSCustomization(results)
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
				Expect(adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationFixedIp{}))
				fixedIP := adapter.Ip.(*types.CustomizationFixedIp)
				Expect(fixedIP.IpAddress).To(Equal(ipv4))

				ipv6Spec := adapter.IpV6Spec
				Expect(ipv6Spec).ToNot(BeNil())
				Expect(ipv6Spec.Gateway).To(Equal([]string{ipv6Gateway}))
				Expect(ipv6Spec.Ip).To(HaveLen(1))
				Expect(ipv6Spec.Ip[0]).To(BeAssignableToTypeOf(&types.CustomizationFixedIpV6{}))
				addressSpec := ipv6Spec.Ip[0].(*types.CustomizationFixedIpV6)
				Expect(addressSpec.IpAddress).To(Equal(ipv6))
				Expect(addressSpec.SubnetMask).To(BeEquivalentTo(ipv6Subnet))
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

				Expect(adapter.Ip).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpGenerator{}))
				ipv6Spec := adapter.IpV6Spec
				Expect(ipv6Spec).ToNot(BeNil())
				Expect(ipv6Spec.Ip).To(HaveLen(1))
				Expect(ipv6Spec.Ip[0]).To(BeAssignableToTypeOf(&types.CustomizationDhcpIpV6Generator{}))
			})
		})
	})
})
