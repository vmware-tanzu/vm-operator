// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"strings"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

func NetPlanCustomization(result NetworkInterfaceResults) (*netplan.Network, error) {
	netPlan := &netplan.Network{
		Version:   constants.NetPlanVersion,
		Ethernets: make(map[string]netplan.Ethernet),
	}

	for _, r := range result.Results {
		npEth := netplan.Ethernet{
			Match: &netplan.Match{
				Macaddress: ptr.To(NormalizeNetplanMac(r.MacAddress)),
			},
			SetName: &r.GuestDeviceName,
			Nameservers: &netplan.Nameserver{
				Addresses: r.Nameservers,
				Search:    r.SearchDomains,
			},
		}

		if r.MTU > 0 {
			npEth.MTU = &r.MTU
		}

		npEth.Dhcp4 = &r.DHCP4
		npEth.Dhcp6 = &r.DHCP6
		// Right now we can set the same value as DHCPv6 configuration
		// and in some future separate/specialize if required
		npEth.AcceptRa = &r.DHCP6

		if !*npEth.Dhcp4 {
			for i := range r.IPConfigs {
				ipConfig := r.IPConfigs[i]
				if ipConfig.IsIPv4 {
					if ipConfig.Gateway != "" {
						if npEth.Gateway4 == nil || *npEth.Gateway4 == "" {
							npEth.Gateway4 = &ipConfig.Gateway
						}
					}
					npEth.Addresses = append(
						npEth.Addresses,
						netplan.Address{
							String: &ipConfig.IPCIDR,
						},
					)
				}
			}
		}
		if !*npEth.Dhcp6 {
			for i := range r.IPConfigs {
				ipConfig := r.IPConfigs[i]
				if !ipConfig.IsIPv4 {
					if ipConfig.Gateway != "" {
						if npEth.Gateway6 == nil || *npEth.Gateway6 == "" {
							npEth.Gateway6 = &ipConfig.Gateway
						}
					}
					npEth.Addresses = append(
						npEth.Addresses,
						netplan.Address{
							String: &ipConfig.IPCIDR,
						},
					)
				}
			}
		}

		for i := range r.Routes {
			route := r.Routes[i]

			var metric *int64
			if route.Metric != 0 {
				metric = ptr.To(int64(route.Metric))
			}

			npEth.Routes = append(
				npEth.Routes,
				netplan.Route{
					To:     &route.To,
					Metric: metric,
					Via:    &route.Via,
				},
			)
		}

		netPlan.Ethernets[r.Name] = npEth
	}

	return netPlan, nil
}

// NormalizeNetplanMac normalizes the mac address format to one compatible with netplan.
func NormalizeNetplanMac(mac string) string {
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToLower(mac)
}
