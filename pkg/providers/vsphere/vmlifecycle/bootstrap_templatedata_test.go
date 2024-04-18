// Copyright (c) 2021-2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
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
	})
})
