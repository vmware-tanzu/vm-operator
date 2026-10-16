// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"fmt"
	"strings"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
	"github.com/vmware-tanzu/vm-operator/pkg/util/netplan"
	"github.com/vmware-tanzu/vm-operator/pkg/util/ptr"
)

// NetPlanCustomization builds CloudInit network configuration from the effective
// interface settings, rejecting DHCP overrides incompatible with networkd.
func NetPlanCustomization(
	bootstraps []Bootstrap,
	vlans []vmopv1.VirtualMachineNetworkVLANSpec) (*netplan.Network, error) {

	netPlan := &netplan.Network{
		Version:   constants.NetPlanVersion,
		Ethernets: make(map[string]netplan.Ethernet),
	}

	for _, b := range bootstraps {
		npEth := netplan.Ethernet{
			Match: &netplan.Match{
				Macaddress: ptr.To(NormalizeNetplanMac(b.MacAddress)),
			},
			SetName: &b.GuestDeviceName,
			Nameservers: &netplan.Nameserver{
				Addresses: b.Nameservers,
				Search:    b.SearchDomains,
			},
		}

		if b.MTU > 0 {
			npEth.MTU = &b.MTU
		}

		npEth.Dhcp4 = &b.DHCP4
		npEth.Dhcp6 = &b.DHCP6
		npEth.AcceptRa = &b.AcceptRA

		if b.DHCP4 && b.DHCP6 && b.IgnoreDHCP4Routes != b.IgnoreDHCP6Routes {
			return nil, fmt.Errorf("interface %q: dual-stack DHCP requires identical DHCP overrides; "+
				"set both gateway4 and gateway6 to None to ignore DHCP routes", b.Name)
		}

		if b.DHCP4 {
			npEth.Dhcp4Overrides = netPlanDHCPOverrides(b.IgnoreDHCPDNS, b.IgnoreDHCP4Routes)
		}
		if b.DHCP6 {
			npEth.Dhcp6Overrides = netPlanDHCPOverrides(b.IgnoreDHCPDNS, b.IgnoreDHCP6Routes)
		}

		if !*npEth.Dhcp4 {
			for i := range b.IPConfigs {
				ipConfig := b.IPConfigs[i]
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
			for i := range b.IPConfigs {
				ipConfig := b.IPConfigs[i]
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

		for i := range b.Routes {
			route := b.Routes[i]

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

		netPlan.Ethernets[b.Name] = npEth
	}

	if len(vlans) > 0 {
		netPlan.Vlans = make(map[string]netplan.VLAN, len(vlans))
		for _, vlan := range vlans {
			npVlan := netplan.VLAN{
				ID:   ptr.To(vlan.ID),
				Link: ptr.To(vlan.Link),
				// Explicitly set dhcp4: false, dhcp6: false, and accept-ra: false
				// to ensure they remain L2-only until we support user configures them.
				Dhcp4:    ptr.To(false),
				Dhcp6:    ptr.To(false),
				AcceptRa: ptr.To(false),
			}

			netPlan.Vlans[vlan.Name] = npVlan
		}
	}

	return netPlan, nil
}

func netPlanDHCPOverrides(ignoreDNS, ignoreRoutes bool) *netplan.DHCPOverrides {
	if !ignoreDNS && !ignoreRoutes {
		return nil
	}
	overrides := &netplan.DHCPOverrides{}
	if ignoreDNS {
		overrides.UseDNS = ptr.To(false)
	}
	if ignoreRoutes {
		overrides.UseRoutes = ptr.To(false)
	}
	return overrides
}

// NormalizeNetplanMac normalizes the mac address format to one compatible with netplan.
func NormalizeNetplanMac(mac string) string {
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToLower(mac)
}
