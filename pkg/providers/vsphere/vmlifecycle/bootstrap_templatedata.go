// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: Apache-2.0

package vmlifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"
	"text/template"

	vmopv1a1 "github.com/vmware-tanzu/vm-operator/api/v1alpha1"
	vmopv1a2 "github.com/vmware-tanzu/vm-operator/api/v1alpha2"
	vmopv1a3 "github.com/vmware-tanzu/vm-operator/api/v1alpha3"
	vmopv1a4 "github.com/vmware-tanzu/vm-operator/api/v1alpha4"
	vmopv1 "github.com/vmware-tanzu/vm-operator/api/v1alpha5"

	pkgctx "github.com/vmware-tanzu/vm-operator/pkg/context"
	"github.com/vmware-tanzu/vm-operator/pkg/providers/vsphere/constants"
)

type NetworkDeviceStatus interface {
	~struct {
		MacAddress   string   `json:"macAddress,omitempty"`
		IPAddresses  []string `json:"ipAddresses,omitempty"`
		Gateway4     string   `json:"gateway4,omitempty"`
	}
}

type NetworkStatus[T any] interface {
	GetDevices() []T
	GetNameservers() []string
}


type TemplateConstants struct {
	FirstIP           string
	FirstNicMacAddr   string
	FirstIPFromNIC    string
	IPsFromNIC        string
	FormatNameservers string
	SubnetMask        string
	IP                string
	FormatIP          string
}

func getV1Alpha1Constants() TemplateConstants {
	return TemplateConstants{
		FirstIP:           constants.V1alpha1FirstIP,
		FirstNicMacAddr:   constants.V1alpha1FirstNicMacAddr,
		FirstIPFromNIC:    constants.V1alpha1FirstIPFromNIC,
		IPsFromNIC:        constants.V1alpha1IPsFromNIC,
		FormatNameservers: constants.V1alpha1FormatNameservers,
		SubnetMask:        constants.V1alpha1SubnetMask,
		IP:                constants.V1alpha1IP,
		FormatIP:          constants.V1alpha1FormatIP,
	}
}

func getV1Alpha2Constants() TemplateConstants {
	return TemplateConstants{
		FirstIP:           constants.V1alpha2FirstIP,
		FirstNicMacAddr:   constants.V1alpha2FirstNicMacAddr,
		FirstIPFromNIC:    constants.V1alpha2FirstIPFromNIC,
		IPsFromNIC:        constants.V1alpha2IPsFromNIC,
		FormatNameservers: constants.V1alpha2FormatNameservers,
		SubnetMask:        constants.V1alpha2SubnetMask,
		IP:                constants.V1alpha2IP,
		FormatIP:          constants.V1alpha2FormatIP,
	}
}

func getV1Alpha3Constants() TemplateConstants {
	return TemplateConstants{
		FirstIP:           constants.V1alpha3FirstIP,
		FirstNicMacAddr:   constants.V1alpha3FirstNicMacAddr,
		FirstIPFromNIC:    constants.V1alpha3FirstIPFromNIC,
		IPsFromNIC:        constants.V1alpha3IPsFromNIC,
		FormatNameservers: constants.V1alpha3FormatNameservers,
		SubnetMask:        constants.V1alpha3SubnetMask,
		IP:                constants.V1alpha3IP,
		FormatIP:          constants.V1alpha3FormatIP,
	}
}

func getV1Alpha4Constants() TemplateConstants {
	return TemplateConstants{
		FirstIP:           constants.V1alpha4FirstIP,
		FirstNicMacAddr:   constants.V1alpha4FirstNicMacAddr,
		FirstIPFromNIC:    constants.V1alpha4FirstIPFromNIC,
		IPsFromNIC:        constants.V1alpha4IPsFromNIC,
		FormatNameservers: constants.V1alpha4FormatNameservers,
		SubnetMask:        constants.V1alpha4SubnetMask,
		IP:                constants.V1alpha4IP,
		FormatIP:          constants.V1alpha4FormatIP,
	}
}

func getV1Alpha5Constants() TemplateConstants {
	return TemplateConstants{
		FirstIP:           constants.V1alpha5FirstIP,
		FirstNicMacAddr:   constants.V1alpha5FirstNicMacAddr,
		FirstIPFromNIC:    constants.V1alpha5FirstIPFromNIC,
		IPsFromNIC:        constants.V1alpha5IPsFromNIC,
		FormatNameservers: constants.V1alpha5FormatNameservers,
		SubnetMask:        constants.V1alpha5SubnetMask,
		IP:                constants.V1alpha5IP,
		FormatIP:          constants.V1alpha5FormatIP,
	}
}

func toTemplateNetworkStatus[T any](bsArgs *BootstrapArgs) []T {
	networkDevicesStatus := make([]T, 0, len(bsArgs.NetworkResults.Results))

	for _, result := range bsArgs.NetworkResults.Results {
		macAddr := strings.ReplaceAll(result.MacAddress, ":", "-")

		status := reflect.New(reflect.TypeOf((*T)(nil)).Elem()).Elem()
		status.FieldByName("MacAddress").SetString(macAddr)
		
		var ipAddresses []string
		var gateway4 string
		for _, ipConfig := range result.IPConfigs {
			if ipConfig.IsIPv4 {
				if gateway4 == "" {
					gateway4 = ipConfig.Gateway
				}
				ipAddresses = append(ipAddresses, ipConfig.IPCIDR)
			}
		}
		
		status.FieldByName("IPAddresses").Set(reflect.ValueOf(ipAddresses))
		status.FieldByName("Gateway4").SetString(gateway4)
		
		networkDevicesStatus = append(networkDevicesStatus, status.Interface().(T))
	}

	return networkDevicesStatus
}

func convertVM[T any](sourceVM interface{}) T {
	var vm T
	vmPtr := reflect.New(reflect.TypeOf(vm).Elem())
	newVM := vmPtr.Interface()
	
	// Use reflection to call ConvertFrom method
	convertFromMethod := vmPtr.MethodByName("ConvertFrom")
	convertFromMethod.Call([]reflect.Value{reflect.ValueOf(sourceVM)})
	
	return newVM.(T)
}

func createTemplateFunctions[T, N any](networkStatus N, networkDevicesStatus []T, consts TemplateConstants) template.FuncMap {
	firstIP := func() (string, error) {
		if len(networkDevicesStatus) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		statusVal := reflect.ValueOf(networkDevicesStatus[0])
		ipAddresses := statusVal.FieldByName("IPAddresses").Interface().([]string)
		return ipAddresses[0], nil
	}

	firstNicMacAddr := func() (string, error) {
		if len(networkDevicesStatus) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		statusVal := reflect.ValueOf(networkDevicesStatus[0])
		return statusVal.FieldByName("MacAddress").String(), nil
	}

	firstIPFromNIC := func(index int) (string, error) {
		if len(networkDevicesStatus) == 0 {
			return "", errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatus) {
			return "", errors.New("index out of bound")
		}
		statusVal := reflect.ValueOf(networkDevicesStatus[index])
		ipAddresses := statusVal.FieldByName("IPAddresses").Interface().([]string)
		return ipAddresses[0], nil
	}

	ipsFromNIC := func(index int) ([]string, error) {
		if len(networkDevicesStatus) == 0 {
			return []string{""}, errors.New("no available network device, check with VI admin")
		}
		if index >= len(networkDevicesStatus) {
			return []string{""}, errors.New("index out of bound")
		}
		statusVal := reflect.ValueOf(networkDevicesStatus[index])
		ipAddresses := statusVal.FieldByName("IPAddresses").Interface().([]string)
		return ipAddresses, nil
	}

	formatNameservers := func(count int, delimiter string) (string, error) {
		var nameservers []string
		networkStatusVal := reflect.ValueOf(networkStatus)
		nameserversVal := networkStatusVal.FieldByName("Nameservers")
		if nameserversVal.Len() == 0 {
			return "", errors.New("no available nameservers, check with VI admin")
		}
		allNameservers := nameserversVal.Interface().([]string)
		if count < 0 || count >= len(allNameservers) {
			nameservers = allNameservers
			return strings.Join(nameservers, delimiter), nil
		}
		nameservers = allNameservers[:count]
		return strings.Join(nameservers, delimiter), nil
	}

	subnetMask := func(cidr string) (string, error) {
		_, ipv4Net, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		netmask := fmt.Sprintf("%d.%d.%d.%d", ipv4Net.Mask[0], ipv4Net.Mask[1], ipv4Net.Mask[2], ipv4Net.Mask[3])
		return netmask, nil
	}

	ipFunc := func(IP string) (string, error) {
		if net.ParseIP(IP) == nil {
			return "", errors.New("input IP address not valid")
		}
		defaultMask := net.ParseIP(IP).DefaultMask()
		ones, _ := defaultMask.Size()
		expectedCidrNotation := IP + "/" + strconv.Itoa(ones)
		return expectedCidrNotation, nil
	}

	formatIP := func(s string, mask string) (string, error) {
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

		ipNet := net.IPNet{
			IP:   ip,
			Mask: net.IPMask(maskIPBytes),
		}
		s = ipNet.String()

		if _, _, err := net.ParseCIDR(s); err != nil {
			return "", fmt.Errorf("invalid ip net: %s", s)
		}

		return s, nil
	}

	return template.FuncMap{
		consts.FirstIP:           firstIP,
		consts.FirstNicMacAddr:   firstNicMacAddr,
		consts.FirstIPFromNIC:    firstIPFromNIC,
		consts.IPsFromNIC:        ipsFromNIC,
		consts.FormatNameservers: formatNameservers,
		consts.SubnetMask:        subnetMask,
		consts.IP:                ipFunc,
		consts.FormatIP:          formatIP,
	}
}

func GetTemplateRenderFunc(
	vmCtx pkgctx.VirtualMachineContext,
	bsArgs *BootstrapArgs,
) TemplateRenderFunc {

	networkDevicesStatusV1A1 := toTemplateNetworkStatus[vmopv1a1.NetworkDeviceStatus](bsArgs)
	networkStatusV1A1 := vmopv1a1.NetworkStatus{
		Devices:     networkDevicesStatusV1A1,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A2 := toTemplateNetworkStatus[vmopv1a2.NetworkDeviceStatus](bsArgs)
	networkStatusV1A2 := vmopv1a2.NetworkStatus{
		Devices:     networkDevicesStatusV1A2,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A3 := toTemplateNetworkStatus[vmopv1a3.NetworkDeviceStatus](bsArgs)
	networkStatusV1A3 := vmopv1a3.NetworkStatus{
		Devices:     networkDevicesStatusV1A3,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A4 := toTemplateNetworkStatus[vmopv1a4.NetworkDeviceStatus](bsArgs)
	networkStatusV1A4 := vmopv1a4.NetworkStatus{
		Devices:     networkDevicesStatusV1A4,
		Nameservers: bsArgs.DNSServers,
	}

	networkDevicesStatusV1A5 := toTemplateNetworkStatus[vmopv1.NetworkDeviceStatus](bsArgs)
	networkStatusV1A5 := vmopv1.NetworkStatus{
		Devices:     networkDevicesStatusV1A5,
		Nameservers: bsArgs.DNSServers,
	}

	// Use separate deep copies of the VM to prevent issues caused by
	// down-converting. This prevents changing actual VM on next reconcile.
	v1a1VM := convertVM[*vmopv1a1.VirtualMachine](vmCtx.VM)
	v1a2VM := convertVM[*vmopv1a2.VirtualMachine](vmCtx.VM)
	v1a3VM := convertVM[*vmopv1a3.VirtualMachine](vmCtx.VM)
	v1a4VM := convertVM[*vmopv1a4.VirtualMachine](vmCtx.VM)

	templateData := struct {
		V1alpha1 vmopv1a1.VirtualMachineTemplate
		V1alpha2 vmopv1a2.VirtualMachineTemplate
		V1alpha3 vmopv1a3.VirtualMachineTemplate
		V1alpha4 vmopv1a4.VirtualMachineTemplate
		V1alpha5 vmopv1.VirtualMachineTemplate
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
		V1alpha5: vmopv1.VirtualMachineTemplate{
			Net: networkStatusV1A5,
			VM:  vmCtx.VM,
		},
	}

	versions := []template.FuncMap{
		createTemplateFunctions(networkStatusV1A1, networkDevicesStatusV1A1, getV1Alpha1Constants()),
		createTemplateFunctions(networkStatusV1A2, networkDevicesStatusV1A2, getV1Alpha2Constants()),
		createTemplateFunctions(networkStatusV1A3, networkDevicesStatusV1A3, getV1Alpha3Constants()),
		createTemplateFunctions(networkStatusV1A4, networkDevicesStatusV1A4, getV1Alpha4Constants()),
		createTemplateFunctions(networkStatusV1A5, networkDevicesStatusV1A5, getV1Alpha5Constants()),
	}

	// Include all but could be nice to leave out newer versions if we could identify if this was
	// created at a prior version.
	funcMap := template.FuncMap{}
	for _, versionFuncMap := range versions {
		for k, v := range versionFuncMap {
			funcMap[k] = v
		}
	}

	// Skip parsing when encountering escape character('\{',"\}")
	normalizeStr := func(str string) string {
		if strings.Contains(str, "\\{") || strings.Contains(str, "\\}") {
			str = strings.ReplaceAll(str, "\\{", "{")
			str = strings.ReplaceAll(str, "\\}", "}")
		}
		return str
	}

	renderTemplate := func(name, templateStr string) (string, error) {
		// Skip parsing when encountering escape characters and just normalize them
		if strings.Contains(templateStr, "\\{") || strings.Contains(templateStr, "\\}") {
			return normalizeStr(templateStr), nil
		}
		
		templ, err := template.New(name).Funcs(funcMap).Parse(templateStr)
		if err != nil {
			return normalizeStr(templateStr), fmt.Errorf("failed to parse template: %w", err)
		}
		var doc bytes.Buffer
		err = templ.Execute(&doc, &templateData)
		if err != nil {
			return normalizeStr(templateStr), fmt.Errorf("failed to execute template: %w", err)
		}
		return normalizeStr(doc.String()), nil
	}

	return renderTemplate
}