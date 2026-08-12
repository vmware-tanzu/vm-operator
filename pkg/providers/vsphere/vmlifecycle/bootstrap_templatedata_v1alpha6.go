// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

// This file contains everything specific to V1alpha6's template functions.
// Unlike v1alpha1-v1alpha5 (which are legacy and share a single file), this
// version gets its own dedicated file so that scaffolding the next version
// (hack/new-schema-version.py) can copy this whole file and mechanically
// rename tokens, rather than needing to surgically patch a shared file.

package vmlifecycle

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"text/template"

	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

func toTemplateNetworkStatusV1A6(bsArgs *BootstrapArgs) []vmopv1.NetworkDeviceStatus {
	networkDevicesStatus := make([]vmopv1.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := vmopv1.NetworkDeviceStatus{
			MacAddress: macAddr,
		}

		for _, ipConfig := range result.IPConfigs {
			if ipConfig.IsIPv4 {
				if status.Gateway4 == "" {
					status.Gateway4 = ipConfig.Gateway
				}

				status.IPAddresses = append(status.IPAddresses, ipConfig.IPCIDR)
			} else {
				if status.Gateway6 == "" {
					status.Gateway6 = ipConfig.Gateway
				}

				// Unfiltered, exactly parallel to the IPv4 case above: a
				// device that legitimately only has a link-local address
				// (e.g. an unnumbered router interface) keeps it here.
				// Callers who want to exclude link-local/loopback/
				// unspecified addresses use V1alpha6_IsUsableIP explicitly.
				status.IPv6Addresses = append(status.IPv6Addresses, ipConfig.IPCIDR)
			}
		}

		networkDevicesStatus = append(networkDevicesStatus, status)
	}

	return networkDevicesStatus
}

// v1a6IsUsableIP reports whether ip (a bare IP or CIDR string) is usable
// off the local link -- i.e. not unspecified, loopback, or link-local
// (unicast or multicast). Works for either address family. Unparsable
// input is reported as not usable rather than an error, so a caller
// filtering a list doesn't have one bad entry abort the whole template
// render. Exposed as V1alpha6_IsUsableIP so callers can filter explicitly;
// also used internally by v1a6FirstIPForDevice's fallback below.
func v1a6IsUsableIP(ip string) bool {
	parsed, _, err := net.ParseCIDR(ip)
	if err != nil {
		parsed = net.ParseIP(ip)
		if parsed == nil {
			return false
		}
	}
	return !parsed.IsUnspecified() && !parsed.IsLoopback() &&
		!parsed.IsLinkLocalUnicast() && !parsed.IsLinkLocalMulticast()
}

// v1a6FirstIPForDevice picks a device's first IPv4 address if it has one
// (unchanged behavior). Otherwise it falls back to the device's IPv6
// addresses, preferring a usable one but degrading to whatever is
// available (e.g. a legitimately link-local-only router interface) rather
// than erroring. Callers that need a specific, non-degrading family should
// use v1a6FirstIPv4/v1a6FirstIPv6 instead.
func v1a6FirstIPForDevice(dev vmopv1.NetworkDeviceStatus) (string, error) {
	if len(dev.IPAddresses) > 0 {
		return dev.IPAddresses[0], nil
	}
	for _, addr := range dev.IPv6Addresses {
		if v1a6IsUsableIP(addr) {
			return addr, nil
		}
	}
	if len(dev.IPv6Addresses) > 0 {
		return dev.IPv6Addresses[0], nil
	}
	return "", errors.New("no available network device, check with VI admin")
}

// Get the first IP address from the first NIC, preferring IPv4 and falling
// back to IPv6 (see v1a6FirstIPForDevice).
func v1a6FirstIP(devices []vmopv1.NetworkDeviceStatus) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	return v1a6FirstIPForDevice(devices[0])
}

// Get the first NIC's MAC address.
func v1a6FirstNicMacAddr(devices []vmopv1.NetworkDeviceStatus) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	return devices[0].MacAddress, nil
}

// Get the first IP address from the ith NIC, preferring IPv4 and falling
// back to IPv6 (see v1a6FirstIPForDevice).
func v1a6FirstIPFromNIC(devices []vmopv1.NetworkDeviceStatus, index int) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	if index >= len(devices) {
		return "", errors.New("index out of bound")
	}
	return v1a6FirstIPForDevice(devices[index])
}

// Get all IP addresses from the ith NIC: its IPv4 addresses if it has any
// (unchanged behavior), otherwise its (unfiltered) IPv6 addresses.
func v1a6IPsFromNIC(devices []vmopv1.NetworkDeviceStatus, index int) ([]string, error) {
	if len(devices) == 0 {
		return []string{""}, errors.New("no available network device, check with VI admin")
	}
	if index >= len(devices) {
		return []string{""}, errors.New("index out of bound")
	}
	dev := devices[index]
	if len(dev.IPAddresses) > 0 {
		return dev.IPAddresses, nil
	}
	return dev.IPv6Addresses, nil
}

// Get the first IPv4 address from the first NIC. Unlike v1a6FirstIP, this
// never falls back to IPv6.
func v1a6FirstIPv4(devices []vmopv1.NetworkDeviceStatus) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	if len(devices[0].IPAddresses) == 0 {
		return "", errors.New("no available IPv4 address, check with VI admin")
	}
	return devices[0].IPAddresses[0], nil
}

// Get the first IPv6 address from the first NIC, unfiltered (may be
// link-local). Unlike v1a6FirstIP, this never falls back to IPv4.
func v1a6FirstIPv6(devices []vmopv1.NetworkDeviceStatus) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	if len(devices[0].IPv6Addresses) == 0 {
		return "", errors.New("no available IPv6 address, check with VI admin")
	}
	return devices[0].IPv6Addresses[0], nil
}

// Get the first IPv4 address from the ith NIC. Unlike v1a6FirstIPFromNIC,
// this never falls back to IPv6.
func v1a6FirstIPv4FromNIC(devices []vmopv1.NetworkDeviceStatus, index int) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	if index >= len(devices) {
		return "", errors.New("index out of bound")
	}
	if len(devices[index].IPAddresses) == 0 {
		return "", errors.New("no available IPv4 address, check with VI admin")
	}
	return devices[index].IPAddresses[0], nil
}

// Get the first IPv6 address from the ith NIC, unfiltered (may be
// link-local). Unlike v1a6FirstIPFromNIC, this never falls back to IPv4.
func v1a6FirstIPv6FromNIC(devices []vmopv1.NetworkDeviceStatus, index int) (string, error) {
	if len(devices) == 0 {
		return "", errors.New("no available network device, check with VI admin")
	}
	if index >= len(devices) {
		return "", errors.New("index out of bound")
	}
	if len(devices[index].IPv6Addresses) == 0 {
		return "", errors.New("no available IPv6 address, check with VI admin")
	}
	return devices[index].IPv6Addresses[0], nil
}

// Format the first occurred count of nameservers with specific delimiter.
func v1a6FormatNameservers(nameservers []string, count int, delimiter string) (string, error) {
	if len(nameservers) == 0 {
		return "", errors.New("no available nameservers, check with VI admin")
	}
	if count < 0 || count >= len(nameservers) {
		return strings.Join(nameservers, delimiter), nil
	}
	return strings.Join(nameservers[:count], delimiter), nil
}

// Get subnet mask from a CIDR notation IP address and prefix length.
// IPv4-only -- errors on IPv6 input instead of returning a garbage 4-byte
// mask; use v1a6PrefixLength for a dual-stack-safe alternative.
func v1a6SubnetMask(cidr string) (string, error) {
	ip, ipv4Net, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", err
	}
	if ip.To4() == nil {
		return "", fmt.Errorf("SubnetMask only supports IPv4 CIDRs, use PrefixLength for IPv6: %s", cidr)
	}
	netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
	return netmask, nil
}

// Format an IP address with default netmask CIDR. IPv4-only -- errors on
// IPv6 input instead of returning a bogus /0 (IPv6 has no default classful
// netmask).
func v1a6IP(addr string) (string, error) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return "", errors.New("input IP address not valid")
	}
	if ip.To4() == nil {
		return "", fmt.Errorf("IP only supports IPv4 addresses, IPv6 has no default netmask: %s", addr)
	}
	defaultMask := ip.DefaultMask()
	ones, _ := defaultMask.Size()
	return addr + "/" + strconv.Itoa(ones), nil
}

// Format an IP address with network length(eg. /24) or decimal
// notation (eg. 255.255.255.0). Format an IP/CIDR with updated mask.
// An empty mask causes just the IP to be returned.
func v1a6FormatIP(s string, mask string) (string, error) {
	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		ip = net.ParseIP(s)
		if ip == nil {
			return "", fmt.Errorf("input IP address not valid")
		}
	}
	s = ip.String()
	if mask == "" {
		return s, nil
	}
	if strings.HasPrefix(mask, "/") {
		s += mask
		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", err
		}
		return s, nil
	}
	maskIP := net.ParseIP(mask)
	if maskIP == nil {
		return "", fmt.Errorf("mask is an invalid IP")
	}
	maskIPBytes := maskIP.To4()
	if len(maskIPBytes) == 0 {
		maskIPBytes = maskIP.To16()
	}
	ipNet := net.IPNet{IP: ip, Mask: net.IPMask(maskIPBytes)}
	s = ipNet.String()
	if _, _, err := net.ParseCIDR(s); err != nil {
		return "", fmt.Errorf("invalid ip net: %s", s)
	}
	return s, nil
}

// PrefixLength returns the numeric network prefix length of an IPv4 or
// IPv6 CIDR, e.g. 24 or 64 -- the dual-stack-safe alternative to
// SubnetMask. Compose with FormatIP via printf "/%d" to recombine an IP
// with a prefix length taken from a different CIDR.
func v1a6PrefixLength(cidr string) (int, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return 0, err
	}
	ones, _ := ipNet.Mask.Size()
	return ones, nil
}

func v1a6TemplateFunctions(
	networkStatusV1A6 vmopv1.NetworkStatus,
	networkDevicesStatusV1A6 []vmopv1.NetworkDeviceStatus) map[string]any {

	return template.FuncMap{
		constants.V1alpha6FirstIP: func() (string, error) {
			return v1a6FirstIP(networkDevicesStatusV1A6)
		},
		constants.V1alpha6FirstNicMacAddr: func() (string, error) {
			return v1a6FirstNicMacAddr(networkDevicesStatusV1A6)
		},
		constants.V1alpha6FirstIPFromNIC: func(index int) (string, error) {
			return v1a6FirstIPFromNIC(networkDevicesStatusV1A6, index)
		},
		constants.V1alpha6IPsFromNIC: func(index int) ([]string, error) {
			return v1a6IPsFromNIC(networkDevicesStatusV1A6, index)
		},
		constants.V1alpha6FormatNameservers: func(count int, delimiter string) (string, error) {
			return v1a6FormatNameservers(networkStatusV1A6.Nameservers, count, delimiter)
		},
		constants.V1alpha6SubnetMask: v1a6SubnetMask,
		constants.V1alpha6IP:         v1a6IP,
		constants.V1alpha6FormatIP:   v1a6FormatIP,
		constants.V1alpha6FirstIPv4: func() (string, error) {
			return v1a6FirstIPv4(networkDevicesStatusV1A6)
		},
		constants.V1alpha6FirstIPv6: func() (string, error) {
			return v1a6FirstIPv6(networkDevicesStatusV1A6)
		},
		constants.V1alpha6FirstIPv4FromNIC: func(index int) (string, error) {
			return v1a6FirstIPv4FromNIC(networkDevicesStatusV1A6, index)
		},
		constants.V1alpha6FirstIPv6FromNIC: func(index int) (string, error) {
			return v1a6FirstIPv6FromNIC(networkDevicesStatusV1A6, index)
		},
		constants.V1alpha6IsUsableIP:   v1a6IsUsableIP,
		constants.V1alpha6PrefixLength: v1a6PrefixLength,
	}
}
