// Copyright (c) 2023 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package network

import (
	"strings"

	"github.com/vmware-tanzu/vm-operator/pkg/vmprovider/providers/vsphere2/constants"
)

// Netplan representation described in https://via.vmw.com/cloud-init-netplan // FIXME: 404.
type Netplan struct {
	Version   int                        `yaml:"version,omitempty"`
	Ethernets map[string]NetplanEthernet `yaml:"ethernets,omitempty"`
}

type NetplanEthernet struct {
	Match       NetplanEthernetMatch      `yaml:"match,omitempty"`
	SetName     string                    `yaml:"set-name,omitempty"`
	Dhcp4       bool                      `yaml:"dhcp4,omitempty"`
	Dhcp6       bool                      `yaml:"dhcp6,omitempty"`
	Addresses   []string                  `yaml:"addresses,omitempty"`
	Gateway4    string                    `yaml:"gateway4,omitempty"`
	Gateway6    string                    `yaml:"gateway6,omitempty"`
	MTU         int64                     `yaml:"mtu,omitempty"`
	Nameservers NetplanEthernetNameserver `yaml:"nameservers,omitempty"`
	Routes      []NetplanEthernetRoute    `yaml:"routes,omitempty"`
}

type NetplanEthernetMatch struct {
	MacAddress string `yaml:"macaddress,omitempty"`
}

type NetplanEthernetNameserver struct {
	Addresses []string `yaml:"addresses,omitempty"`
	Search    []string `yaml:"search,omitempty"`
}

type NetplanEthernetRoute struct {
	To     string `yaml:"to"`
	Via    string `yaml:"via"`
	Metric int32  `yaml:"metric,omitempty"`
}

func NetPlanCustomization(result NetworkInterfaceResults) (*Netplan, error) {
	netPlan := &Netplan{
		Version:   constants.NetPlanVersion,
		Ethernets: make(map[string]NetplanEthernet),
	}

	for _, r := range result.Results {
		npEth := NetplanEthernet{
			Match: NetplanEthernetMatch{
				MacAddress: NormalizeNetplanMac(r.MacAddress),
			},
			SetName: r.Name,
			MTU:     r.MTU,
			Nameservers: NetplanEthernetNameserver{
				Addresses: r.Nameservers,
				Search:    r.SearchDomains,
			},
		}

		npEth.Dhcp4 = r.DHCP4
		npEth.Dhcp6 = r.DHCP6

		if !npEth.Dhcp4 {
			for _, ipConfig := range r.IPConfigs {
				if ipConfig.IsIPv4 {
					if npEth.Gateway4 == "" {
						npEth.Gateway4 = ipConfig.Gateway
					}
					npEth.Addresses = append(npEth.Addresses, ipConfig.IPCIDR)
				}
			}
		}
		if !npEth.Dhcp6 {
			for _, ipConfig := range r.IPConfigs {
				if !ipConfig.IsIPv4 {
					if npEth.Gateway6 == "" {
						npEth.Gateway6 = ipConfig.Gateway
					}
					npEth.Addresses = append(npEth.Addresses, ipConfig.IPCIDR)
				}
			}
		}

		for _, route := range r.Routes {
			npEth.Routes = append(npEth.Routes, NetplanEthernetRoute(route))
		}

		netPlan.Ethernets[npEth.SetName] = npEth
	}

	return netPlan, nil
}

// NormalizeNetplanMac normalizes the mac address format to one compatible with netplan.
func NormalizeNetplanMac(mac string) string {
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToLower(mac)
}
