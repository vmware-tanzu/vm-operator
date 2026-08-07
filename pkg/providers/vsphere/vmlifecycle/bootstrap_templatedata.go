// © Broadcom. All Rights Reserved.
// The term “Broadcom” refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"text/template"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1a5 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha6"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

func GetTemplateRenderFunc(
	vmCtx pkgctx.VirtualMachineContext,
	bsArgs *BootstrapArgs,
) TemplateRenderFunc {

	// There is a lot of duplication here, especially since the "template" types are the same in v1a1
	// and v1a2. We've conflated a lot of things here making this all a little nuts.

	networkDevicesStatusV1A1 := toTemplateNetworkStatusV1A1(bsArgs)
	networkStatusV1A1 := vmopv1a1.NetworkStatus{
		Devices:     networkDevicesStatusV1A1,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A2 := toTemplateNetworkStatusV1A2(bsArgs)
	networkStatusV1A2 := vmopv1a2.NetworkStatus{
		Devices:     networkDevicesStatusV1A2,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A3 := toTemplateNetworkStatusV1A3(bsArgs)
	networkStatusV1A3 := vmopv1a3.NetworkStatus{
		Devices:     networkDevicesStatusV1A3,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A4 := toTemplateNetworkStatusV1A4(bsArgs)
	networkStatusV1A4 := vmopv1a4.NetworkStatus{
		Devices:     networkDevicesStatusV1A4,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A5 := toTemplateNetworkStatusV1A5(bsArgs)
	networkStatusV1A5 := vmopv1a5.NetworkStatus{
		Devices:     networkDevicesStatusV1A5,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A6 := toTemplateNetworkStatusV1A6(bsArgs)
	networkStatusV1A6 := vmopv1.NetworkStatus{
		Devices:     networkDevicesStatusV1A6,
		Nameservers: bsArgs.DNSServers,
	}

	// Use separate deep copies of the VM to prevent issues caused by
	// down-converting. This prevents changing actual VM on next reconcile.
	v1a1VM := &vmopv1a1.VirtualMachine{}
	_ = v1a1VM.ConvertFrom(vmCtx.VM.DeepCopy())

	v1a2VM := &vmopv1a2.VirtualMachine{}
	_ = v1a2VM.ConvertFrom(vmCtx.VM.DeepCopy())

	v1a3VM := &vmopv1a3.VirtualMachine{}
	_ = v1a3VM.ConvertFrom(vmCtx.VM.DeepCopy())

	v1a4VM := &vmopv1a4.VirtualMachine{}
	_ = v1a4VM.ConvertFrom(vmCtx.VM.DeepCopy())

	v1a5VM := &vmopv1a5.VirtualMachine{}
	_ = v1a5VM.ConvertFrom(vmCtx.VM.DeepCopy())

	templateData := struct {
		V1alpha1 vmopv1a1.VirtualMachineTemplate
		V1alpha2 vmopv1a2.VirtualMachineTemplate
		V1alpha3 vmopv1a3.VirtualMachineTemplate
		V1alpha4 vmopv1a4.VirtualMachineTemplate
		V1alpha5 vmopv1a5.VirtualMachineTemplate
		V1alpha6 vmopv1.VirtualMachineTemplate
	}{
		V1alpha1: vmopv1a1.VirtualMachineTemplate{
			Net: networkStatusV1A1,
			VM:  v1a1VM,
		},
		V1alpha2: vmopv1a2.VirtualMachineTemplate{
			Net: networkStatusV1A2,
			VM:  v1a2VM,
		},
		V1alpha3: vmopv1a3.VirtualMachineTemplate{
			Net: networkStatusV1A3,
			VM:  v1a3VM,
		},
		V1alpha4: vmopv1a4.VirtualMachineTemplate{
			Net: networkStatusV1A4,
			VM:  v1a4VM,
		},
		V1alpha5: vmopv1a5.VirtualMachineTemplate{
			Net: networkStatusV1A5,
			VM:  v1a5VM,
		},
		V1alpha6: vmopv1.VirtualMachineTemplate{
			Net: networkStatusV1A6,
			VM:  vmCtx.VM,
		},
	}

	v1a1FuncMap := v1a1TemplateFunctions(networkStatusV1A1, networkDevicesStatusV1A1)
	v1a2FuncMap := v1a2TemplateFunctions(networkStatusV1A2, networkDevicesStatusV1A2)
	v1a3FuncMap := v1a3TemplateFunctions(networkStatusV1A3, networkDevicesStatusV1A3)
	v1a4FuncMap := v1a4TemplateFunctions(networkStatusV1A4, networkDevicesStatusV1A4)
	v1a5FuncMap := v1a5TemplateFunctions(networkStatusV1A5, networkDevicesStatusV1A5)
	v1a6FuncMap := v1a6TemplateFunctions(networkStatusV1A6, networkDevicesStatusV1A6)

	// Include all but could be nice to leave out newer versions if we could identify if this was
	// created at a prior version.
	funcMap := template.FuncMap{}
	for k, v := range v1a1FuncMap {
		funcMap[k] = v
	}
	for k, v := range v1a2FuncMap {
		funcMap[k] = v
	}
	for k, v := range v1a3FuncMap {
		funcMap[k] = v
	}
	for k, v := range v1a4FuncMap {
		funcMap[k] = v
	}
	for k, v := range v1a5FuncMap {
		funcMap[k] = v
	}
	for k, v := range v1a6FuncMap {
		funcMap[k] = v
	}

	// Skip parsing when encountering escape character('\{',"\}")
	normalizeStr := func(str string) string {
		if strings.Contains(str, "\\{") || strings.Contains(str, "\\}") {
			str = strings.ReplaceAll(str, "\\{", "{")
			str = strings.ReplaceAll(str, "\\}", "}")
		}
		return str
	}

	// TODO: Don't log, return errors instead.
	renderTemplate := func(name, templateStr string) string {
		templ, err := template.New(name).Funcs(funcMap).Parse(templateStr)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to parse template", "templateStr", templateStr)
			return normalizeStr(templateStr)
		}
		var doc bytes.Buffer
		err = templ.Execute(&doc, &templateData)
		if err != nil {
			vmCtx.Logger.Error(err, "failed to execute template", "templateStr", templateStr)
			return normalizeStr(templateStr)
		}
		return normalizeStr(doc.String())
	}

	return renderTemplate
}

func toTemplateNetworkStatusV1A1(bsArgs *BootstrapArgs) []vmopv1a1.NetworkDeviceStatus {
	networkDevicesStatus := make([]vmopv1a1.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		// When using Sysprep, the MAC address must be in the format of "-".
		// CloudInit normalizes it again to ":" when adding it to the netplan.
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := vmopv1a1.NetworkDeviceStatus{
			MacAddress: macAddr,
		}

		for _, ipConfig := range result.IPConfigs {
			// We mostly only did IPv4 before so keep that going.
			if ipConfig.IsIPv4 {
				if status.Gateway4 == "" {
					status.Gateway4 = ipConfig.Gateway
				}

				status.IPAddresses = append(status.IPAddresses, ipConfig.IPCIDR)
			}
		}

		networkDevicesStatus = append(networkDevicesStatus, status)
	}

	return networkDevicesStatus
}

func v1a1TemplateFunctions(
	networkStatusV1A1 vmopv1a1.NetworkStatus,
	networkDevicesStatusV1A1 []vmopv1a1.NetworkDeviceStatus) map[string]any {

	// Get the first IP address from the first NIC.
	v1alpha1FirstIP := func() (string, error) {
		if len(networkDevicesStatusV1A1) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A1[0].IPAddresses[0], nil
	}

	// Get the first NIC's MAC address.
	v1alpha1FirstNicMacAddr := func() (string, error) {
		if len(networkDevicesStatusV1A1) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A1[0].MacAddress, nil
	}

	// Get the first IP address from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha1FirstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatusV1A1) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A1) {
			return "", errors.New("index out of bound")
		}
		return networkDevicesStatusV1A1[index].IPAddresses[0], nil
	}

	// Get all IP addresses from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha1IPsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatusV1A1) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A1) {
			return []string{""}, errors.New("index out of bound")
		}
		return networkDevicesStatusV1A1[index].IPAddresses, nil
	}

	// Format the first occurred count of nameservers with specific delimiter
	// A negative count number would mean format all nameservers
	v1alpha1FormatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		if len(networkStatusV1A1.Nameservers) == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		if count < 0 || count >= len(networkStatusV1A1.Nameservers) {
			nameservers = networkStatusV1A1.Nameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = networkStatusV1A1.Nameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	// Get subnet mask from a CIDR notation IP address and prefix length
	// if IP address and prefix length not valid, throw an error and template string won't be parsed
	v1alpha1SubnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	// Format an IP address with default netmask CIDR
	// if IP not valid, throw an error and template string won't be parsed
	v1alpha1IP := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)
		return expectedCidrNotation, nil
	}

	// Format an IP address with network length(eg. /24) or decimal
	// notation (eg. 255.255.255.0). Format an IP/CIDR with updated mask.
	// An empty mask causes just the IP to be returned.
	v1alpha1FormatIP := func(s string, mask string) (string, error) {
		// Get the IP address for the input string.
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			ip = net.ParseIP(s)
			if ip == nil {
				return "", fmt.Errorf("input IP address not valid")
			}
		}
		// Store the IP as a string back into s.
		s = ip.String()

		// If no mask was provided then return just the IP.
		if mask == "" {
			return s, nil
		}

		// The provided mask is a network length.
		if strings.HasPrefix(mask, "/") {
			s += mask
			if _, _, err := net.ParseCIDR(s); err != nil {
				return "", err
			}
			return s, nil
		}

		// The provided mask is subnet mask.
		maskIP := net.ParseIP(mask)
		if maskIP == nil {
			return "", fmt.Errorf("mask is an invalid IP")
		}

		maskIPBytes := maskIP.To4()
		if len(maskIPBytes) == 0 {
			maskIPBytes = maskIP.To16()
		}

		ipNet := net.IPNet{
			IP:   ip,
			Mask: net.IPMask(maskIPBytes),
		}
		s = ipNet.String()

		// Validate the ipNet is an IP/CIDR
		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", fmt.Errorf("invalid ip net: %s", s)
		}

		return s, nil
	}

	return template.FuncMap{
		constants.V1alpha1FirstIP:           v1alpha1FirstIP,
		constants.V1alpha1FirstNicMacAddr:   v1alpha1FirstNicMacAddr,
		constants.V1alpha1FirstIPFromNIC:    v1alpha1FirstIPFromNIC,
		constants.V1alpha1IPsFromNIC:        v1alpha1IPsFromNIC,
		constants.V1alpha1FormatNameservers: v1alpha1FormatNameservers,
		// These are more util function that we've conflated version namespaces.
		constants.V1alpha1SubnetMask: v1alpha1SubnetMask,
		constants.V1alpha1IP:         v1alpha1IP,
		constants.V1alpha1FormatIP:   v1alpha1FormatIP,
	}
}

func toTemplateNetworkStatusV1A2(bsArgs *BootstrapArgs) []vmopv1a2.NetworkDeviceStatus {
	networkDevicesStatus := make([]vmopv1a2.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		// When using Sysprep, the MAC address must be in the format of "-".
		// CloudInit normalizes it again to ":" when adding it to the netplan.
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := vmopv1a2.NetworkDeviceStatus{
			MacAddress: macAddr,
		}

		for _, ipConfig := range result.IPConfigs {
			// We mostly only did IPv4 before so keep that going.
			if ipConfig.IsIPv4 {
				if status.Gateway4 == "" {
					status.Gateway4 = ipConfig.Gateway
				}

				status.IPAddresses = append(status.IPAddresses, ipConfig.IPCIDR)
			}
		}

		networkDevicesStatus = append(networkDevicesStatus, status)
	}

	return networkDevicesStatus
}

// This is basically identical to v1a1TemplateFunctions.
func v1a2TemplateFunctions(
	networkStatusV1A2 vmopv1a2.NetworkStatus,
	networkDevicesStatusV1A2 []vmopv1a2.NetworkDeviceStatus) map[string]any {

	// Get the first IP address from the first NIC.
	v1alpha2FirstIP := func() (string, error) {
		if len(networkDevicesStatusV1A2) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A2[0].IPAddresses[0], nil
	}

	// Get the first NIC's MAC address.
	v1alpha2FirstNicMacAddr := func() (string, error) {
		if len(networkDevicesStatusV1A2) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A2[0].MacAddress, nil
	}

	// Get the first IP address from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha2FirstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatusV1A2) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A2) {
			return "", errors.New("index out of bound")
		}
		return networkDevicesStatusV1A2[index].IPAddresses[0], nil
	}

	// Get all IP addresses from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha2IPsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatusV1A2) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A2) {
			return []string{""}, errors.New("index out of bound")
		}
		return networkDevicesStatusV1A2[index].IPAddresses, nil
	}

	// Format the first occurred count of nameservers with specific delimiter
	// A negative count number would mean format all nameservers
	v1alpha2FormatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		if len(networkStatusV1A2.Nameservers) == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		if count < 0 || count >= len(networkStatusV1A2.Nameservers) {
			nameservers = networkStatusV1A2.Nameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = networkStatusV1A2.Nameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	// Get subnet mask from a CIDR notation IP address and prefix length
	// if IP address and prefix length not valid, throw an error and template string won't be parsed
	v1alpha2SubnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	// Format an IP address with default netmask CIDR
	// if IP not valid, throw an error and template string won't be parsed
	v1alpha2IP := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)
		return expectedCidrNotation, nil
	}

	// Format an IP address with network length(eg. /24) or decimal
	// notation (eg. 255.255.255.0). Format an IP/CIDR with updated mask.
	// An empty mask causes just the IP to be returned.
	v1alpha2FormatIP := func(s string, mask string) (string, error) {
		// Get the IP address for the input string.
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			ip = net.ParseIP(s)
			if ip == nil {
				return "", fmt.Errorf("input IP address not valid")
			}
		}
		// Store the IP as a string back into s.
		s = ip.String()

		// If no mask was provided then return just the IP.
		if mask == "" {
			return s, nil
		}

		// The provided mask is a network length.
		if strings.HasPrefix(mask, "/") {
			s += mask
			if _, _, err := net.ParseCIDR(s); err != nil {
				return "", err
			}
			return s, nil
		}

		// The provided mask is subnet mask.
		maskIP := net.ParseIP(mask)
		if maskIP == nil {
			return "", fmt.Errorf("mask is an invalid IP")
		}

		maskIPBytes := maskIP.To4()
		if len(maskIPBytes) == 0 {
			maskIPBytes = maskIP.To16()
		}

		ipNet := net.IPNet{
			IP:   ip,
			Mask: net.IPMask(maskIPBytes),
		}
		s = ipNet.String()

		// Validate the ipNet is an IP/CIDR
		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", fmt.Errorf("invalid ip net: %s", s)
		}

		return s, nil
	}

	return template.FuncMap{
		constants.V1alpha2FirstIP:           v1alpha2FirstIP,
		constants.V1alpha2FirstNicMacAddr:   v1alpha2FirstNicMacAddr,
		constants.V1alpha2FirstIPFromNIC:    v1alpha2FirstIPFromNIC,
		constants.V1alpha2IPsFromNIC:        v1alpha2IPsFromNIC,
		constants.V1alpha2FormatNameservers: v1alpha2FormatNameservers,
		// These are more util function that we've conflated version namespaces.
		constants.V1alpha2SubnetMask: v1alpha2SubnetMask,
		constants.V1alpha2IP:         v1alpha2IP,
		constants.V1alpha2FormatIP:   v1alpha2FormatIP,
	}
}

func toTemplateNetworkStatusV1A3(bsArgs *BootstrapArgs) []vmopv1a3.NetworkDeviceStatus {
	networkDevicesStatus := make([]vmopv1a3.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		// When using Sysprep, the MAC address must be in the format of "-".
		// CloudInit normalizes it again to ":" when adding it to the netplan.
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := vmopv1a3.NetworkDeviceStatus{
			MacAddress: macAddr,
		}

		for _, ipConfig := range result.IPConfigs {
			// We mostly only did IPv4 before so keep that going.
			if ipConfig.IsIPv4 {
				if status.Gateway4 == "" {
					status.Gateway4 = ipConfig.Gateway
				}

				status.IPAddresses = append(status.IPAddresses, ipConfig.IPCIDR)
			}
		}

		networkDevicesStatus = append(networkDevicesStatus, status)
	}

	return networkDevicesStatus
}

func toTemplateNetworkStatusV1A4(bsArgs *BootstrapArgs) []vmopv1a4.NetworkDeviceStatus {
	networkDevicesStatus := make([]vmopv1a4.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		// When using Sysprep, the MAC address must be in the format of "-".
		// CloudInit normalizes it again to ":" when adding it to the netplan.
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := vmopv1a4.NetworkDeviceStatus{
			MacAddress: macAddr,
		}

		for _, ipConfig := range result.IPConfigs {
			// We mostly only did IPv4 before so keep that going.
			if ipConfig.IsIPv4 {
				if status.Gateway4 == "" {
					status.Gateway4 = ipConfig.Gateway
				}

				status.IPAddresses = append(status.IPAddresses, ipConfig.IPCIDR)
			}
		}

		networkDevicesStatus = append(networkDevicesStatus, status)
	}

	return networkDevicesStatus
}

func toTemplateNetworkStatusV1A5(bsArgs *BootstrapArgs) []vmopv1a5.NetworkDeviceStatus {
	networkDevicesStatus := make([]vmopv1a5.NetworkDeviceStatus, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		// When using Sysprep, the MAC address must be in the format of "-".
		// CloudInit normalizes it again to ":" when adding it to the netplan.
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := vmopv1a5.NetworkDeviceStatus{
			MacAddress: macAddr,
		}

		for _, ipConfig := range result.IPConfigs {
			// We mostly only did IPv4 before so keep that going.
			if ipConfig.IsIPv4 {
				if status.Gateway4 == "" {
					status.Gateway4 = ipConfig.Gateway
				}

				status.IPAddresses = append(status.IPAddresses, ipConfig.IPCIDR)
			}
		}

		networkDevicesStatus = append(networkDevicesStatus, status)
	}

	return networkDevicesStatus
}

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

// This is basically identical to v1a2TemplateFunctions.
func v1a3TemplateFunctions(
	networkStatusV1A3 vmopv1a3.NetworkStatus,
	networkDevicesStatusV1A3 []vmopv1a3.NetworkDeviceStatus) map[string]any {

	// Get the first IP address from the first NIC.
	v1alpha3FirstIP := func() (string, error) {
		if len(networkDevicesStatusV1A3) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A3[0].IPAddresses[0], nil
	}

	// Get the first NIC's MAC address.
	v1alpha3FirstNicMacAddr := func() (string, error) {
		if len(networkDevicesStatusV1A3) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A3[0].MacAddress, nil
	}

	// Get the first IP address from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha3FirstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatusV1A3) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A3) {
			return "", errors.New("index out of bound")
		}
		return networkDevicesStatusV1A3[index].IPAddresses[0], nil
	}

	// Get all IP addresses from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha3IPsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatusV1A3) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A3) {
			return []string{""}, errors.New("index out of bound")
		}
		return networkDevicesStatusV1A3[index].IPAddresses, nil
	}

	// Format the first occurred count of nameservers with specific delimiter
	// A negative count number would mean format all nameservers
	v1alpha3FormatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		if len(networkStatusV1A3.Nameservers) == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		if count < 0 || count >= len(networkStatusV1A3.Nameservers) {
			nameservers = networkStatusV1A3.Nameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = networkStatusV1A3.Nameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	// Get subnet mask from a CIDR notation IP address and prefix length
	// if IP address and prefix length not valid, throw an error and template string won't be parsed
	v1alpha3SubnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	// Format an IP address with default netmask CIDR
	// if IP not valid, throw an error and template string won't be parsed
	v1alpha3IP := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)
		return expectedCidrNotation, nil
	}

	// Format an IP address with network length(eg. /24) or decimal
	// notation (eg. 255.255.255.0). Format an IP/CIDR with updated mask.
	// An empty mask causes just the IP to be returned.
	v1alpha3FormatIP := func(s string, mask string) (string, error) {
		// Get the IP address for the input string.
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			ip = net.ParseIP(s)
			if ip == nil {
				return "", fmt.Errorf("input IP address not valid")
			}
		}
		// Store the IP as a string back into s.
		s = ip.String()

		// If no mask was provided then return just the IP.
		if mask == "" {
			return s, nil
		}

		// The provided mask is a network length.
		if strings.HasPrefix(mask, "/") {
			s += mask
			if _, _, err := net.ParseCIDR(s); err != nil {
				return "", err
			}
			return s, nil
		}

		// The provided mask is subnet mask.
		maskIP := net.ParseIP(mask)
		if maskIP == nil {
			return "", fmt.Errorf("mask is an invalid IP")
		}

		maskIPBytes := maskIP.To4()
		if len(maskIPBytes) == 0 {
			maskIPBytes = maskIP.To16()
		}

		ipNet := net.IPNet{
			IP:   ip,
			Mask: net.IPMask(maskIPBytes),
		}
		s = ipNet.String()

		// Validate the ipNet is an IP/CIDR
		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", fmt.Errorf("invalid ip net: %s", s)
		}

		return s, nil
	}

	return template.FuncMap{
		constants.V1alpha3FirstIP:           v1alpha3FirstIP,
		constants.V1alpha3FirstNicMacAddr:   v1alpha3FirstNicMacAddr,
		constants.V1alpha3FirstIPFromNIC:    v1alpha3FirstIPFromNIC,
		constants.V1alpha3IPsFromNIC:        v1alpha3IPsFromNIC,
		constants.V1alpha3FormatNameservers: v1alpha3FormatNameservers,
		// These are more util function that we've conflated version namespaces.
		constants.V1alpha3SubnetMask: v1alpha3SubnetMask,
		constants.V1alpha3IP:         v1alpha3IP,
		constants.V1alpha3FormatIP:   v1alpha3FormatIP,
	}
}

// This is basically identical to v1a3TemplateFunctions.
func v1a4TemplateFunctions(
	networkStatusV1A4 vmopv1a4.NetworkStatus,
	networkDevicesStatusV1A4 []vmopv1a4.NetworkDeviceStatus) map[string]any {

	// Get the first IP address from the first NIC.
	v1alpha4FirstIP := func() (string, error) {
		if len(networkDevicesStatusV1A4) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A4[0].IPAddresses[0], nil
	}

	// Get the first NIC's MAC address.
	v1alpha4FirstNicMacAddr := func() (string, error) {
		if len(networkDevicesStatusV1A4) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A4[0].MacAddress, nil
	}

	// Get the first IP address from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha4FirstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatusV1A4) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A4) {
			return "", errors.New("index out of bound")
		}
		return networkDevicesStatusV1A4[index].IPAddresses[0], nil
	}

	// Get all IP addresses from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha4IPsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatusV1A4) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A4) {
			return []string{""}, errors.New("index out of bound")
		}
		return networkDevicesStatusV1A4[index].IPAddresses, nil
	}

	// Format the first occurred count of nameservers with specific delimiter
	// A negative count number would mean format all nameservers
	v1alpha4FormatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		if len(networkStatusV1A4.Nameservers) == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		if count < 0 || count >= len(networkStatusV1A4.Nameservers) {
			nameservers = networkStatusV1A4.Nameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = networkStatusV1A4.Nameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	// Get subnet mask from a CIDR notation IP address and prefix length
	// if IP address and prefix length not valid, throw an error and template string won't be parsed
	v1alpha4SubnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	// Format an IP address with default netmask CIDR
	// if IP not valid, throw an error and template string won't be parsed
	v1alpha4IP := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)
		return expectedCidrNotation, nil
	}

	// Format an IP address with network length(eg. /24) or decimal
	// notation (eg. 255.255.255.0). Format an IP/CIDR with updated mask.
	// An empty mask causes just the IP to be returned.
	v1alpha4FormatIP := func(s string, mask string) (string, error) {
		// Get the IP address for the input string.
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			ip = net.ParseIP(s)
			if ip == nil {
				return "", fmt.Errorf("input IP address not valid")
			}
		}
		// Store the IP as a string back into s.
		s = ip.String()

		// If no mask was provided then return just the IP.
		if mask == "" {
			return s, nil
		}

		// The provided mask is a network length.
		if strings.HasPrefix(mask, "/") {
			s += mask
			if _, _, err := net.ParseCIDR(s); err != nil {
				return "", err
			}
			return s, nil
		}

		// The provided mask is subnet mask.
		maskIP := net.ParseIP(mask)
		if maskIP == nil {
			return "", fmt.Errorf("mask is an invalid IP")
		}

		maskIPBytes := maskIP.To4()
		if len(maskIPBytes) == 0 {
			maskIPBytes = maskIP.To16()
		}

		ipNet := net.IPNet{
			IP:   ip,
			Mask: net.IPMask(maskIPBytes),
		}
		s = ipNet.String()

		// Validate the ipNet is an IP/CIDR
		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", fmt.Errorf("invalid ip net: %s", s)
		}

		return s, nil
	}

	return template.FuncMap{
		constants.V1alpha4FirstIP:           v1alpha4FirstIP,
		constants.V1alpha4FirstNicMacAddr:   v1alpha4FirstNicMacAddr,
		constants.V1alpha4FirstIPFromNIC:    v1alpha4FirstIPFromNIC,
		constants.V1alpha4IPsFromNIC:        v1alpha4IPsFromNIC,
		constants.V1alpha4FormatNameservers: v1alpha4FormatNameservers,
		// These are more util function that we've conflated version namespaces.
		constants.V1alpha4SubnetMask: v1alpha4SubnetMask,
		constants.V1alpha4IP:         v1alpha4IP,
		constants.V1alpha4FormatIP:   v1alpha4FormatIP,
	}
}

func v1a5TemplateFunctions(
	networkStatusV1A5 vmopv1a5.NetworkStatus,
	networkDevicesStatusV1A5 []vmopv1a5.NetworkDeviceStatus) map[string]any {

	// Get the first IP address from the first NIC.
	v1alpha5FirstIP := func() (string, error) {
		if len(networkDevicesStatusV1A5) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A5[0].IPAddresses[0], nil
	}

	// Get the first NIC's MAC address.
	v1alpha5FirstNicMacAddr := func() (string, error) {
		if len(networkDevicesStatusV1A5) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		return networkDevicesStatusV1A5[0].MacAddress, nil
	}

	// Get the first IP address from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha5FirstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatusV1A5) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A5) {
			return "", errors.New("index out of bound")
		}
		return networkDevicesStatusV1A5[index].IPAddresses[0], nil
	}

	// Get all IP addresses from the ith NIC.
	// if index out of bound, throw an error and template string won't be parsed
	v1alpha5IPsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatusV1A5) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatusV1A5) {
			return []string{""}, errors.New("index out of bound")
		}
		return networkDevicesStatusV1A5[index].IPAddresses, nil
	}

	// Format the first occurred count of nameservers with specific delimiter
	// A negative count number would mean format all nameservers
	v1alpha5FormatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		if len(networkStatusV1A5.Nameservers) == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		if count < 0 || count >= len(networkStatusV1A5.Nameservers) {
			nameservers = networkStatusV1A5.Nameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = networkStatusV1A5.Nameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	// Get subnet mask from a CIDR notation IP address and prefix length
	// if IP address and prefix length not valid, throw an error and template string won't be parsed
	v1alpha5SubnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	// Format an IP address with default netmask CIDR
	// if IP not valid, throw an error and template string won't be parsed
	v1alpha5IP := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)
		return expectedCidrNotation, nil
	}

	// Format an IP address with network length(eg. /24) or decimal
	// notation (eg. 255.255.255.0). Format an IP/CIDR with updated mask.
	// An empty mask causes just the IP to be returned.
	v1alpha5FormatIP := func(s string, mask string) (string, error) {
		// Get the IP address for the input string.
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			ip = net.ParseIP(s)
			if ip == nil {
				return "", fmt.Errorf("input IP address not valid")
			}
		}
		// Store the IP as a string back into s.
		s = ip.String()

		// If no mask was provided then return just the IP.
		if mask == "" {
			return s, nil
		}

		// The provided mask is a network length.
		if strings.HasPrefix(mask, "/") {
			s += mask
			if _, _, err := net.ParseCIDR(s); err != nil {
				return "", err
			}
			return s, nil
		}

		// The provided mask is subnet mask.
		maskIP := net.ParseIP(mask)
		if maskIP == nil {
			return "", fmt.Errorf("mask is an invalid IP")
		}

		maskIPBytes := maskIP.To4()
		if len(maskIPBytes) == 0 {
			maskIPBytes = maskIP.To16()
		}

		ipNet := net.IPNet{
			IP:   ip,
			Mask: net.IPMask(maskIPBytes),
		}
		s = ipNet.String()

		// Validate the ipNet is an IP/CIDR
		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", fmt.Errorf("invalid ip net: %s", s)
		}

		return s, nil
	}

	return template.FuncMap{
		constants.V1alpha5FirstIP:           v1alpha5FirstIP,
		constants.V1alpha5FirstNicMacAddr:   v1alpha5FirstNicMacAddr,
		constants.V1alpha5FirstIPFromNIC:    v1alpha5FirstIPFromNIC,
		constants.V1alpha5IPsFromNIC:        v1alpha5IPsFromNIC,
		constants.V1alpha5FormatNameservers: v1alpha5FormatNameservers,
		// These are more util function that we've conflated version namespaces.
		constants.V1alpha5SubnetMask: v1alpha5SubnetMask,
		constants.V1alpha5IP:         v1alpha5IP,
		constants.V1alpha5FormatIP:   v1alpha5FormatIP,
	}
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
