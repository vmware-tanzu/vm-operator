// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package netplan_test

import (
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan/schema"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

var _ = DescribeTable("MarshalYAML",
	func(expYAML string, in netplan.Config, expErr error) {
		// Marshal
		data, err := netplan.MarshalYAML(in)
		if expErr != nil {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
			Expect(string(data)).To(MatchYAML(expYAML))
		}
	},
	Entry(
		"single static ip",
		netplanSingleStaticIP,
		netplan.Config{
			Network: netplan.Network{
				Version:  2,
				Renderer: ptr.To(netplan.Renderer("networkd")),
				Ethernets: map[string]netplan.Ethernet{
					"enp3s0": {
						Addresses: []netplan.Address{
							{
								String: ptr.To("10.10.10.2/24"),
							},
						},
					},
				},
			},
		},
		nil,
	),

	Entry(
		"single dhcp4 ip",
		netplanSingleDHCP4IPBoolTrueFalse,
		netplan.Config{
			Network: netplan.Network{
				Version:  2,
				Renderer: ptr.To(netplan.Renderer("networkd")),
				Ethernets: map[string]netplan.Ethernet{
					"enp3s0": {
						Dhcp4: ptr.To(true),
					},
				},
			},
		},
		nil,
	),

	Entry(
		"mixed static and dhcp4 ips with overrides",
		netplanStaticAndDHCPWithOverridesBoolTrueFalse,
		netplan.Config{
			Network: netplan.Network{
				Version: 2,
				Ethernets: map[string]netplan.Ethernet{
					"enp5s0": {
						Addresses: []netplan.Address{
							{
								String: ptr.To("10.10.10.2/24"),
							},
						},
						Dhcp4: ptr.To(false),
					},
					"enp6s0": {
						Dhcp4: ptr.To(true),
						Dhcp4Overrides: &schema.DHCPOverrides{
							RouteMetric: ptr.To(int64(200)),
						},
					},
				},
			},
		},
		nil,
	),
)

var _ = DescribeTable("Unmarshal",
	func(in string, expObj netplan.Config, expErr error) {
		obj, err := netplan.UnmarshalYAML([]byte(in))
		if expErr != nil {
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(expErr))
		} else {
			Expect(err).ToNot(HaveOccurred())
			Expect(obj).To(Equal(expObj), cmp.Diff(obj, expObj))
		}

	},
	Entry(
		"single static ip",
		netplanSingleStaticIP,
		netplan.Config{
			Network: netplan.Network{
				Version:  2,
				Renderer: ptr.To(netplan.Renderer("networkd")),
				Ethernets: map[string]netplan.Ethernet{
					"enp3s0": {
						Addresses: []netplan.Address{
							{
								String: ptr.To("10.10.10.2/24"),
							},
						},
					},
				},
			},
		},
		nil,
	),

	Entry(
		"single dhcp4 ip",
		netplanSingleDHCP4IPBoolYesNo,
		netplan.Config{
			Network: netplan.Network{
				Version:  2,
				Renderer: ptr.To(netplan.Renderer("networkd")),
				Ethernets: map[string]netplan.Ethernet{
					"enp3s0": {
						Dhcp4: ptr.To(true),
					},
				},
			},
		},
		nil,
	),

	Entry(
		"mixed static and dhcp4 ips with overrides",
		netplanStaticAndDHCPWithOverridesBoolYesNo,
		netplan.Config{
			Network: netplan.Network{
				Version: 2,
				Ethernets: map[string]netplan.Ethernet{
					"enp5s0": {
						Addresses: []netplan.Address{
							{
								String: ptr.To("10.10.10.2/24"),
							},
						},
						Dhcp4: ptr.To(false),
					},
					"enp6s0": {
						Dhcp4: ptr.To(true),
						Dhcp4Overrides: &schema.DHCPOverrides{
							RouteMetric: ptr.To(int64(200)),
						},
					},
				},
			},
		},
		nil,
	),
)

const netplanSingleStaticIP = `network:
  version: 2
  renderer: networkd
  ethernets:
    enp3s0:
      addresses:
        - 10.10.10.2/24`

const netplanSingleDHCP4IPBoolTrueFalse = `network:
  version: 2
  renderer: networkd
  ethernets:
    enp3s0:
      dhcp4: true`

const netplanStaticAndDHCPWithOverridesBoolTrueFalse = `network:
  version: 2
  ethernets:
    enp5s0:
      dhcp4: false
      addresses:
        - 10.10.10.2/24
    enp6s0:
      dhcp4: true
      dhcp4-overrides:
        route-metric: 200`

const netplanSingleDHCP4IPBoolYesNo = `network:
  version: 2
  renderer: networkd
  ethernets:
    enp3s0:
      dhcp4: yes`

const netplanStaticAndDHCPWithOverridesBoolYesNo = `network:
  version: 2
  ethernets:
    enp5s0:
      dhcp4: no
      addresses:
        - 10.10.10.2/24
    enp6s0:
      dhcp4: yes
      dhcp4-overrides:
        route-metric: 200`
