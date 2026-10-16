// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/constants/testlabels"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("Netplan", Label(testlabels.API), func() {
	DescribeTable("DHCP overrides",
		func(dhcp4, dhcp6, ignoreDNS, ignore4, ignore6 bool, wantErr bool) {
			b := network.Bootstrap{
				Name: "eth0", DHCP4: dhcp4, DHCP6: dhcp6,
				IgnoreDHCPDNS: ignoreDNS, IgnoreDHCP4Routes: ignore4, IgnoreDHCP6Routes: ignore6,
				Routes: []network.NetworkInterfaceRoute{{To: "198.51.100.0/24", Via: "192.0.2.1"}},
			}
			config, err := network.NetPlanCustomization([]network.Bootstrap{b}, nil)
			if wantErr {
				Expect(err).To(MatchError(ContainSubstring("dual-stack DHCP requires identical DHCP overrides")))
				Expect(config).To(BeNil())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			eth := config.Ethernets["eth0"]
			for i, overrides := range []*netplan.DHCPOverrides{eth.Dhcp4Overrides, eth.Dhcp6Overrides} {
				enabled := []bool{dhcp4, dhcp6}[i]
				ignoreRoutes := []bool{ignore4, ignore6}[i]
				if !enabled || (!ignoreDNS && !ignoreRoutes) {
					Expect(overrides).To(BeNil())
					continue
				}
				Expect(overrides).ToNot(BeNil())
				if ignoreDNS {
					Expect(overrides.UseDNS).To(HaveValue(BeFalse()))
				} else {
					Expect(overrides.UseDNS).To(BeNil())
				}
				if ignoreRoutes {
					Expect(overrides.UseRoutes).To(HaveValue(BeFalse()))
				} else {
					Expect(overrides.UseRoutes).To(BeNil())
				}
			}
			if dhcp4 && dhcp6 {
				Expect(eth.Dhcp4Overrides).To(Equal(eth.Dhcp6Overrides))
			}
			Expect(eth.Routes).To(HaveLen(1))
			Expect(eth.Routes[0].To).To(HaveValue(Equal("198.51.100.0/24")))
			Expect(eth.Routes[0].Via).To(HaveValue(Equal("192.0.2.1")))
		},
		Entry("IPv4 DNS", true, false, true, false, false, false),
		Entry("IPv6 DNS", false, true, true, false, false, false),
		Entry("dual-stack DNS", true, true, true, false, false, false),
		Entry("IPv4 routes", true, false, false, true, false, false),
		Entry("IPv6 routes", false, true, false, false, true, false),
		Entry("dual-stack routes", true, true, false, true, true, false),
		Entry("dual-stack DNS and routes", true, true, true, true, true, false),
		Entry("static interfaces omit overrides", false, false, true, true, true, false),
		Entry("DHCP defaults and additional static routes", true, true, false, false, false, false),
		Entry("IPv4-only suppression on dual-stack fails", true, true, true, true, false, true),
		Entry("IPv6-only suppression on dual-stack fails", true, true, false, false, true, true),
	)

	It("renders explicit DNS and gateway overrides from the VM spec", func() {
		vm := &vmopv1.VirtualMachine{Spec: vmopv1.VirtualMachineSpec{
			Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{}},
			Network:   &vmopv1.VirtualMachineNetworkSpec{Nameservers: []string{"192.0.2.53"}},
		}}
		b := network.InterfaceBootstrap(context.Background(), vm,
			network.Bootstrap{DHCP4: true, DHCP6: true},
			vmopv1.VirtualMachineNetworkInterfaceSpec{Name: "eth0", Gateway4: "None", Gateway6: "None"})
		config, err := network.NetPlanCustomization([]network.Bootstrap{b}, nil)
		Expect(err).ToNot(HaveOccurred())
		data, err := netplan.MarshalYAML(netplan.Config{Network: *config})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(data)).To(ContainSubstring("dhcp4-overrides:\n        use-dns: false\n        use-routes: false"))
		Expect(string(data)).To(ContainSubstring("dhcp6-overrides:\n        use-dns: false\n        use-routes: false"))
		Expect(config.Ethernets["eth0"].Nameservers.Addresses).To(Equal([]string{"192.0.2.53"}))
	})

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
			bootstraps []network.Bootstrap
			vlans      []vmopv1.VirtualMachineNetworkVLANSpec
			config     *netplan.Network
			err        error
		)

		BeforeEach(func() {
			bootstraps = nil
			vlans = nil
			config = nil
		})

		JustBeforeEach(func() {
			config, err = network.NetPlanCustomization(bootstraps, vlans)
		})

		Context("IPv4/6 Static adapter", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
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
							{
								To:  "default",
								Via: "192.168.1.1",
							},
							{
								To:  "default",
								Via: "fe80::478:cbfd:455:c5fe",
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

				Expect(np.Routes).To(HaveLen(4))
				route0 := np.Routes[0]
				Expect(route0.To).To(HaveValue(Equal("185.107.56.0/24")))
				Expect(route0.Via).To(HaveValue(Equal("10.1.1.1")))
				Expect(route0.Metric).To(HaveValue(BeEquivalentTo(42)))
				route1 := np.Routes[1]
				Expect(route1.To).To(HaveValue(Equal("134.23.5.3.0/24")))
				Expect(route1.Via).To(HaveValue(Equal("20.2.2.2")))
				Expect(route1.Metric).To(BeNil())
				route2 := np.Routes[2]
				Expect(route2.To).To(HaveValue(Equal("default")))
				Expect(route2.Via).To(HaveValue(Equal("192.168.1.1")))
				Expect(route2.Metric).To(BeNil())
				route3 := np.Routes[3]
				Expect(route3.To).To(HaveValue(Equal("default")))
				Expect(route3.Via).To(HaveValue(Equal("fe80::478:cbfd:455:c5fe")))
				Expect(route3.Metric).To(BeNil())
			})

			Context("Gateway4/6 are disabled", func() {
				BeforeEach(func() {
					bootstraps[0].IPConfigs[0].Gateway = ""
					bootstraps[0].IPConfigs[1].Gateway = ""
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

			Context("MTU is zero", func() {
				BeforeEach(func() {
					bootstraps[0].MTU = 0
				})

				It("MTU is nil", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(config).ToNot(BeNil())
					Expect(config.Version).To(Equal(constants.NetPlanVersion))

					Expect(config.Ethernets).To(HaveLen(1))
					Expect(config.Ethernets).To(HaveKey(ifName))

					np := config.Ethernets[ifName]
					Expect(np.MTU).To(BeNil())
				})
			})
		})

		Context("IPv4/6 DHCP", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           true,
						DHCP6:           true,
						AcceptRA:        true,
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
				Expect(np.Addresses).To(BeEmpty())
				Expect(np.Dhcp4).To(HaveValue(BeTrue()))
				Expect(np.Dhcp6).To(HaveValue(BeTrue()))
				Expect(np.AcceptRa).To(HaveValue(BeTrue()))
				Expect(np.MTU).To(HaveValue(BeEquivalentTo(9000)))
				Expect(np.Nameservers.Addresses).To(Equal([]string{dnsServer1}))
				Expect(np.Nameservers.Search).To(Equal([]string{searchDomain1}))
				Expect(np.Routes).To(BeEmpty())
			})
		})

		Context("DHCP6 without AcceptRA (DHCPv6-only)", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP6:           true,
						AcceptRA:        false,
					},
				}
			})

			It("Dhcp6=true, AcceptRa=false", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.Ethernets).To(HaveLen(1))
				np := config.Ethernets[ifName]
				Expect(np.Dhcp6).To(HaveValue(BeTrue()))
				Expect(np.AcceptRa).To(HaveValue(BeFalse()))
			})
		})

		Context("NoIPAM", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						NoIPAM:          true,
						MTU:             9000,
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
				Expect(np.Addresses).To(BeEmpty())
				Expect(np.Dhcp4).To(HaveValue(BeFalse()))
				Expect(np.Dhcp6).To(HaveValue(BeFalse()))
				Expect(np.AcceptRa).To(HaveValue(BeFalse()))
			})
		})

		Context("IPv4-Only Static", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "192.168.1.100/24",
								IsIPv4:  true,
								Gateway: "192.168.1.1",
							},
						},
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           false,
						DHCP6:           false,
						MTU:             1500,
						Nameservers:     []string{dnsServer1},
					},
				}
			})

			It("returns success with IPv4-only configuration", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())

				np := config.Ethernets[ifName]
				Expect(np.Addresses).To(HaveLen(1))
				Expect(np.Addresses[0]).To(Equal(netplan.Address{String: ptr.To("192.168.1.100/24")}))
				Expect(np.Gateway4).To(HaveValue(Equal("192.168.1.1")))
				Expect(np.Gateway6).To(BeNil())
				Expect(np.Dhcp4).To(HaveValue(BeFalse()))
				Expect(np.Dhcp6).To(HaveValue(BeFalse()))
			})
		})

		Context("IPv6-Only Static", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "2001:db8::100/64",
								IsIPv4:  false,
								Gateway: "2001:db8::1",
							},
						},
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           false,
						DHCP6:           false,
						MTU:             1500,
						Nameservers:     []string{"2001:4860:4860::8888"},
					},
				}
			})

			It("returns success with IPv6-only configuration", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())

				np := config.Ethernets[ifName]
				Expect(np.Addresses).To(HaveLen(1))
				Expect(np.Addresses[0]).To(Equal(netplan.Address{String: ptr.To("2001:db8::100/64")}))
				Expect(np.Gateway4).To(BeNil())
				Expect(np.Gateway6).To(HaveValue(Equal("2001:db8::1")))
				Expect(np.Dhcp4).To(HaveValue(BeFalse()))
				Expect(np.Dhcp6).To(HaveValue(BeFalse()))
			})
		})

		Context("Multiple IPv6 Addresses", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "2001:db8::100/64",
								IsIPv4:  false,
								Gateway: "2001:db8::1",
							},
							{
								IPCIDR:  "2001:db8::101/64",
								IsIPv4:  false,
								Gateway: "2001:db8::1",
							},
						},
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           false,
						DHCP6:           false,
						MTU:             1500,
						Nameservers:     []string{"2001:4860:4860::8888"},
					},
				}
			})

			It("returns success with multiple IPv6 addresses", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.Version).To(Equal(constants.NetPlanVersion))

				Expect(config.Ethernets).To(HaveLen(1))
				Expect(config.Ethernets).To(HaveKey(ifName))

				np := config.Ethernets[ifName]
				Expect(np.Addresses).To(HaveLen(2))
				Expect(np.Addresses[0]).To(Equal(netplan.Address{String: ptr.To("2001:db8::100/64")}))
				Expect(np.Addresses[1]).To(Equal(netplan.Address{String: ptr.To("2001:db8::101/64")}))
				Expect(np.Gateway6).To(HaveValue(Equal("2001:db8::1")))
			})
		})

		Context("Multiple IPv4 Addresses", func() {
			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{
								IPCIDR:  "192.168.1.100/24",
								IsIPv4:  true,
								Gateway: "192.168.1.1",
							},
							{
								IPCIDR:  "192.168.1.101/24",
								IsIPv4:  true,
								Gateway: "192.168.1.1",
							},
						},
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           false,
						DHCP6:           false,
						MTU:             1500,
						Nameservers:     []string{dnsServer1},
					},
				}
			})

			It("returns success with multiple IPv4 addresses", func() {
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())
				Expect(config.Version).To(Equal(constants.NetPlanVersion))

				Expect(config.Ethernets).To(HaveLen(1))
				Expect(config.Ethernets).To(HaveKey(ifName))

				np := config.Ethernets[ifName]
				Expect(np.Addresses).To(HaveLen(2))
				Expect(np.Addresses[0]).To(Equal(netplan.Address{String: ptr.To("192.168.1.100/24")}))
				Expect(np.Addresses[1]).To(Equal(netplan.Address{String: ptr.To("192.168.1.101/24")}))
				Expect(np.Gateway4).To(HaveValue(Equal("192.168.1.1")))
			})
		})

		Context("VLANs", func() {
			const (
				vlanName1 = "vlan100"
				vlanID1   = int64(100)
				vlanName2 = "vlan200"
				vlanID2   = int64(200)
			)

			BeforeEach(func() {
				bootstraps = []network.Bootstrap{
					{
						MacAddress:      macAddr1,
						Name:            ifName,
						GuestDeviceName: guestDevName,
						DHCP4:           true,
						DHCP6:           false,
					},
				}
			})

			Context("Single VLAN", func() {
				BeforeEach(func() {
					vlans = []vmopv1.VirtualMachineNetworkVLANSpec{
						{
							Name: vlanName1,
							ID:   vlanID1,
							Link: ifName,
						},
					}
				})

				It("returns success with VLAN configured", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(config).ToNot(BeNil())
					Expect(config.Version).To(Equal(constants.NetPlanVersion))

					Expect(config.Ethernets).To(HaveLen(1))
					Expect(config.Ethernets).To(HaveKey(ifName))

					Expect(config.Vlans).To(HaveLen(1))
					Expect(config.Vlans).To(HaveKey(vlanName1))

					vlan := config.Vlans[vlanName1]
					Expect(vlan.ID).To(HaveValue(Equal(vlanID1)))
					Expect(vlan.Link).To(HaveValue(Equal(ifName)))
				})
			})

			Context("Multiple VLANs", func() {
				BeforeEach(func() {
					vlans = []vmopv1.VirtualMachineNetworkVLANSpec{
						{
							Name: vlanName1,
							ID:   vlanID1,
							Link: ifName,
						},
						{
							Name: vlanName2,
							ID:   vlanID2,
							Link: ifName,
						},
					}
				})

				It("returns success with multiple VLANs configured", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(config).ToNot(BeNil())
					Expect(config.Version).To(Equal(constants.NetPlanVersion))

					Expect(config.Ethernets).To(HaveLen(1))
					Expect(config.Vlans).To(HaveLen(2))

					Expect(config.Vlans).To(HaveKey(vlanName1))
					vlan1 := config.Vlans[vlanName1]
					Expect(vlan1.ID).To(HaveValue(Equal(vlanID1)))
					Expect(vlan1.Link).To(HaveValue(Equal(ifName)))

					Expect(config.Vlans).To(HaveKey(vlanName2))
					vlan2 := config.Vlans[vlanName2]
					Expect(vlan2.ID).To(HaveValue(Equal(vlanID2)))
					Expect(vlan2.Link).To(HaveValue(Equal(ifName)))
				})
			})

			Context("No VLANs", func() {
				BeforeEach(func() {
					vlans = nil
				})

				It("returns success without VLANs", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(config).ToNot(BeNil())
					Expect(config.Vlans).To(BeNil())
				})
			})

			Context("Empty VLANs list", func() {
				BeforeEach(func() {
					vlans = []vmopv1.VirtualMachineNetworkVLANSpec{}
				})

				It("returns success without VLANs", func() {
					Expect(err).ToNot(HaveOccurred())
					Expect(config).ToNot(BeNil())
					Expect(config.Vlans).To(BeNil())
				})
			})
		})
	})
})
