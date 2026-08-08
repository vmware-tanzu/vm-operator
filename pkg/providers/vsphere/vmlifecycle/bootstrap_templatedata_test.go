// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/network"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/vmlifecycle"
)

var _ = Describe("TemplateVMMetadata", func() {

	const (
		ip1         = "192.168.1.37"
		ip1Cidr     = ip1 + "/24"
		ip2         = "192.168.10.48"
		ip2Cidr     = ip2 + "/24"
		gateway1    = "192.168.1.1"
		gateway2    = "192.168.10.1"
		nameserver1 = "8.8.8.8"
		nameserver2 = "1.1.1.1"
		macAddr1    = "8a-cb-a0-1d-8d-c4"
		macAddr2    = "00-cb-30-42-05-89"
	)

	var (
		vmCtx  pkgctx.VirtualMachineContext
		vm     *vmopv1.VirtualMachine
		bsArgs *vmlifecycle.BootstrapArgs
	)

	BeforeEach(func() {
		vm = &vmopv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dummy-vm",
				Namespace: "dummy-ns",
			},
		}

		vmCtx = pkgctx.VirtualMachineContext{
			Context: context.Background(),
			Logger:  suite.GetLogger().WithName("bootstrap-template-tests"),
			VM:      vm,
		}

		bsArgs = &vmlifecycle.BootstrapArgs{}
		bsArgs.Data = make(map[string]string)
		bsArgs.DNSServers = []string{nameserver1, nameserver2}
		bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
			{
				MacAddress: macAddr1,
				IPConfigs: []network.NetworkInterfaceIPConfig{
					{
						Gateway: gateway1,
						IPCIDR:  ip1Cidr,
						IsIPv4:  true,
					},
				},
			},
			{
				MacAddress: macAddr2,
				IPConfigs: []network.NetworkInterfaceIPConfig{
					{
						Gateway: gateway2,
						IPCIDR:  ip2Cidr,
						IsIPv4:  true,
					},
				},
			},
		}
	})

	Context("Template Functions", func() {
		DescribeTable("v1alpha1 template functions",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("first_cidrIp", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}", ip1Cidr),
			Entry("second_cidrIp", "{{ (index (index .V1alpha1.Net.Devices 1).IPAddresses 0) }}", ip2Cidr),
			Entry("first_gateway", "{{ (index .V1alpha1.Net.Devices 0).Gateway4 }}", gateway1),
			Entry("second_gateway", "{{ (index .V1alpha1.Net.Devices 1).Gateway4 }}", gateway2),
			Entry("nameserver", "{{ (index .V1alpha1.Net.Nameservers 0) }}", nameserver1),
			Entry("first_macAddr", "{{ (index .V1alpha1.Net.Devices 0).MacAddress }}", macAddr1),
			Entry("second_macAddr", "{{ (index .V1alpha1.Net.Devices 1).MacAddress }}", macAddr2),
			Entry("name", "{{ .V1alpha1.VM.Name }}", "dummy-vm"),
		)

		DescribeTable("v1alpha2 template functions",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("first_cidrIp", "{{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) }}", ip1Cidr),
			Entry("second_cidrIp", "{{ (index (index .V1alpha2.Net.Devices 1).IPAddresses 0) }}", ip2Cidr),
			Entry("first_gateway", "{{ (index .V1alpha2.Net.Devices 0).Gateway4 }}", gateway1),
			Entry("second_gateway", "{{ (index .V1alpha2.Net.Devices 1).Gateway4 }}", gateway2),
			Entry("nameserver", "{{ (index .V1alpha2.Net.Nameservers 0) }}", nameserver1),
			Entry("first_macAddr", "{{ (index .V1alpha2.Net.Devices 0).MacAddress }}", macAddr1),
			Entry("second_macAddr", "{{ (index .V1alpha2.Net.Devices 1).MacAddress }}", macAddr2),
			Entry("name", "{{ .V1alpha2.VM.Name }}", "dummy-vm"),
		)

		DescribeTable("v1alpha3 template functions",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("first_cidrIp", "{{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}", ip1Cidr),
			Entry("second_cidrIp", "{{ (index (index .V1alpha3.Net.Devices 1).IPAddresses 0) }}", ip2Cidr),
			Entry("first_gateway", "{{ (index .V1alpha3.Net.Devices 0).Gateway4 }}", gateway1),
			Entry("second_gateway", "{{ (index .V1alpha3.Net.Devices 1).Gateway4 }}", gateway2),
			Entry("nameserver", "{{ (index .V1alpha3.Net.Nameservers 0) }}", nameserver1),
			Entry("first_macAddr", "{{ (index .V1alpha3.Net.Devices 0).MacAddress }}", macAddr1),
			Entry("second_macAddr", "{{ (index .V1alpha3.Net.Devices 1).MacAddress }}", macAddr2),
			Entry("name", "{{ .V1alpha3.VM.Name }}", "dummy-vm"),
		)

		DescribeTable("v1alpha4 template functions",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("first_cidrIp", "{{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) }}", ip1Cidr),
			Entry("second_cidrIp", "{{ (index (index .V1alpha4.Net.Devices 1).IPAddresses 0) }}", ip2Cidr),
			Entry("first_gateway", "{{ (index .V1alpha4.Net.Devices 0).Gateway4 }}", gateway1),
			Entry("second_gateway", "{{ (index .V1alpha4.Net.Devices 1).Gateway4 }}", gateway2),
			Entry("nameserver", "{{ (index .V1alpha4.Net.Nameservers 0) }}", nameserver1),
			Entry("first_macAddr", "{{ (index .V1alpha4.Net.Devices 0).MacAddress }}", macAddr1),
			Entry("second_macAddr", "{{ (index .V1alpha4.Net.Devices 1).MacAddress }}", macAddr2),
			Entry("name", "{{ .V1alpha4.VM.Name }}", "dummy-vm"),
		)

		DescribeTable("v1alpha5 template functions",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("first_cidrIp", "{{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}", ip1Cidr),
			Entry("second_cidrIp", "{{ (index (index .V1alpha5.Net.Devices 1).IPAddresses 0) }}", ip2Cidr),
			Entry("first_gateway", "{{ (index .V1alpha5.Net.Devices 0).Gateway4 }}", gateway1),
			Entry("second_gateway", "{{ (index .V1alpha5.Net.Devices 1).Gateway4 }}", gateway2),
			Entry("nameserver", "{{ (index .V1alpha5.Net.Nameservers 0) }}", nameserver1),
			Entry("first_macAddr", "{{ (index .V1alpha5.Net.Devices 0).MacAddress }}", macAddr1),
			Entry("second_macAddr", "{{ (index .V1alpha5.Net.Devices 1).MacAddress }}", macAddr2),
			Entry("name", "{{ .V1alpha5.VM.Name }}", "dummy-vm"),
		)

		DescribeTable("v1alpha6 template functions",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("first_cidrIp", "{{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) }}", ip1Cidr),
			Entry("second_cidrIp", "{{ (index (index .V1alpha6.Net.Devices 1).IPAddresses 0) }}", ip2Cidr),
			Entry("first_gateway", "{{ (index .V1alpha6.Net.Devices 0).Gateway4 }}", gateway1),
			Entry("second_gateway", "{{ (index .V1alpha6.Net.Devices 1).Gateway4 }}", gateway2),
			Entry("nameserver", "{{ (index .V1alpha6.Net.Nameservers 0) }}", nameserver1),
			Entry("first_macAddr", "{{ (index .V1alpha6.Net.Devices 0).MacAddress }}", macAddr1),
			Entry("second_macAddr", "{{ (index .V1alpha6.Net.Devices 1).MacAddress }}", macAddr2),
			Entry("name", "{{ .V1alpha6.VM.Name }}", "dummy-vm"),
		)
	})

	Context("Function names", func() {
		DescribeTable("v1alpha1 constant names",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("cidr_ip1", "{{ "+constants.V1alpha1FirstIP+" }}", ip1Cidr),
			Entry("cidr_ip2", "{{ "+constants.V1alpha1FirstIPFromNIC+" 1 }}", ip2Cidr),
			Entry("cidr_ip3", "{{ ("+constants.V1alpha1IP+" \"192.168.1.37\") }}", ip1Cidr),
			Entry("cidr_ip4", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1.37\" \"/24\") }}", ip1Cidr),
			Entry("cidr_ip5", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1.37\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip6", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1.37/28\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip7", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1.37/28\" \"/24\") }}", ip1Cidr),
			Entry("ip1", "{{ "+constants.V1alpha1FormatIP+" "+constants.V1alpha1FirstIP+" \"\" }}", ip1),
			Entry("ip2", "{{ "+constants.V1alpha1FormatIP+" \"192.168.1.37/28\" \"\" }}", ip1),
			Entry("ips_1", "{{ "+constants.V1alpha1IPsFromNIC+" 0 }}", fmt.Sprint([]string{ip1Cidr})),
			Entry("subnetmask", "{{ "+constants.V1alpha1SubnetMask+" \"192.168.1.37/26\" }}", "255.255.255.192"),
			Entry("firstNicMacAddr", "{{ "+constants.V1alpha1FirstNicMacAddr+" }}", macAddr1),
			Entry("formatted_nameserver1", "{{ "+constants.V1alpha1FormatNameservers+" 1 \"-\"}}", nameserver1),
			Entry("formatted_nameserver2", "{{ "+constants.V1alpha1FormatNameservers+" -1 \"-\"}}", nameserver1+"-"+nameserver2),
		)

		DescribeTable("v1alpha2 constant names",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("cidr_ip1", "{{ "+constants.V1alpha2FirstIP+" }}", ip1Cidr),
			Entry("cidr_ip2", "{{ "+constants.V1alpha2FirstIPFromNIC+" 1 }}", ip2Cidr),
			Entry("cidr_ip3", "{{ ("+constants.V1alpha2IP+" \"192.168.1.37\") }}", ip1Cidr),
			Entry("cidr_ip4", "{{ ("+constants.V1alpha2FormatIP+" \"192.168.1.37\" \"/24\") }}", ip1Cidr),
			Entry("cidr_ip5", "{{ ("+constants.V1alpha2FormatIP+" \"192.168.1.37\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip6", "{{ ("+constants.V1alpha2FormatIP+" \"192.168.1.37/28\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip7", "{{ ("+constants.V1alpha2FormatIP+" \"192.168.1.37/28\" \"/24\") }}", ip1Cidr),
			Entry("ip1", "{{ "+constants.V1alpha2FormatIP+" "+constants.V1alpha1FirstIP+" \"\" }}", ip1),
			Entry("ip2", "{{ "+constants.V1alpha2FormatIP+" \"192.168.1.37/28\" \"\" }}", ip1),
			Entry("ips_1", "{{ "+constants.V1alpha2IPsFromNIC+" 0 }}", fmt.Sprint([]string{ip1Cidr})),
			Entry("subnetmask", "{{ "+constants.V1alpha2SubnetMask+" \"192.168.1.37/26\" }}", "255.255.255.192"),
			Entry("firstNicMacAddr", "{{ "+constants.V1alpha2FirstNicMacAddr+" }}", macAddr1),
			Entry("formatted_nameserver1", "{{ "+constants.V1alpha2FormatNameservers+" 1 \"-\"}}", nameserver1),
			Entry("formatted_nameserver2", "{{ "+constants.V1alpha2FormatNameservers+" -1 \"-\"}}", nameserver1+"-"+nameserver2),
		)

		DescribeTable("v1alpha3 constant names",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("cidr_ip1", "{{ "+constants.V1alpha3FirstIP+" }}", ip1Cidr),
			Entry("cidr_ip2", "{{ "+constants.V1alpha3FirstIPFromNIC+" 1 }}", ip2Cidr),
			Entry("cidr_ip3", "{{ ("+constants.V1alpha3IP+" \"192.168.1.37\") }}", ip1Cidr),
			Entry("cidr_ip4", "{{ ("+constants.V1alpha3FormatIP+" \"192.168.1.37\" \"/24\") }}", ip1Cidr),
			Entry("cidr_ip5", "{{ ("+constants.V1alpha3FormatIP+" \"192.168.1.37\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip6", "{{ ("+constants.V1alpha3FormatIP+" \"192.168.1.37/28\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip7", "{{ ("+constants.V1alpha3FormatIP+" \"192.168.1.37/28\" \"/24\") }}", ip1Cidr),
			Entry("ip1", "{{ "+constants.V1alpha3FormatIP+" "+constants.V1alpha1FirstIP+" \"\" }}", ip1),
			Entry("ip2", "{{ "+constants.V1alpha3FormatIP+" \"192.168.1.37/28\" \"\" }}", ip1),
			Entry("ips_1", "{{ "+constants.V1alpha3IPsFromNIC+" 0 }}", fmt.Sprint([]string{ip1Cidr})),
			Entry("subnetmask", "{{ "+constants.V1alpha3SubnetMask+" \"192.168.1.37/26\" }}", "255.255.255.192"),
			Entry("firstNicMacAddr", "{{ "+constants.V1alpha3FirstNicMacAddr+" }}", macAddr1),
			Entry("formatted_nameserver1", "{{ "+constants.V1alpha3FormatNameservers+" 1 \"-\"}}", nameserver1),
			Entry("formatted_nameserver2", "{{ "+constants.V1alpha3FormatNameservers+" -1 \"-\"}}", nameserver1+"-"+nameserver2),
		)

		DescribeTable("v1alpha4 constant names",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("cidr_ip1", "{{ "+constants.V1alpha4FirstIP+" }}", ip1Cidr),
			Entry("cidr_ip2", "{{ "+constants.V1alpha4FirstIPFromNIC+" 1 }}", ip2Cidr),
			Entry("cidr_ip3", "{{ ("+constants.V1alpha4IP+" \"192.168.1.37\") }}", ip1Cidr),
			Entry("cidr_ip4", "{{ ("+constants.V1alpha4FormatIP+" \"192.168.1.37\" \"/24\") }}", ip1Cidr),
			Entry("cidr_ip5", "{{ ("+constants.V1alpha4FormatIP+" \"192.168.1.37\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip6", "{{ ("+constants.V1alpha4FormatIP+" \"192.168.1.37/28\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip7", "{{ ("+constants.V1alpha4FormatIP+" \"192.168.1.37/28\" \"/24\") }}", ip1Cidr),
			Entry("ip1", "{{ "+constants.V1alpha4FormatIP+" "+constants.V1alpha1FirstIP+" \"\" }}", ip1),
			Entry("ip2", "{{ "+constants.V1alpha4FormatIP+" \"192.168.1.37/28\" \"\" }}", ip1),
			Entry("ips_1", "{{ "+constants.V1alpha4IPsFromNIC+" 0 }}", fmt.Sprint([]string{ip1Cidr})),
			Entry("subnetmask", "{{ "+constants.V1alpha4SubnetMask+" \"192.168.1.37/26\" }}", "255.255.255.192"),
			Entry("firstNicMacAddr", "{{ "+constants.V1alpha4FirstNicMacAddr+" }}", macAddr1),
			Entry("formatted_nameserver1", "{{ "+constants.V1alpha4FormatNameservers+" 1 \"-\"}}", nameserver1),
			Entry("formatted_nameserver2", "{{ "+constants.V1alpha4FormatNameservers+" -1 \"-\"}}", nameserver1+"-"+nameserver2),
		)

		DescribeTable("v1alpha5 constant names",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("cidr_ip1", "{{ "+constants.V1alpha5FirstIP+" }}", ip1Cidr),
			Entry("cidr_ip2", "{{ "+constants.V1alpha5FirstIPFromNIC+" 1 }}", ip2Cidr),
			Entry("cidr_ip3", "{{ ("+constants.V1alpha5IP+" \"192.168.1.37\") }}", ip1Cidr),
			Entry("cidr_ip4", "{{ ("+constants.V1alpha5FormatIP+" \"192.168.1.37\" \"/24\") }}", ip1Cidr),
			Entry("cidr_ip5", "{{ ("+constants.V1alpha5FormatIP+" \"192.168.1.37\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip6", "{{ ("+constants.V1alpha5FormatIP+" \"192.168.1.37/28\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip7", "{{ ("+constants.V1alpha5FormatIP+" \"192.168.1.37/28\" \"/24\") }}", ip1Cidr),
			Entry("ip1", "{{ "+constants.V1alpha5FormatIP+" "+constants.V1alpha1FirstIP+" \"\" }}", ip1),
			Entry("ip2", "{{ "+constants.V1alpha5FormatIP+" \"192.168.1.37/28\" \"\" }}", ip1),
			Entry("ips_1", "{{ "+constants.V1alpha5IPsFromNIC+" 0 }}", fmt.Sprint([]string{ip1Cidr})),
			Entry("subnetmask", "{{ "+constants.V1alpha5SubnetMask+" \"192.168.1.37/26\" }}", "255.255.255.192"),
			Entry("firstNicMacAddr", "{{ "+constants.V1alpha5FirstNicMacAddr+" }}", macAddr1),
			Entry("formatted_nameserver1", "{{ "+constants.V1alpha5FormatNameservers+" 1 \"-\"}}", nameserver1),
			Entry("formatted_nameserver2", "{{ "+constants.V1alpha5FormatNameservers+" -1 \"-\"}}", nameserver1+"-"+nameserver2),
		)

		DescribeTable("v1alpha6 constant names",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("cidr_ip1", "{{ "+constants.V1alpha6FirstIP+" }}", ip1Cidr),
			Entry("cidr_ip2", "{{ "+constants.V1alpha6FirstIPFromNIC+" 1 }}", ip2Cidr),
			Entry("cidr_ip3", "{{ ("+constants.V1alpha6IP+" \"192.168.1.37\") }}", ip1Cidr),
			Entry("cidr_ip4", "{{ ("+constants.V1alpha6FormatIP+" \"192.168.1.37\" \"/24\") }}", ip1Cidr),
			Entry("cidr_ip5", "{{ ("+constants.V1alpha6FormatIP+" \"192.168.1.37\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip6", "{{ ("+constants.V1alpha6FormatIP+" \"192.168.1.37/28\" \"255.255.255.0\") }}", ip1Cidr),
			Entry("cidr_ip7", "{{ ("+constants.V1alpha6FormatIP+" \"192.168.1.37/28\" \"/24\") }}", ip1Cidr),
			Entry("ip1", "{{ "+constants.V1alpha6FormatIP+" "+constants.V1alpha6FirstIP+" \"\" }}", ip1),
			Entry("ip2", "{{ "+constants.V1alpha6FormatIP+" \"192.168.1.37/28\" \"\" }}", ip1),
			Entry("ips_1", "{{ "+constants.V1alpha6IPsFromNIC+" 0 }}", fmt.Sprint([]string{ip1Cidr})),
			Entry("subnetmask", "{{ "+constants.V1alpha6SubnetMask+" \"192.168.1.37/26\" }}", "255.255.255.192"),
			Entry("firstNicMacAddr", "{{ "+constants.V1alpha6FirstNicMacAddr+" }}", macAddr1),
			Entry("formatted_nameserver1", "{{ "+constants.V1alpha6FormatNameservers+" 1 \"-\"}}", nameserver1),
			Entry("formatted_nameserver2", "{{ "+constants.V1alpha6FormatNameservers+" -1 \"-\"}}", nameserver1+"-"+nameserver2),
			Entry("first_ipv4_explicit", "{{ "+constants.V1alpha6FirstIPv4+" }}", ip1Cidr),
			Entry("first_ipv4_from_nic_explicit", "{{ "+constants.V1alpha6FirstIPv4FromNIC+" 1 }}", ip2Cidr),
			Entry("prefix_length_v4", "{{ "+constants.V1alpha6PrefixLength+" \"192.168.1.37/26\" }}", "26"),
			Entry("prefix_length_v6", "{{ "+constants.V1alpha6PrefixLength+" \"2001:db8::/64\" }}", "64"),
			Entry("prefix_length_compose", "{{ "+constants.V1alpha6FormatIP+" \"192.168.1.37\" (printf \"/%d\" ("+constants.V1alpha6PrefixLength+" \"10.0.0.0/26\")) }}", "192.168.1.37/26"),
			Entry("is_usable_ip_true", "{{ "+constants.V1alpha6IsUsableIP+" \"192.168.1.37\" }}", "true"),
			Entry("is_usable_ip_loopback", "{{ "+constants.V1alpha6IsUsableIP+" \"127.0.0.1\" }}", "false"),
			Entry("is_usable_ip_linklocal_v4", "{{ "+constants.V1alpha6IsUsableIP+" \"169.254.1.1\" }}", "false"),
			Entry("is_usable_ip_linklocal_v6", "{{ "+constants.V1alpha6IsUsableIP+" \"fe80::1\" }}", "false"),
			Entry("is_usable_ip_global_v6", "{{ "+constants.V1alpha6IsUsableIP+" \"2001:db8::1\" }}", "true"),
			Entry("is_usable_ip_invalid", "{{ "+constants.V1alpha6IsUsableIP+" \"not-an-ip\" }}", "false"),
		)
	})

	Context("V1alpha6 IPv4/IPv6 fallback semantics", func() {
		const (
			ip6Global        = "2001:db8::10"
			ip6GlobalCidr    = ip6Global + "/64"
			ip6LinkLocal     = "fe80::1"
			ip6LinkLocalCidr = ip6LinkLocal + "/64"
			gateway6         = "2001:db8::1"
		)

		render := func(str string) string {
			fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
			return fn("", str)
		}

		When("a device has both an IPv4 and a global IPv6 address", func() {
			BeforeEach(func() {
				bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
					{
						MacAddress: macAddr1,
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{Gateway: gateway1, IPCIDR: ip1Cidr, IsIPv4: true},
							{Gateway: gateway6, IPCIDR: ip6GlobalCidr, IsIPv4: false},
						},
					},
				}
			})

			It("FirstIP/FirstIPFromNIC/IPsFromNIC still return IPv4, unchanged", func() {
				Expect(render("{{ " + constants.V1alpha6FirstIP + " }}")).To(Equal(ip1Cidr))
				Expect(render("{{ " + constants.V1alpha6FirstIPFromNIC + " 0 }}")).To(Equal(ip1Cidr))
				Expect(render("{{ " + constants.V1alpha6IPsFromNIC + " 0 }}")).To(Equal(fmt.Sprint([]string{ip1Cidr})))
			})

			It("FirstIPv6/FirstIPv6FromNIC return the IPv6 address", func() {
				Expect(render("{{ " + constants.V1alpha6FirstIPv6 + " }}")).To(Equal(ip6GlobalCidr))
				Expect(render("{{ " + constants.V1alpha6FirstIPv6FromNIC + " 0 }}")).To(Equal(ip6GlobalCidr))
			})

			It("populates Gateway6", func() {
				Expect(render("{{ (index .V1alpha6.Net.Devices 0).Gateway6 }}")).To(Equal(gateway6))
			})
		})

		When("a device has only a global IPv6 address (no IPv4)", func() {
			BeforeEach(func() {
				bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
					{
						MacAddress: macAddr1,
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{Gateway: gateway6, IPCIDR: ip6GlobalCidr, IsIPv4: false},
						},
					},
				}
			})

			It("FirstIP/FirstIPFromNIC/IPsFromNIC fall back to the IPv6 address", func() {
				Expect(render("{{ " + constants.V1alpha6FirstIP + " }}")).To(Equal(ip6GlobalCidr))
				Expect(render("{{ " + constants.V1alpha6FirstIPFromNIC + " 0 }}")).To(Equal(ip6GlobalCidr))
				Expect(render("{{ " + constants.V1alpha6IPsFromNIC + " 0 }}")).To(Equal(fmt.Sprint([]string{ip6GlobalCidr})))
			})

			It("the strict FirstIPv4/FirstIPv4FromNIC never fall back (error, template unrendered)", func() {
				str := "{{ " + constants.V1alpha6FirstIPv4 + " }}"
				Expect(render(str)).To(Equal(str))
				str = "{{ " + constants.V1alpha6FirstIPv4FromNIC + " 0 }}"
				Expect(render(str)).To(Equal(str))
			})
		})

		When("a device has only a link-local IPv6 address (no IPv4, no other IPv6)", func() {
			BeforeEach(func() {
				bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
					{
						MacAddress: macAddr1,
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{Gateway: gateway6, IPCIDR: ip6LinkLocalCidr, IsIPv4: false},
						},
					},
				}
			})

			It("IsUsableIP reports false for the link-local address", func() {
				Expect(render("{{ " + constants.V1alpha6IsUsableIP + " \"" + ip6LinkLocalCidr + "\" }}")).To(Equal("false"))
			})

			It("FirstIPv6 returns it raw, unfiltered", func() {
				Expect(render("{{ " + constants.V1alpha6FirstIPv6 + " }}")).To(Equal(ip6LinkLocalCidr))
			})

			It("FirstIP/FirstIPFromNIC degrade gracefully to it rather than erroring", func() {
				Expect(render("{{ " + constants.V1alpha6FirstIP + " }}")).To(Equal(ip6LinkLocalCidr))
				Expect(render("{{ " + constants.V1alpha6FirstIPFromNIC + " 0 }}")).To(Equal(ip6LinkLocalCidr))
			})
		})

		When("a device has both a usable and a link-local IPv6 address (no IPv4)", func() {
			BeforeEach(func() {
				bsArgs.NetworkResults.Results = []network.NetworkInterfaceResult{
					{
						MacAddress: macAddr1,
						IPConfigs: []network.NetworkInterfaceIPConfig{
							{Gateway: gateway6, IPCIDR: ip6LinkLocalCidr, IsIPv4: false},
							{Gateway: gateway6, IPCIDR: ip6GlobalCidr, IsIPv4: false},
						},
					},
				}
			})

			It("FirstIP/FirstIPFromNIC prefer the usable address over the link-local one", func() {
				Expect(render("{{ " + constants.V1alpha6FirstIP + " }}")).To(Equal(ip6GlobalCidr))
				Expect(render("{{ " + constants.V1alpha6FirstIPFromNIC + " 0 }}")).To(Equal(ip6GlobalCidr))
			})

			It("IPsFromNIC returns all IPv6 addresses unfiltered, in order", func() {
				Expect(render("{{ " + constants.V1alpha6IPsFromNIC + " 0 }}")).To(Equal(fmt.Sprint([]string{ip6LinkLocalCidr, ip6GlobalCidr})))
			})
		})
	})

	Context("Invalid template names", func() {
		DescribeTable("returns the original text",
			func(str string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(str))
			},
			Entry("ip1", "{{ "+constants.V1alpha1IP+" \"192.1.0\" }}"),
			Entry("ip2", "{{ "+constants.V1alpha1FirstIPFromNIC+" 5 }}"),
			Entry("ips_1", "{{ "+constants.V1alpha1IPsFromNIC+" 5 }}"),
			Entry("cidr_ip1", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"),
			Entry("cidr_ip2", "{{ ("+constants.V1alpha1FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"),
			Entry("gateway", "{{ (index .V1alpha1.Net.NetworkInterfaces ).Gateway }}"),
			Entry("nameserver", "{{ (index .V1alpha1.Net.NameServers 0) }}"),
		)

		DescribeTable("returns the original text, v1a2 style",
			func(str string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(str))
			},
			Entry("ip1", "{{ "+constants.V1alpha2IP+" \"192.1.0\" }}"),
			Entry("ip2", "{{ "+constants.V1alpha2FirstIPFromNIC+" 5 }}"),
			Entry("ips_1", "{{ "+constants.V1alpha2IPsFromNIC+" 5 }}"),
			Entry("cidr_ip1", "{{ ("+constants.V1alpha2FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"),
			Entry("cidr_ip2", "{{ ("+constants.V1alpha2FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"),
			Entry("gateway", "{{ (index .V1alpha2.Net.NetworkInterfaces ).Gateway }}"),
			Entry("nameserver", "{{ (index .V1alpha2.Net.NameServers 0) }}"),
		)

		DescribeTable("returns the original text, v1a3 style",
			func(str string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(str))
			},
			Entry("ip1", "{{ "+constants.V1alpha3IP+" \"192.1.0\" }}"),
			Entry("ip2", "{{ "+constants.V1alpha3FirstIPFromNIC+" 5 }}"),
			Entry("ips_1", "{{ "+constants.V1alpha3IPsFromNIC+" 5 }}"),
			Entry("cidr_ip1", "{{ ("+constants.V1alpha3FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"),
			Entry("cidr_ip2", "{{ ("+constants.V1alpha3FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"),
			Entry("gateway", "{{ (index .V1alpha3.Net.NetworkInterfaces ).Gateway }}"),
			Entry("nameserver", "{{ (index .V1alpha3.Net.NameServers 0) }}"),
		)

		DescribeTable("returns the original text, v1a4 style",
			func(str string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(str))
			},
			Entry("ip1", "{{ "+constants.V1alpha4IP+" \"192.1.0\" }}"),
			Entry("ip2", "{{ "+constants.V1alpha4FirstIPFromNIC+" 5 }}"),
			Entry("ips_1", "{{ "+constants.V1alpha4IPsFromNIC+" 5 }}"),
			Entry("cidr_ip1", "{{ ("+constants.V1alpha4FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"),
			Entry("cidr_ip2", "{{ ("+constants.V1alpha4FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"),
			Entry("gateway", "{{ (index .V1alpha4.Net.NetworkInterfaces ).Gateway }}"),
			Entry("nameserver", "{{ (index .V1alpha4.Net.NameServers 0) }}"),
		)

		DescribeTable("returns the original text, v1a5 style",
			func(str string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(str))
			},
			Entry("ip1", "{{ "+constants.V1alpha5IP+" \"192.1.0\" }}"),
			Entry("ip2", "{{ "+constants.V1alpha5FirstIPFromNIC+" 5 }}"),
			Entry("ips_1", "{{ "+constants.V1alpha5IPsFromNIC+" 5 }}"),
			Entry("cidr_ip1", "{{ ("+constants.V1alpha5FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"),
			Entry("cidr_ip2", "{{ ("+constants.V1alpha5FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"),
			Entry("gateway", "{{ (index .V1alpha5.Net.NetworkInterfaces ).Gateway }}"),
			Entry("nameserver", "{{ (index .V1alpha5.Net.NameServers 0) }}"),
		)

		DescribeTable("returns the original text, v1a6 style",
			func(str string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(str))
			},
			Entry("ip1", "{{ "+constants.V1alpha6IP+" \"192.1.0\" }}"),
			Entry("ip2", "{{ "+constants.V1alpha6FirstIPFromNIC+" 5 }}"),
			Entry("ips_1", "{{ "+constants.V1alpha6IPsFromNIC+" 5 }}"),
			Entry("cidr_ip1", "{{ ("+constants.V1alpha6FormatIP+" \"192.168.1.37\" \"127.255.255.255\") }}"),
			Entry("cidr_ip2", "{{ ("+constants.V1alpha6FormatIP+" \"192.168.1\" \"255.0.0.0\") }}"),
			Entry("gateway", "{{ (index .V1alpha6.Net.NetworkInterfaces ).Gateway }}"),
			Entry("nameserver", "{{ (index .V1alpha6.Net.NameServers 0) }}"),
			Entry("subnetmask_ipv6", "{{ "+constants.V1alpha6SubnetMask+" \"2001:db8::1/64\" }}"),
			Entry("ip_ipv6", "{{ "+constants.V1alpha6IP+" \"2001:db8::1\" }}"),
			Entry("first_ipv4_from_nic_out_of_bound", "{{ "+constants.V1alpha6FirstIPv4FromNIC+" 5 }}"),
			Entry("first_ipv6_from_nic_out_of_bound", "{{ "+constants.V1alpha6FirstIPv6FromNIC+" 5 }}"),
			Entry("first_ipv6_no_ipv6_available", "{{ "+constants.V1alpha6FirstIPv6+" }}"),
			Entry("prefix_length_invalid", "{{ "+constants.V1alpha6PrefixLength+" \"not-a-cidr\" }}"),
		)
	})

	Context("String has escape characters", func() {
		DescribeTable("return one level of escaped removed",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("skip_data1", "\\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data2", "\\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data3", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data4", "skip \\{\\{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) \\}\\}", "skip {{ (index (index .V1alpha1.Net.Devices 0).IPAddresses 0) }}"),
		)

		DescribeTable("return one level of escaped removed, v1a2 style",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("skip_data1", "\\{\\{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data2", "\\{\\{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) }}", "{{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data3", "{{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data4", "skip \\{\\{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) \\}\\}", "skip {{ (index (index .V1alpha2.Net.Devices 0).IPAddresses 0) }}"),
		)

		DescribeTable("return one level of escaped removed, v1a3 style",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("skip_data1", "\\{\\{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data2", "\\{\\{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}", "{{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data3", "{{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data4", "skip \\{\\{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) \\}\\}", "skip {{ (index (index .V1alpha3.Net.Devices 0).IPAddresses 0) }}"),
		)

		DescribeTable("return one level of escaped removed, v1a4 style",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("skip_data1", "\\{\\{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data2", "\\{\\{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) }}", "{{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data3", "{{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data4", "skip \\{\\{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) \\}\\}", "skip {{ (index (index .V1alpha4.Net.Devices 0).IPAddresses 0) }}"),
		)

		DescribeTable("return one level of escaped removed, v1a5 style",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("skip_data1", "\\{\\{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data2", "\\{\\{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}", "{{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data3", "{{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data4", "skip \\{\\{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) \\}\\}", "skip {{ (index (index .V1alpha5.Net.Devices 0).IPAddresses 0) }}"),
		)

		DescribeTable("return one level of escaped removed, v1a6 style",
			func(str, expected string) {
				fn := vmlifecycle.GetTemplateRenderFunc(vmCtx, bsArgs)
				out := fn("", str)
				Expect(out).To(Equal(expected))
			},
			Entry("skip_data1", "\\{\\{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data2", "\\{\\{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) }}", "{{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data3", "{{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) \\}\\}", "{{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) }}"),
			Entry("skip_data4", "skip \\{\\{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) \\}\\}", "skip {{ (index (index .V1alpha6.Net.Devices 0).IPAddresses 0) }}"),
		)
	})
})
