// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"net"

	"github.com/go-logr/logr"
	vimtypes "github.com/vmware/govmomi/vim25/types"
)

func GuestOSCustomization(results NetworkInterfaceResults,
	logger logr.Logger) ([]vimtypes.CustomizationAdapterMapping, error) {
	mappings := make([]vimtypes.CustomizationAdapterMapping, 0, len(results.Results))

	for i, r := range results.Results {
		adapter := vimtypes.CustomizationIPSettings{
			// Per-adapter is only supported on Windows. Linux only supports the global and ignores this field.
			DnsServerList: r.Nameservers,
		}

		switch {
		case r.DHCP4:
			adapter.Ip = &vimtypes.CustomizationDhcpIpGenerator{}
		case r.NoIPAM:
			adapter.Ip = &vimtypes.CustomizationDisableIpV4{}
		default:
			// GOSC doesn't support multiple IPv4 address per interface so use the first one.
			// Old code only ever set one gateway so do the same here too.
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
				if ipConfig.Gateway != "" {
					adapter.Gateway = []string{ipConfig.Gateway}
				}
				break
			}

		}

		switch {
		case r.DHCP6:
			adapter.IpV6Spec = &vimtypes.CustomizationIPSettingsIpV6AddressSpec{
				Ip: []vimtypes.BaseCustomizationIpV6Generator{
					&vimtypes.CustomizationDhcpIpV6Generator{},
				},
			}
		default:
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
					SubnetMask: int32(ones), //nolint:gosec // disable G115
				})
				if ipConfig.Gateway != "" {
					adapter.IpV6Spec.Gateway = append(adapter.IpV6Spec.Gateway, ipConfig.Gateway)
				}
			}
		}

		// When adapter.Ip is nil, the vSphere API requires it to be set.
		// Set it to disable IPv4, which handles both IPv6-only and completely unconfigured cases.
		if adapter.Ip == nil {
			adapter.Ip = &vimtypes.CustomizationDisableIpV4{}
			if adapter.IpV6Spec != nil {
				// IPv6-only: disable IPv4
				logger.Info("IPv6-only: set adapter.Ip to disable IPv4",
					"adapterIndex", i,
					"macAddress", r.MacAddress)
			} else {
				// Completely unconfigured: disable IPv4 to satisfy vSphere API requirement
				// This matches Linux behavior where an interface can exist without an IP address
				logger.Info("Unconfigured interface: set adapter.Ip to disable IPv4",
					"adapterIndex", i,
					"macAddress", r.MacAddress)
			}
		}

		logger.V(4).Info("Final adapter state",
			"adapterIndex", i,
			"macAddress", r.MacAddress,
			"adapterIp", adapter.Ip,
			"hasIpV6Spec", adapter.IpV6Spec != nil)

		mappings = append(mappings, vimtypes.CustomizationAdapterMapping{
			MacAddress: r.MacAddress,
			Adapter:    adapter,
		})
	}

	return mappings, nil
}
