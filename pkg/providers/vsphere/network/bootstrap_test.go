// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

//nolint:goconst
package network_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	netopv1alpha1 "github.com/vmware-tanzu/net-operator-api/api/v1alpha1"
	vpcv1alpha1 "github.com/vmware-tanzu/nsx-operator/pkg/apis/vpc/v1alpha1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	ncpv1alpha1 "github.com/vmware-tanzu/vm-operator/external/ncp/api/v1alpha1"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = Describe("InterfaceBootstrap", func() {
	var (
		ctx           context.Context
		vm            *vmopv1.VirtualMachine
		initial       network.Bootstrap
		interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec
		bootstrap     network.Bootstrap
	)

	BeforeEach(func() {
		ctx = context.Background()
		initial = network.Bootstrap{}
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
			},
		}
		interfaceSpec = vmopv1.VirtualMachineNetworkInterfaceSpec{
			Name: "eth0",
		}
	})

	JustBeforeEach(func() {
		vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, interfaceSpec)
		bootstrap = network.InterfaceBootstrap(ctx, vm, initial, interfaceSpec)
	})

	Context("Name", func() {
		It("copies Name from interfaceSpec", func() {
			Expect(bootstrap.Name).To(Equal("eth0"))
		})
	})

	Context("GuestDeviceName", func() {
		When("GuestDeviceName is set in interfaceSpec", func() {
			BeforeEach(func() {
				interfaceSpec.GuestDeviceName = "ens3"
			})
			It("uses the value as-is", func() {
				Expect(bootstrap.GuestDeviceName).To(Equal("ens3"))
			})
		})

		When("GuestDeviceName is empty in interfaceSpec", func() {
			It("falls back to interfaceSpec.Name", func() {
				Expect(bootstrap.GuestDeviceName).To(Equal("eth0"))
			})
		})
	})

	Context("MacAddress", func() {
		When("initial.MacAddress is uppercase", func() {
			BeforeEach(func() {
				initial.MacAddress = "AA:BB:CC:DD:EE:FF"
			})
			It("lowercases the bootstrap", func() {
				Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			})
		})

		When("initial.MacAddress is empty", func() {
			It("bootstrap is empty without panicking", func() {
				Expect(bootstrap.MacAddress).To(BeEmpty())
			})
		})

		When("interfaceSpec.MACAddr is set but initial.MacAddress differs", func() {
			BeforeEach(func() {
				initial.MacAddress = "aa:bb:cc:dd:ee:ff"
				interfaceSpec.MACAddr = "11:22:33:44:55:66"
			})
			It("InterfaceBootstrap ignores interfaceSpec.MACAddr; MAC comes solely from initial", func() {
				Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			})
		})
	})

	Context("MTU", func() {
		When("interfaceSpec.MTU is non-nil", func() {
			BeforeEach(func() {
				interfaceSpec.MTU = ptr.To[int64](9000)
			})
			It("copies the value", func() {
				Expect(bootstrap.MTU).To(BeEquivalentTo(9000))
			})
		})

		When("interfaceSpec.MTU is nil", func() {
			It("bootstrap.MTU stays at the initial value (zero)", func() {
				Expect(bootstrap.MTU).To(BeZero())
			})
		})

		When("interfaceSpec.MTU is an explicit zero pointer", func() {
			BeforeEach(func() {
				interfaceSpec.MTU = ptr.To[int64](0)
			})
			It("copies zero", func() {
				Expect(bootstrap.MTU).To(BeZero())
			})
		})
	})

	Context("DHCP4", func() {
		When("spec=*true, initial=false", func() {
			BeforeEach(func() {
				interfaceSpec.DHCP4 = ptr.To(true)
			})
			It("enables DHCP4", func() {
				Expect(bootstrap.DHCP4).To(BeTrue())
			})
		})

		When("spec=nil, initial=true", func() {
			BeforeEach(func() {
				initial.DHCP4 = true
			})
			It("defers to provider — stays enabled", func() {
				Expect(bootstrap.DHCP4).To(BeTrue())
			})
		})

		When("spec=*false, initial=true", func() {
			BeforeEach(func() {
				initial.DHCP4 = true
				interfaceSpec.DHCP4 = ptr.To(false)
			})
			It("user intent overrides provider — disables DHCP4", func() {
				Expect(bootstrap.DHCP4).To(BeFalse())
			})
		})

		When("spec=nil, initial=false", func() {
			It("stays disabled", func() {
				Expect(bootstrap.DHCP4).To(BeFalse())
			})
		})
	})

	Context("DHCP6", func() {
		When("spec=*true, initial=false", func() {
			BeforeEach(func() {
				interfaceSpec.DHCP6 = ptr.To(true)
			})
			It("enables DHCP6", func() {
				Expect(bootstrap.DHCP6).To(BeTrue())
			})
		})

		When("spec=nil, initial=true", func() {
			BeforeEach(func() {
				initial.DHCP6 = true
			})
			It("defers to provider — stays enabled", func() {
				Expect(bootstrap.DHCP6).To(BeTrue())
			})
		})

		When("spec=*false, initial=true", func() {
			BeforeEach(func() {
				initial.DHCP6 = true
				interfaceSpec.DHCP6 = ptr.To(false)
			})
			It("user intent overrides provider — disables DHCP6", func() {
				Expect(bootstrap.DHCP6).To(BeFalse())
			})
		})

		When("spec=nil, initial=false", func() {
			It("stays disabled", func() {
				Expect(bootstrap.DHCP6).To(BeFalse())
			})
		})
	})

	Context("User Addresses", func() {
		When("spec has valid IPv4 and IPv6 CIDRs", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.1/24", IsIPv4: true, Gateway: "10.0.0.254"},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24", "2001:db8::100/64"}
			})
			It("replaces initial IPConfigs completely", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(2))
				Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
				Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
				Expect(bootstrap.IPConfigs[1].IPCIDR).To(Equal("2001:db8::100/64"))
				Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeFalse())
			})
		})

		When("spec has an invalid CIDR", func() {
			BeforeEach(func() {
				interfaceSpec.Addresses = []string{"not-a-cidr", "192.168.1.100/24"}
			})
			It("skips invalid entries silently", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(1))
				Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			})
		})

		When("all spec.Addresses are invalid CIDRs", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.1/24", IsIPv4: true, Gateway: "10.0.0.254"},
				}
				interfaceSpec.Addresses = []string{"not-a-cidr", "also-not-a-cidr"}
			})
			It("replaces initial IPConfigs with an empty slice (entering the addresses block is sufficient)", func() {
				Expect(bootstrap.IPConfigs).To(BeEmpty())
			})
		})

		When("spec has no addresses", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.1/24", IsIPv4: true, Gateway: "10.0.0.254"},
				}
			})
			It("preserves initial IPConfigs", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(1))
				Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("10.0.0.1/24"))
			})
		})

		When("spec has only IPv4 addresses", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.1/24", IsIPv4: true},
					{IPCIDR: "2001:db8::1/64", IsIPv4: false},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24"}
			})
			It("only IPv4 entry in bootstrap, even if initial had IPv6", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(1))
				Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			})
		})
	})

	Context("Gateway Backfill", func() {
		When("initial has IPv4 then IPv6 IPConfigs; spec has addresses, no gateways", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.100/24", IsIPv4: true, Gateway: "10.0.0.1"},
					{IPCIDR: "2001:db8::100/64", IsIPv4: false, Gateway: "fd::1"},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24", "2001:db8::200/64"}
			})
			It("backfills both gateways from initial IPConfigs", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(2))
				Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("10.0.0.1"))
				Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
				Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("fd::1"))
				Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeFalse())
			})
		})

		When("initial has IPv6 then IPv4 IPConfigs; spec has addresses, no gateways", func() {
			BeforeEach(func() {
				// IPv6 entry is first, IPv4 entry is second.
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "2001:db8::100/64", IsIPv4: false, Gateway: "fd::1"},
					{IPCIDR: "10.0.0.100/24", IsIPv4: true, Gateway: "10.0.0.1"},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24", "2001:db8::200/64"}
			})
			It("backfills both gateways from initial IPConfigs", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(2))
				Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("10.0.0.1"))
				Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
				Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("fd::1"))
				Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeFalse())
			})
		})

		When("initial has only IPv4 with gateway; spec has IPv4+IPv6 addresses, no gateways", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.100/24", IsIPv4: true, Gateway: "10.0.0.1"},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24", "2001:db8::200/64"}
			})
			It("backfills Gateway4 but Gateway6 stays empty", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(2))
				Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("10.0.0.1"))
				Expect(bootstrap.IPConfigs[1].Gateway).To(BeEmpty())
			})
		})

		When("initial has IPv4 with empty gateway", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.100/24", IsIPv4: true, Gateway: ""},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24"}
			})
			It("does not use empty gateway for backfill", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(1))
				Expect(bootstrap.IPConfigs[0].Gateway).To(BeEmpty())
			})
		})

		When("spec.Gateway4 is already set; spec.Gateway6 is empty", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "10.0.0.100/24", IsIPv4: true, Gateway: "10.0.0.1"},
					{IPCIDR: "2001:db8::100/64", IsIPv4: false, Gateway: "fd::1"},
				}
				interfaceSpec.Addresses = []string{"192.168.1.100/24", "2001:db8::200/64"}
				interfaceSpec.Gateway4 = "172.16.1.1"
			})
			It("only attempts to backfill Gateway6 from initial", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(2))
				Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("172.16.1.1"))
				Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("fd::1"))
			})
		})
	})

	Context("Gateway Override", func() {
		When("spec has addresses and explicit Gateway4/Gateway6", func() {
			BeforeEach(func() {
				interfaceSpec.Addresses = []string{"192.168.1.100/24", "2001:db8::100/64"}
				interfaceSpec.Gateway4 = "172.16.1.1"
				interfaceSpec.Gateway6 = "2001:db8::2"
			})
			It("applies spec gateways to bootstrap IPConfigs", func() {
				Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("172.16.1.1"))
				Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("2001:db8::2"))
			})
		})

		When("no spec addresses; initial IPConfigs present; spec has Gateway4 and Gateway6", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "192.168.1.100/24", IsIPv4: true, Gateway: "192.168.1.1"},
					{IPCIDR: "2001:db8::100/64", IsIPv4: false, Gateway: "2001:db8::1"},
				}
				interfaceSpec.Gateway4 = "172.16.1.1"
				interfaceSpec.Gateway6 = "2001:db8::2"
			})
			It("updates gateways on existing IPConfig entries by IP family", func() {
				Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("172.16.1.1"))
				Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("2001:db8::2"))
			})
		})

		When("spec.Gateway4 is \"None\"", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "192.168.1.100/24", IsIPv4: true, Gateway: "192.168.1.1"},
				}
				interfaceSpec.Gateway4 = "None"
			})
			It("clears the IPv4 gateway", func() {
				Expect(bootstrap.IPConfigs[0].Gateway).To(BeEmpty())
			})
		})

		When("spec.Gateway6 is \"None\"", func() {
			BeforeEach(func() {
				initial.IPConfigs = []network.NetworkInterfaceIPConfig{
					{IPCIDR: "2001:db8::100/64", IsIPv4: false, Gateway: "2001:db8::1"},
				}
				interfaceSpec.Gateway6 = "None"
			})
			It("clears the IPv6 gateway", func() {
				Expect(bootstrap.IPConfigs[0].Gateway).To(BeEmpty())
			})
		})

		When("spec.Gateway4 is \"None\" alongside spec addresses", func() {
			BeforeEach(func() {
				interfaceSpec.Addresses = []string{"192.168.1.100/24"}
				interfaceSpec.Gateway4 = "None"
			})
			It("resulting IPv4 entries have empty Gateway", func() {
				Expect(bootstrap.IPConfigs).To(HaveLen(1))
				Expect(bootstrap.IPConfigs[0].Gateway).To(BeEmpty())
			})
		})
	})

	Context("Routes", func() {
		When("spec.Routes has multiple entries with mixed Metric", func() {
			BeforeEach(func() {
				interfaceSpec.Routes = []vmopv1.VirtualMachineNetworkRouteSpec{
					{To: "10.10.10.10", Via: "5.5.5.5", Metric: 42},
					{To: "default", Via: "1.2.3.4"},
				}
			})
			It("copies all entries with To/Via/Metric preserved", func() {
				Expect(bootstrap.Routes).To(HaveLen(2))
				Expect(bootstrap.Routes[0]).To(Equal(network.NetworkInterfaceRoute{
					To: "10.10.10.10", Via: "5.5.5.5", Metric: 42,
				}))
				Expect(bootstrap.Routes[1]).To(Equal(network.NetworkInterfaceRoute{
					To: "default", Via: "1.2.3.4", Metric: 0,
				}))
			})
		})

		When("spec.Routes is empty", func() {
			It("bootstrap.Routes is nil/empty", func() {
				Expect(bootstrap.Routes).To(BeEmpty())
			})
		})
	})

	Context("Nameservers", func() {
		When("interfaceSpec.Nameservers is set", func() {
			BeforeEach(func() {
				interfaceSpec.Nameservers = []string{"9.9.9.9"}
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Nameservers: []string{"1.1.1.1"},
				}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
			})
			It("uses spec nameservers; global default is ignored even when CloudInit enables it", func() {
				Expect(bootstrap.Nameservers).To(HaveExactElements("9.9.9.9"))
			})
		})

		When("interfaceSpec.Nameservers is empty, CloudInit enables global nameservers, vm.Spec.Network has nameservers", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Nameservers: []string{"149.112.112.112"},
				}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
			})
			It("falls back to global nameservers", func() {
				Expect(bootstrap.Nameservers).To(HaveExactElements("149.112.112.112"))
			})
		})

		When("interfaceSpec.Nameservers is empty, no CloudInit bootstrap", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Nameservers: []string{"149.112.112.112"},
				}
			})
			It("bootstrap.Nameservers stays empty", func() {
				Expect(bootstrap.Nameservers).To(BeEmpty())
			})
		})

		When("interfaceSpec.Nameservers is empty, CloudInit enables global nameservers, vm.Spec.Network has no nameservers", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
			})
			It("bootstrap.Nameservers is empty", func() {
				Expect(bootstrap.Nameservers).To(BeEmpty())
			})
		})

		When("interfaceSpec.Nameservers is empty, CloudInit present with UseGlobalNameserversAsDefault=false", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					Nameservers: []string{"149.112.112.112"},
				}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						UseGlobalNameserversAsDefault: ptr.To(false),
					},
				}
			})
			It("explicit opt-out suppresses global fallback; bootstrap.Nameservers is empty", func() {
				Expect(bootstrap.Nameservers).To(BeEmpty())
			})
		})
	})

	Context("SearchDomains", func() {
		When("interfaceSpec.SearchDomains is set", func() {
			BeforeEach(func() {
				interfaceSpec.SearchDomains = []string{"vmware.com"}
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					SearchDomains: []string{"broadcom.net"},
				}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
			})
			It("uses spec search domains; global default is ignored even when CloudInit enables it", func() {
				Expect(bootstrap.SearchDomains).To(HaveExactElements("vmware.com"))
			})
		})

		When("interfaceSpec.SearchDomains is empty, CloudInit enables global search domains, vm.Spec.Network has domains", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					SearchDomains: []string{"broadcom.net"},
				}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
			})
			It("falls back to global search domains", func() {
				Expect(bootstrap.SearchDomains).To(HaveExactElements("broadcom.net"))
			})
		})

		When("interfaceSpec.SearchDomains is empty, no CloudInit bootstrap", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					SearchDomains: []string{"broadcom.net"},
				}
				// vm.Spec.Bootstrap is nil → defaultToGlobalSearchDomains=false
			})
			It("bootstrap.SearchDomains stays empty", func() {
				Expect(bootstrap.SearchDomains).To(BeEmpty())
			})
		})

		When("interfaceSpec.SearchDomains is empty, CloudInit enables global search domains, vm.Spec.Network has no domains", func() {
			BeforeEach(func() {
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{},
				}
				// vm.Spec.Network is nil → no global search domains to fall back to
			})
			It("bootstrap.SearchDomains is empty", func() {
				Expect(bootstrap.SearchDomains).To(BeEmpty())
			})
		})

		When("interfaceSpec.SearchDomains is empty, CloudInit present with UseGlobalSearchDomainsAsDefault=false", func() {
			BeforeEach(func() {
				vm.Spec.Network = &vmopv1.VirtualMachineNetworkSpec{
					SearchDomains: []string{"broadcom.net"},
				}
				vm.Spec.Bootstrap = &vmopv1.VirtualMachineBootstrapSpec{
					CloudInit: &vmopv1.VirtualMachineBootstrapCloudInitSpec{
						UseGlobalSearchDomainsAsDefault: ptr.To(false),
					},
				}
			})
			It("explicit opt-out suppresses global fallback; bootstrap.SearchDomains is empty", func() {
				Expect(bootstrap.SearchDomains).To(BeEmpty())
			})
		})
	})

	Context("NoIPAM", func() {
		When("initial.NoIPAM is true", func() {
			BeforeEach(func() {
				initial.NoIPAM = true
			})
			It("bootstrap.NoIPAM is true (the function does not modify it)", func() {
				Expect(bootstrap.NoIPAM).To(BeTrue())
			})
		})

		When("initial.NoIPAM is false", func() {
			It("bootstrap.NoIPAM is false", func() {
				Expect(bootstrap.NoIPAM).To(BeFalse())
			})
		})
	})

	Context("Combined sanity — all overrides applied at once", func() {
		BeforeEach(func() {
			initial.MacAddress = "01:02:03:04:05:06"
			interfaceSpec = vmopv1.VirtualMachineNetworkInterfaceSpec{
				Name:            "my-network-interface",
				GuestDeviceName: "eth42",
				Addresses:       []string{"172.42.1.100/24", "fd1a:6c85:79fe:7c98:0000:0000:0000:000f/56"},
				Gateway4:        "172.42.1.1",
				Gateway6:        "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
				MTU:             ptr.To[int64](9000),
				Nameservers:     []string{"9.9.9.9"},
				Routes: []vmopv1.VirtualMachineNetworkRouteSpec{
					{To: "10.10.10.10", Via: "5.5.5.5", Metric: 42},
					{To: "default", Via: "1.2.3.4"},
				},
				SearchDomains: []string{"vmware.com"},
			}
		})

		It("produces a complete Bootstrap with all spec fields applied", func() {
			Expect(bootstrap.Name).To(Equal("my-network-interface"))
			Expect(bootstrap.GuestDeviceName).To(Equal("eth42"))
			Expect(bootstrap.MacAddress).To(Equal("01:02:03:04:05:06"))
			Expect(bootstrap.MTU).To(BeEquivalentTo(9000))
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(2))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("172.42.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("172.42.1.1"))
			Expect(bootstrap.IPConfigs[1].IPCIDR).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:000f/56"))
			Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))
			Expect(bootstrap.Nameservers).To(HaveExactElements("9.9.9.9"))
			Expect(bootstrap.SearchDomains).To(HaveExactElements("vmware.com"))
			Expect(bootstrap.Routes).To(HaveLen(2))
			Expect(bootstrap.Routes[0]).To(Equal(network.NetworkInterfaceRoute{To: "10.10.10.10", Via: "5.5.5.5", Metric: 42}))
			Expect(bootstrap.Routes[1]).To(Equal(network.NetworkInterfaceRoute{To: "default", Via: "1.2.3.4", Metric: 0}))
		})
	})
})

var _ = Describe("NetOPInterfaceBootstrap", func() {
	var (
		ctx           context.Context
		vm            *vmopv1.VirtualMachine
		interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec
		netIf         *netopv1alpha1.NetworkInterface
		macAddress    string
		bootstrap     network.Bootstrap
	)

	BeforeEach(func() {
		ctx = context.Background()
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
			},
		}
		interfaceSpec = vmopv1.VirtualMachineNetworkInterfaceSpec{
			Name: "eth0",
		}
		netIf = &netopv1alpha1.NetworkInterface{}
		macAddress = ""
	})

	JustBeforeEach(func() {
		vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, interfaceSpec)
		bootstrap = network.NetOPInterfaceBootstrap(ctx, vm, netIf, interfaceSpec, macAddress)
	})

	When("IPAssignmentMode=StaticPool, IPv6AssignmentMode=StaticPool, IPConfigs populated", func() {
		BeforeEach(func() {
			macAddress = "AA:BB:CC:DD:EE:FF"

			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:       "192.168.1.100",
					IPFamily: corev1.IPv4Protocol,
					Gateway:  "192.168.1.1",
					// SubnetMask (legacy) path
					SubnetMask: "255.255.255.0",
				},
				{
					IP:       "2001:db8::100",
					IPFamily: corev1.IPv6Protocol,
					Gateway:  "2001:db8::1",
					// Prefix path takes precedence over SubnetMask
					Prefix: ptr.To[int32](64),
				},
			}
		})

		It("produces non-DHCP Bootstrap with IPConfigs in CIDR notation, MAC lowercased", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.IPConfigs).To(HaveLen(2))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
			Expect(bootstrap.IPConfigs[1].IPCIDR).To(Equal("2001:db8::100/64"))
			Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("2001:db8::1"))
		})
	})

	When("assignment modes are unset and there are no IPConfigs", func() {
		It("effective IPv4 mode is DHCP → bootstrap.DHCP4=true", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("IPAssignmentMode=None, IPv6AssignmentMode=None", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeNone
			netIf.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeNone
		})
		It("bootstrap.NoIPAM=true, no IPConfigs", func() {
			Expect(bootstrap.NoIPAM).To(BeTrue())
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("combined with spec overrides (DHCP4 enable)", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:         "192.168.1.100",
					IPFamily:   corev1.IPv4Protocol,
					Gateway:    "192.168.1.1",
					SubnetMask: "255.255.255.0",
				},
			}
			interfaceSpec.DHCP4 = ptr.To(true)
		})
		It("spec DHCP4=true overrides StaticPool initial", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
		})
	})

	When("IPAssignmentMode is unset but status has IPv4 IPConfigs (legacy mode detection)", func() {
		BeforeEach(func() {
			// IPAssignmentMode is intentionally left "" to exercise the legacy
			// inference path in EffectiveNetOPIPv4AssignmentMode: when no explicit
			// mode is set but IPv4 IPConfigs with non-empty IPs are present, the
			// function infers StaticPool rather than defaulting to DHCP.
			macAddress = "aa:bb:cc:dd:ee:ff"
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:         "192.168.1.100",
					IPFamily:   corev1.IPv4Protocol,
					Gateway:    "192.168.1.1",
					SubnetMask: "255.255.255.0",
				},
			}
		})
		It("infers StaticPool: DHCP4=false, IPConfigs populated from status", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})

	When("IPAssignmentMode=DHCP (explicit, not legacy inference)", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
		})
		It("DHCP4=true, DHCP6=false, no IPConfigs", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("IPAssignmentMode=DHCP, IPv6AssignmentMode=DHCP (both explicit)", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
			netIf.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
		})
		It("DHCP4=true, DHCP6=true, no IPConfigs", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.DHCP6).To(BeTrue())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("IPv4=DHCP, IPv6=StaticPool, IPv6 IPConfig present in status", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
			netIf.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:       "2001:db8::100",
					IPFamily: corev1.IPv6Protocol,
					Gateway:  "2001:db8::1",
					Prefix:   ptr.To[int32](64),
				},
			}
		})
		It("DHCP4=true, DHCP6=false; only IPv6 IPConfig in bootstrap (IPv4 filtered because mode=DHCP)", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("2001:db8::100/64"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("2001:db8::1"))
		})
	})

	When("IPv4=StaticPool with Prefix field (not SubnetMask)", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:       "192.168.1.100",
					IPFamily: corev1.IPv4Protocol,
					Gateway:  "192.168.1.1",
					Prefix:   ptr.To[int32](24),
				},
				{
					IP:       "192.168.1.101",
					IPFamily: corev1.IPv4Protocol,
					Gateway:  "192.168.1.2",
					Prefix:   ptr.To[int32](24),
				},
			}
		})
		It("produces IPv4 IPConfigs via the Prefix→CIDR path", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(2))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
			Expect(bootstrap.IPConfigs[1].IPCIDR).To(Equal("192.168.1.101/24"))
			Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("192.168.1.2"))
		})
	})

	When("IPv6=StaticPool with multiple IPv6 IPConfigs using Prefix field", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:       "2001:db8::100",
					IPFamily: corev1.IPv6Protocol,
					Gateway:  "2001:db8::1",
					Prefix:   ptr.To[int32](64),
				},
				{
					IP:       "2001:db8::101",
					IPFamily: corev1.IPv6Protocol,
					Gateway:  "2001:db8::1",
					Prefix:   ptr.To[int32](64),
				},
			}
		})
		It("produces multiple IPv6 IPConfigs via the Prefix→CIDR path", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(2))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("2001:db8::100/64"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("2001:db8::1"))
			Expect(bootstrap.IPConfigs[1].IPCIDR).To(Equal("2001:db8::101/64"))
			Expect(bootstrap.IPConfigs[1].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[1].Gateway).To(Equal("2001:db8::1"))
		})
	})

	When("IPv4=StaticPool, IPv6=DHCP, both IPConfig types present in status", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPv6AssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:         "192.168.1.100",
					IPFamily:   corev1.IPv4Protocol,
					Gateway:    "192.168.1.1",
					SubnetMask: "255.255.255.0",
				},
				{
					IP:       "2001:db8::100",
					IPFamily: corev1.IPv6Protocol,
					Gateway:  "2001:db8::1",
					Prefix:   ptr.To[int32](64),
				},
			}
		})
		It("DHCP4=false, DHCP6=true; only IPv4 IPConfig in bootstrap (IPv6 filtered because mode=DHCP)", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.DHCP6).To(BeTrue())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})

	When("IPv4=DHCP with an IPv4 IPConfig present in status", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeDHCP
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:         "192.168.1.100",
					IPFamily:   corev1.IPv4Protocol,
					SubnetMask: "255.255.255.0",
					Gateway:    "192.168.1.1",
				},
			}
		})
		It("IPv4 IPConfig is skipped because mode is DHCP, not StaticPool; DHCP4=true, IPConfigs empty", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("IPv4=StaticPool, status has an IPConfig with an unrecognized IPFamily", func() {
		BeforeEach(func() {
			netIf.Status.IPAssignmentMode = netopv1alpha1.NetworkInterfaceIPAssignmentModeStaticPool
			netIf.Status.IPConfigs = []netopv1alpha1.IPConfig{
				{
					IP:         "192.168.1.100",
					IPFamily:   corev1.IPv4Protocol,
					SubnetMask: "255.255.255.0",
					Gateway:    "192.168.1.1",
				},
				{
					IP:       "10.0.0.1",
					IPFamily: "Unknown",
					Gateway:  "10.0.0.254",
				},
			}
		})
		It("IPConfig with unrecognized IPFamily is skipped; only the recognized IPv4 entry is included", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})
})

var _ = Describe("NCPInterfaceBootstrap", func() {
	var (
		ctx           context.Context
		vm            *vmopv1.VirtualMachine
		interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec
		vnetIf        *ncpv1alpha1.VirtualNetworkInterface
		macAddress    string
		bootstrap     network.Bootstrap
	)

	BeforeEach(func() {
		ctx = context.Background()
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
			},
		}
		interfaceSpec = vmopv1.VirtualMachineNetworkInterfaceSpec{
			Name: "eth0",
		}
		vnetIf = &ncpv1alpha1.VirtualNetworkInterface{
			Status: ncpv1alpha1.VirtualNetworkInterfaceStatus{
				ProviderStatus: &ncpv1alpha1.VirtualNetworkInterfaceProviderStatus{},
			},
		}
		macAddress = ""
	})

	JustBeforeEach(func() {
		vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, interfaceSpec)
		bootstrap = network.NCPInterfaceBootstrap(ctx, vm, vnetIf, interfaceSpec, macAddress)
	})

	When("status has IPAddresses set", func() {
		BeforeEach(func() {
			macAddress = "aa:bb:cc:dd:ee:ff"
			vnetIf.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
				{IP: "192.168.1.100", SubnetMask: "255.255.255.0", Gateway: "192.168.1.1"},
			}
		})
		It("produces IPConfigs in CIDR notation and MAC from status", func() {
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})

	When("status has no IPAddresses", func() {
		It("bootstrap.DHCP4=true", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("status has a single entry with empty IP (DHCP stub)", func() {
		BeforeEach(func() {
			vnetIf.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
				{IP: ""},
			}
		})
		It("bootstrap.DHCP4=true (matches len==1 && IP==\"\" guard)", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("status has multiple IPAddresses and one has an empty IP", func() {
		BeforeEach(func() {
			vnetIf.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
				{IP: ""},
				{IP: "192.168.1.100", SubnetMask: "255.255.255.0", Gateway: "192.168.1.1"},
			}
		})
		It("entry with empty IP is skipped; the valid entry is included", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})

	When("combined with spec overrides (user addresses)", func() {
		BeforeEach(func() {
			vnetIf.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
				{IP: "10.0.0.100", SubnetMask: "255.255.255.0", Gateway: "10.0.0.1"},
			}
			interfaceSpec.Addresses = []string{"192.168.1.100/24"}
			interfaceSpec.Gateway4 = "192.168.1.1"
		})
		It("spec addresses replace NCP IPConfigs, gateway from spec", func() {
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})

	When("status has an IPv6 address with IPv6 SubnetMask", func() {
		BeforeEach(func() {
			macAddress = "aa:bb:cc:dd:ee:ff"
			vnetIf.Status.IPAddresses = []ncpv1alpha1.VirtualNetworkInterfaceIP{
				{
					IP:         "fd1a:6c85:79fe:7c98:0000:0000:0000:000f",
					SubnetMask: "ffff:ffff:ffff:ff00:0000:0000:0000:0000",
					Gateway:    "fd1a:6c85:79fe:7c98:0000:0000:0000:0001",
				},
			}
		})
		It("produces IPv6 IPConfig with CIDR notation via ipCIDRNotation IPv6 path", func() {
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("fd1a:6c85:79fe:7c98:0000:0000:0000:0001"))
		})
	})
})

var _ = Describe("VPCInterfaceBootstrap", func() {
	var (
		ctx           context.Context
		vm            *vmopv1.VirtualMachine
		interfaceSpec vmopv1.VirtualMachineNetworkInterfaceSpec
		subnetPort    *vpcv1alpha1.SubnetPort
		macAddress    string
		bootstrap     network.Bootstrap
	)

	BeforeEach(func() {
		ctx = context.Background()
		vm = &vmopv1.VirtualMachine{
			Spec: vmopv1.VirtualMachineSpec{
				Network: &vmopv1.VirtualMachineNetworkSpec{},
			},
		}
		interfaceSpec = vmopv1.VirtualMachineNetworkInterfaceSpec{
			Name: "eth0",
		}
		subnetPort = &vpcv1alpha1.SubnetPort{}
		macAddress = ""
	})

	JustBeforeEach(func() {
		vm.Spec.Network.Interfaces = append(vm.Spec.Network.Interfaces, interfaceSpec)
		bootstrap = network.VPCInterfaceBootstrap(ctx, vm, subnetPort, interfaceSpec, macAddress)
	})

	When("status has valid IPAddresses and a MAC address", func() {
		BeforeEach(func() {
			macAddress = "aa:bb:cc:dd:ee:ff"
			subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
				{IPAddress: "192.168.1.100/24", Gateway: "192.168.1.1"},
			}
		})
		It("produces IPConfigs and MAC from status", func() {
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeTrue())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("192.168.1.1"))
		})
	})

	When("no IPAddresses and DHCPDeactivatedOnSubnet=false", func() {
		It("bootstrap.DHCP4=true", func() {
			Expect(bootstrap.DHCP4).To(BeTrue())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("no IPAddresses and DHCPDeactivatedOnSubnet=true", func() {
		BeforeEach(func() {
			subnetPort.Status.NetworkInterfaceConfig.DHCPDeactivatedOnSubnet = true
		})
		It("bootstrap.NoIPAM=true", func() {
			Expect(bootstrap.NoIPAM).To(BeTrue())
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("IPAddresses contains just Gateway and DHCPDeactivatedOnSubnet=true", func() {
		BeforeEach(func() {
			subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
				{IPAddress: "", Gateway: "192.168.1.1"},
			}
			subnetPort.Status.NetworkInterfaceConfig.DHCPDeactivatedOnSubnet = true
		})
		It("bootstrap.NoIPAM=true", func() {
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeTrue())
			Expect(bootstrap.IPConfigs).To(BeEmpty())
		})
	})

	When("IPAddresses contains entries with empty IPAddress (DHCP/NoIPAM stub entries)", func() {
		BeforeEach(func() {
			macAddress = "aa:bb:cc:dd:ee:ff"
			subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
				{IPAddress: "", Gateway: "192.168.1.1"},
				{IPAddress: "192.168.1.100/24", Gateway: "192.168.1.1"},
			}
		})
		It("skips entries with empty IPAddress", func() {
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("192.168.1.100/24"))
		})
	})

	When("status has an IPv6 IPAddress in CIDR notation", func() {
		BeforeEach(func() {
			macAddress = "aa:bb:cc:dd:ee:ff"
			subnetPort.Status.NetworkInterfaceConfig.IPAddresses = []vpcv1alpha1.NetworkInterfaceIPAddress{
				{
					IPAddress: "fd1a:6c85:79fe:7c98::f/56",
					Gateway:   "fd1a:6c85:79fe:7c98::1",
				},
			}
		})
		It("produces an IPv6 IPConfig; IsIPv4=false, IPCIDR preserved as-is", func() {
			Expect(bootstrap.MacAddress).To(Equal("aa:bb:cc:dd:ee:ff"))
			Expect(bootstrap.DHCP4).To(BeFalse())
			Expect(bootstrap.NoIPAM).To(BeFalse())
			Expect(bootstrap.IPConfigs).To(HaveLen(1))
			Expect(bootstrap.IPConfigs[0].IPCIDR).To(Equal("fd1a:6c85:79fe:7c98::f/56"))
			Expect(bootstrap.IPConfigs[0].IsIPv4).To(BeFalse())
			Expect(bootstrap.IPConfigs[0].Gateway).To(Equal("fd1a:6c85:79fe:7c98::1"))
		})
	})
})
