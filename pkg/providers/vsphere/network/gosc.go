// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"fmt"
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

		// When only IPv6 is configured (no IPv4 addresses, no DHCP4), the vSphere API
		// requires adapter.Ip to be set. Set it to DHCP as a fallback.
		conditionCheck := adapter.Ip == nil && !r.NoIPAM && (adapter.IpV6Spec != nil || r.DHCP6)
		if logger.GetSink() != nil {
			logger.V(4).Info("IPv6-only fix condition check",
				"adapterIndex", i,
				"adapterIpIsNil", adapter.Ip == nil,
				"noIPAM", r.NoIPAM,
				"hasIpV6Spec", adapter.IpV6Spec != nil,
				"dhcp6", r.DHCP6,
				"conditionMet", conditionCheck)
		}

		if conditionCheck {
			adapter.Ip = &vimtypes.CustomizationDhcpIpGenerator{}
			if logger.GetSink() != nil {
				logger.Info("Applied IPv6-only fix: set adapter.Ip to DHCP generator",
					"adapterIndex", i,
					"macAddress", r.MacAddress)
			}
		}

		if logger.GetSink() != nil {
			logger.V(4).Info("Final adapter state",
				"adapterIndex", i,
				"macAddress", r.MacAddress,
				"adapterIp", adapter.Ip,
				"hasIpV6Spec", adapter.IpV6Spec != nil)
			if adapter.Ip != nil {
				switch v := adapter.Ip.(type) {
				case *vimtypes.CustomizationDhcpIpGenerator:
					logger.V(5).Info("Adapter IP type", "adapterIndex", i, "type", "CustomizationDhcpIpGenerator")
				case *vimtypes.CustomizationFixedIp:
					logger.V(5).Info("Adapter IP type", "adapterIndex", i, "type", "CustomizationFixedIp", "address", v.IpAddress)
				default:
					logger.V(5).Info("Adapter IP type", "adapterIndex", i, "type", fmt.Sprintf("%T", v))
				}
			}
		}

		mappings = append(mappings, vimtypes.CustomizationAdapterMapping{
			MacAddress: r.MacAddress,
			Adapter:    adapter,
		})
	}

	return mappings, nil
}
