// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"net"

	vimtypes "github.com/vmware/govmomi/vim25/types"
)

func GuestOSCustomization(results NetworkInterfaceResults) ([]vimtypes.CustomizationAdapterMapping, error) {
	mappings := make([]vimtypes.CustomizationAdapterMapping, 0, len(results.Results))

	for _, r := range results.Results {
		adapter := vimtypes.CustomizationIPSettings{
			DnsServerList: r.Nameservers,
		}

		if r.DHCP4 {
			adapter.Ip = &vimtypes.CustomizationDhcpIpGenerator{}
		} else {
			// GOSC doesn't support multiple IPv4 address per interface so use the first one. Old code
			// only ever set one gateway so do the same here too.
			for _, ipConfig := range r.IPConfigs {
				if !ipConfig.IsIPv4 {
					continue
				}

				ip, ipNet, err := net.ParseCIDR(ipConfig.IPCIDR)
				if err != nil {
					return nil, err
				}
				subnetMask := net.CIDRMask(ipNet.Mask.Size())

				adapter.Ip = &vimtypes.CustomizationFixedIp{IpAddress: ip.String()}
				adapter.SubnetMask = net.IP(subnetMask).String()
				adapter.Gateway = []string{ipConfig.Gateway}
				break
			}
		}

		if r.DHCP6 {
			adapter.IpV6Spec = &vimtypes.CustomizationIPSettingsIpV6AddressSpec{
				Ip: []vimtypes.BaseCustomizationIpV6Generator{
					&vimtypes.CustomizationDhcpIpV6Generator{},
				},
			}
		} else {
			for _, ipConfig := range r.IPConfigs {
				if ipConfig.IsIPv4 {
					continue
				}

				ip, ipNet, err := net.ParseCIDR(ipConfig.IPCIDR)
				if err != nil {
					return nil, err
				}
				ones, _ := ipNet.Mask.Size()

				if adapter.IpV6Spec == nil {
					adapter.IpV6Spec = &vimtypes.CustomizationIPSettingsIpV6AddressSpec{}
				}
				adapter.IpV6Spec.Ip = append(adapter.IpV6Spec.Ip, &vimtypes.CustomizationFixedIpV6{
					IpAddress:  ip.String(),
					SubnetMask: int32(ones),
				})
				adapter.IpV6Spec.Gateway = append(adapter.IpV6Spec.Gateway, ipConfig.Gateway)
			}
		}

		mappings = append(mappings, vimtypes.CustomizationAdapterMapping{
			MacAddress: r.MacAddress,
			Adapter:    adapter,
		})
	}

	return mappings, nil
}
