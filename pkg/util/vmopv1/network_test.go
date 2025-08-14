// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmopv1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1util "github.com/vmware-tanzu/vm-operator/pkg/util/vmopv1"
)

var _ = Describe("ValidateHostAndDomainName", func() {
	const (
		vmName     = "my-vm"
		hostName   = vmName + "-name"
		domainName = "com"
		ip4        = "1.2.3.4"
		ip6        = "2001:db8:3333:4444:5555:6666:7777:8888"
	)

	var (
		fieldMetadataName = field.NewPath("metadata", "name")
		fieldNetwork      = field.NewPath("spec", "network")
		fieldHostName     = fieldNetwork.Child("hostName")
		fieldDomainName   = fieldNetwork.Child("domainName")

		a1  = "a"
		a16 = strings.Repeat(a1, 16)
		a63 = strings.Repeat(a1, 63)
		a64 = strings.Repeat(a1, 64)
	)

	DescribeTable("Tests",
		func(vm vmopv1.VirtualMachine, err error) {
			if err == nil {
				Ω(vmopv1util.ValidateHostAndDomainName(vm)).Should(Succeed())
			} else {
				Ω(vmopv1util.ValidateHostAndDomainName(vm)).Should(MatchError(err))
			}
		},
		Entry(
			"vm name is empty",
			vmopv1.VirtualMachine{},
			field.Invalid(fieldMetadataName, "", vmopv1util.ErrInvalidHostName),
		),
		Entry(
			"vm name is valid host name",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
			},
			nil,
		),
		Entry(
			"network spec is nil",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
			},
			nil,
		),
		Entry(
			"host name is empty",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: "",
					},
				},
			},
			nil,
		),
		Entry(
			"host name is valid",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: hostName,
					},
				},
			},
			nil,
		),
		Entry(
			"host name exceeds 15 characters",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: a16,
					},
				},
			},
			nil,
		),
		Entry(
			"host name exceeds 15 characters on Windows (determined by sysprep)",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: a16,
					},
					Bootstrap: &vmopv1.VirtualMachineBootstrapSpec{
						Sysprep: &vmopv1.VirtualMachineBootstrapSysprepSpec{},
					},
				},
			},
			field.Invalid(fieldHostName, a16, vmopv1util.ErrInvalidHostNameWindows),
		),
		Entry(
			"host name exceeds 15 characters on Windows (determined by guest ID)",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: a16,
					},
					GuestID: "windows2022srvNext_64Guest",
				},
			},
			field.Invalid(fieldHostName, a16, vmopv1util.ErrInvalidHostNameWindows),
		),
		Entry(
			"host name exceeds 63 characters",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: a64,
					},
				},
			},
			field.Invalid(fieldHostName, a64, vmopv1util.ErrInvalidHostName),
		),
		Entry(
			"host name has invalid character",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: "-" + hostName,
					},
				},
			},
			field.Invalid(fieldHostName, "-"+hostName, vmopv1util.ErrInvalidHostName),
		),
		Entry(
			"host name is ip4 address",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: ip4,
					},
				},
			},
			nil,
		),
		Entry(
			"host name is ip4 address w non-empty domain name",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName:   ip4,
						DomainName: domainName,
					},
				},
			},
			field.Invalid(fieldHostName, ip4, vmopv1util.ErrInvalidHostNameIPWithDomainName),
		),
		Entry(
			"host name is ip6 address",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName: ip6,
					},
				},
			},
			nil,
		),
		Entry(
			"host name is ip6 address w non-empty domain name",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName:   ip6,
						DomainName: domainName,
					},
				},
			},
			field.Invalid(fieldHostName, ip6, vmopv1util.ErrInvalidHostNameIPWithDomainName),
		),
		Entry(
			"host name and domain name combined exceed 255 characters",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName:   a63,
						DomainName: fmt.Sprintf("%[1]s.%[1]s.%[1]s.com", a63),
					},
				},
			},
			field.Invalid(fieldNetwork, fmt.Sprintf("%[1]s.%[1]s.%[1]s.%[1]s.com", a63), vmopv1util.ErrInvalidHostAndDomainName),
		),
		Entry(
			"domain name begins with leading dash",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName:   hostName,
						DomainName: "-" + domainName,
					},
				},
			},
			field.Invalid(fieldDomainName, "-"+domainName, vmopv1util.ErrInvalidDomainName),
		),
		Entry(
			"domain name is a single character",
			vmopv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name: vmName,
				},
				Spec: vmopv1.VirtualMachineSpec{
					Network: &vmopv1.VirtualMachineNetworkSpec{
						HostName:   hostName,
						DomainName: a1,
					},
				},
			},
			field.Invalid(fieldDomainName, a1, vmopv1util.ErrInvalidDomainName),
		),
	)
})
