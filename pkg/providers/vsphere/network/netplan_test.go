// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("Netplan", func() {
	const (
		ifName        = "my-interface"
		guestDevName  = "eth42"
		macAddr1      = "50-8A-80-9D-28-22"
		macAddr1Norm  = "50:8a:80:9d:28:22"
		ipv4Gateway   = "192.168.1.1"
		ipv4          = "192.168.1.10"
		ipv4CIDR      = ipv4 + "/24"
		ipv6Gateway   = "fd8e:b5a0:f172:123::1"
		ipv6          = "fd8e:b5a0:f172:123::f"
		ipv6Subnet    = 48
		dnsServer1    = "9.9.9.9"
		searchDomain1 = "foobar.local"
	)

	Context("NetPlanCustomization", func() {

		var (
			results network.NetworkInterfaceResults
			config  *netplan.Network
			err     error
		)

		BeforeEach(func() {
			results = network.NetworkInterfaceResults{}
			config = nil
		})

		JustBeforeEach(func() {
			config, err = network.NetPlanCustomization(results)
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
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           false,
						DHCP6:           false,
						MTU:             1500,
						Nameservers:     []string{dnsServer1},
						SearchDomains:   []string{searchDomain1},
						Routes: []network.NetworkInterfaceRoute{
							{
								To:     "185.107.56.0/24",
								Via:    "10.1.1.1",
								Metric: 42,
							},
							{
								To:  "134.23.5.3.0/24",
								Via: "20.2.2.2",
							},
						},
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.Version).To(Equal(constants.NetPlanVersion))

				Expect(config.Ethernets).To(HaveLen(1))
				Expect(config.Ethernets).To(HaveKey(ifName))

				np := config.Ethernets[ifName]
				Expect(np.Match.Macaddress).To(HaveValue(Equal(macAddr1Norm)))
				Expect(np.SetName).To(HaveValue(Equal(guestDevName)))
				Expect(np.Dhcp4).To(HaveValue(BeFalse()))
				Expect(np.Dhcp6).To(HaveValue(BeFalse()))
				Expect(np.AcceptRa).To(HaveValue(BeFalse()))
				Expect(np.Addresses).To(HaveLen(2))
				Expect(np.Addresses[0]).To(Equal(netplan.Address{String: ptr.To(ipv4CIDR)}))
				Expect(np.Addresses[1]).To(Equal(netplan.Address{String: ptr.To(ipv6 + fmt.Sprintf("/%d", ipv6Subnet))}))
				Expect(np.Gateway4).To(HaveValue(Equal(ipv4Gateway)))
				Expect(np.Gateway6).To(HaveValue(Equal(ipv6Gateway)))
				Expect(np.MTU).To(HaveValue(BeEquivalentTo(1500)))
				Expect(np.Nameservers.Addresses).To(Equal([]string{dnsServer1}))
				Expect(np.Nameservers.Search).To(Equal([]string{searchDomain1}))

				Expect(np.Routes).To(HaveLen(2))
				route0 := np.Routes[0]
				Expect(route0.To).To(HaveValue(Equal("185.107.56.0/24")))
				Expect(route0.Via).To(HaveValue(Equal("10.1.1.1")))
				Expect(route0.Metric).To(HaveValue(BeEquivalentTo(42)))
				route1 := np.Routes[1]
				Expect(route1.To).To(HaveValue(Equal("134.23.5.3.0/24")))
				Expect(route1.Via).To(HaveValue(Equal("20.2.2.2")))
				Expect(route1.Metric).To(BeNil())
			})

			Context("Gateway4/6 are disabled", func() {
				BeforeEach(func() {
					results.Results[0].IPConfigs[0].Gateway = ""
					results.Results[0].IPConfigs[1].Gateway = ""
				})

				It("gateways are nil", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(config).ToNot(BeNil())
					Expect(config.Version).To(Equal(constants.NetPlanVersion))

					Expect(config.Ethernets).To(HaveLen(1))
					Expect(config.Ethernets).To(HaveKey(ifName))

					np := config.Ethernets[ifName]
					Expect(np.Gateway4).To(BeNil())
					Expect(np.Gateway6).To(BeNil())
				})
			})
		})

		Context("IPv4/6 DHCP", func() {
			BeforeEach(func() {
				results.Results = []network.NetworkInterfaceResult{
					{
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           true,
						DHCP6:           true,
						MTU:             9000,
						Nameservers:     []string{dnsServer1},
						SearchDomains:   []string{searchDomain1},
					},
				}
			})

			It("returns success", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.Version).To(Equal(constants.NetPlanVersion))

				Expect(config.Ethernets).To(HaveLen(1))
				Expect(config.Ethernets).To(HaveKey(ifName))

				np := config.Ethernets[ifName]
				Expect(np.Match.Macaddress).To(HaveValue(Equal(macAddr1Norm)))
				Expect(np.SetName).To(HaveValue(Equal(guestDevName)))
				Expect(np.Dhcp4).To(HaveValue(BeTrue()))
				Expect(np.Dhcp6).To(HaveValue(BeTrue()))
				Expect(np.AcceptRa).To(HaveValue(BeTrue()))
				Expect(np.MTU).To(HaveValue(BeEquivalentTo(9000)))
				Expect(np.Nameservers.Addresses).To(Equal([]string{dnsServer1}))
				Expect(np.Nameservers.Search).To(Equal([]string{searchDomain1}))
				Expect(np.Routes).To(BeEmpty())
			})
		})
	})
})
